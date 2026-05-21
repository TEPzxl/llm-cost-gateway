package auth

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/email"
	"github.com/tep/llm-cost-gateway/internal/store"
)

func TestMagicLinkServiceRequestsAndConsumesOneTimeToken(t *testing.T) {
	ctx := context.Background()
	st := openAuthTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	org := createAuthTestOrganization(t, ctx, st, "magic-link-org")
	now := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)

	sessionService := NewSessionService(st.Queries, "session-hash-secret", WithSessionClock(func() time.Time {
		return now
	}))
	member, err := sessionService.UpsertMember(ctx, UpsertMemberParams{
		OrgID:       org.ID,
		Email:       " Owner@Example.COM ",
		DisplayName: "Owner",
		Role:        RoleOwner,
	})
	if err != nil {
		t.Fatalf("UpsertMember returned error: %v", err)
	}

	sender := email.NewFakeSender()
	service := NewMagicLinkService(st, "session-hash-secret", sender, MagicLinkConfig{
		BaseURL: "https://console.example.com/login",
		TTL:     15 * time.Minute,
	}, WithMagicLinkClock(func() time.Time {
		return now
	}))

	if err := service.Request(ctx, MagicLinkRequestParams{
		OrgSlug: org.Slug,
		Email:   "owner@example.com",
	}); err != nil {
		t.Fatalf("Request returned error: %v", err)
	}
	messages := sender.Messages()
	if len(messages) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(messages))
	}
	if messages[0].To != "owner@example.com" {
		t.Fatalf("message to = %q, want normalized email", messages[0].To)
	}
	token := extractMagicLinkToken(t, messages[0].TextBody)
	if token == "" {
		t.Fatal("magic link token is empty")
	}

	var storedHash string
	if err := st.Pool.QueryRow(ctx, `SELECT token_hash FROM magic_link_tokens WHERE org_id = $1`, org.ID).Scan(&storedHash); err != nil {
		t.Fatalf("read stored magic link token hash: %v", err)
	}
	if storedHash == token || strings.Contains(messages[0].TextBody, storedHash) {
		t.Fatal("magic link storage or email leaked token hash/plaintext in the wrong place")
	}

	login, err := service.Consume(ctx, MagicLinkConsumeParams{Token: token})
	if err != nil {
		t.Fatalf("Consume returned error: %v", err)
	}
	if login.Token == "" || login.Role != RoleOwner || login.MembershipID != member.Membership.ID {
		t.Fatalf("login = %+v, want owner session for membership %s", login, member.Membership.ID)
	}
	if _, err := sessionService.Authenticate(ctx, login.Token); err != nil {
		t.Fatalf("Authenticate consumed session returned error: %v", err)
	}

	if _, err := service.Consume(ctx, MagicLinkConsumeParams{Token: token}); !errors.Is(err, ErrInvalidMagicLink) {
		t.Fatalf("second Consume error = %v, want %v", err, ErrInvalidMagicLink)
	}
}

func TestMagicLinkServiceRejectsExpiredAndDisabledMemberTokens(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *testing.T, *store.Store, *MagicLinkService)
	}{
		{
			name: "expired token",
			mutate: func(_ context.Context, _ *testing.T, _ *store.Store, service *MagicLinkService) {
				service.now = func() time.Time {
					return time.Date(2026, 5, 22, 10, 0, 1, 0, time.UTC)
				}
			},
		},
		{
			name: "disabled membership",
			mutate: func(ctx context.Context, t *testing.T, st *store.Store, _ *MagicLinkService) {
				t.Helper()
				if _, err := st.Pool.Exec(ctx, `UPDATE org_memberships SET status = 'disabled'`); err != nil {
					t.Fatalf("disable membership: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := openAuthTestStore(t, ctx)
			resetAuthTestDatabase(t, ctx, st)
			org := createAuthTestOrganization(t, ctx, st, "magic-link-"+uuid.NewString()[:8])
			now := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)

			sessionService := NewSessionService(st.Queries, "session-hash-secret", WithSessionClock(func() time.Time {
				return now
			}))
			if _, err := sessionService.UpsertMember(ctx, UpsertMemberParams{
				OrgID:       org.ID,
				Email:       "viewer@example.com",
				DisplayName: "Viewer",
				Role:        RoleViewer,
			}); err != nil {
				t.Fatalf("UpsertMember returned error: %v", err)
			}

			sender := email.NewFakeSender()
			service := NewMagicLinkService(st, "session-hash-secret", sender, MagicLinkConfig{
				BaseURL: "https://console.example.com/login",
				TTL:     time.Minute,
			}, WithMagicLinkClock(func() time.Time {
				return now
			}))
			if err := service.Request(ctx, MagicLinkRequestParams{
				OrgSlug: org.Slug,
				Email:   "viewer@example.com",
			}); err != nil {
				t.Fatalf("Request returned error: %v", err)
			}
			token := extractMagicLinkToken(t, sender.Messages()[0].TextBody)

			tt.mutate(ctx, t, st, service)
			if _, err := service.Consume(ctx, MagicLinkConsumeParams{Token: token}); !errors.Is(err, ErrInvalidMagicLink) {
				t.Fatalf("Consume error = %v, want %v", err, ErrInvalidMagicLink)
			}
		})
	}
}

func TestMagicLinkServiceDoesNotEnumerateUnknownEmail(t *testing.T) {
	ctx := context.Background()
	st := openAuthTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	org := createAuthTestOrganization(t, ctx, st, "magic-link-enum")

	sender := email.NewFakeSender()
	service := NewMagicLinkService(st, "session-hash-secret", sender, MagicLinkConfig{
		BaseURL: "https://console.example.com/login",
		TTL:     time.Minute,
	})
	if err := service.Request(ctx, MagicLinkRequestParams{
		OrgSlug: org.Slug,
		Email:   "missing@example.com",
	}); err != nil {
		t.Fatalf("Request unknown email returned error: %v", err)
	}
	if len(sender.Messages()) != 0 {
		t.Fatalf("sent messages for unknown email = %d, want 0", len(sender.Messages()))
	}
}

func extractMagicLinkToken(t *testing.T, body string) string {
	t.Helper()
	fields := strings.Fields(body)
	for _, field := range fields {
		parsed, err := url.Parse(strings.TrimSpace(field))
		if err != nil {
			continue
		}
		if token := parsed.Query().Get("magic_token"); token != "" {
			return token
		}
	}
	t.Fatalf("magic_token not found in email body: %q", body)
	return ""
}
