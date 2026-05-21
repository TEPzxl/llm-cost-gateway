package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

func TestSessionServiceUpsertsAndListsMembers(t *testing.T) {
	ctx := context.Background()
	st := openAuthTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	org := createAuthTestOrganization(t, ctx, st, "auth-members")

	service := NewSessionService(st.Queries, "session-hash-secret")
	created, err := service.UpsertMember(ctx, UpsertMemberParams{
		OrgID:       org.ID,
		Email:       " OWNER@Example.COM ",
		DisplayName: "Owner",
		Role:        RoleOwner,
	})
	if err != nil {
		t.Fatalf("UpsertMember returned error: %v", err)
	}
	if created.User.Email != "owner@example.com" {
		t.Fatalf("email = %q, want normalized email", created.User.Email)
	}
	if created.Membership.Role != RoleOwner {
		t.Fatalf("role = %q, want %q", created.Membership.Role, RoleOwner)
	}

	updated, err := service.UpsertMember(ctx, UpsertMemberParams{
		OrgID:       org.ID,
		Email:       "owner@example.com",
		DisplayName: "Owner Updated",
		Role:        RoleAdmin,
	})
	if err != nil {
		t.Fatalf("UpsertMember update returned error: %v", err)
	}
	if updated.User.ID != created.User.ID {
		t.Fatalf("updated user id = %s, want %s", updated.User.ID, created.User.ID)
	}
	if updated.Membership.ID != created.Membership.ID {
		t.Fatalf("updated membership id = %s, want %s", updated.Membership.ID, created.Membership.ID)
	}
	if updated.Membership.Role != RoleAdmin {
		t.Fatalf("updated role = %q, want %q", updated.Membership.Role, RoleAdmin)
	}

	members, err := service.ListMembers(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListMembers returned error: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("members len = %d, want 1", len(members))
	}
	if members[0].Email != "owner@example.com" || members[0].Role != RoleAdmin {
		t.Fatalf("member = %+v, want normalized admin member", members[0])
	}
}

func TestSessionServicePasswordlessMockLoginAuthenticatesSession(t *testing.T) {
	ctx := context.Background()
	st := openAuthTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	org := createAuthTestOrganization(t, ctx, st, "auth-session")
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)

	service := NewSessionService(st.Queries, "session-hash-secret", WithSessionClock(func() time.Time {
		return now
	}))
	member, err := service.UpsertMember(ctx, UpsertMemberParams{
		OrgID:       org.ID,
		Email:       "viewer@example.com",
		DisplayName: "Viewer",
		Role:        RoleViewer,
	})
	if err != nil {
		t.Fatalf("UpsertMember returned error: %v", err)
	}

	login, err := service.PasswordlessMockLogin(ctx, PasswordlessMockLoginParams{
		OrgSlug: org.Slug,
		Email:   " VIEWER@example.com ",
	})
	if err != nil {
		t.Fatalf("PasswordlessMockLogin returned error: %v", err)
	}
	if login.Token == "" {
		t.Fatal("login token is empty")
	}
	if login.Role != RoleViewer || login.MembershipID != member.Membership.ID {
		t.Fatalf("login member = %s/%s, want %s/%s", login.Role, login.MembershipID, RoleViewer, member.Membership.ID)
	}
	if !login.Session.ExpiresAt.Equal(now.Add(defaultSessionTTL)) {
		t.Fatalf("expires_at = %s, want %s", login.Session.ExpiresAt, now.Add(defaultSessionTTL))
	}

	later := now.Add(time.Minute)
	service.now = func() time.Time { return later }
	principal, err := service.Authenticate(ctx, login.Token)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if principal.OrgID != org.ID || principal.ActorType != ActorUser || principal.Role != RoleViewer {
		t.Fatalf("principal = %+v, want viewer user in org %s", principal, org.ID)
	}
	if principal.UserID == nil || *principal.UserID != member.User.ID {
		t.Fatalf("principal user id = %v, want %s", principal.UserID, member.User.ID)
	}
	if principal.SessionID == nil || *principal.SessionID != login.Session.ID {
		t.Fatalf("principal session id = %v, want %s", principal.SessionID, login.Session.ID)
	}

	stored, err := st.Queries.GetUserSessionByHash(ctx, NewTokenHasher("session-hash-secret").Hash(login.Token))
	if err != nil {
		t.Fatalf("GetUserSessionByHash returned error: %v", err)
	}
	if stored.LastUsedAt == nil || !stored.LastUsedAt.Equal(later) {
		t.Fatalf("last_used_at = %v, want %s", stored.LastUsedAt, later)
	}
}

func TestSessionServiceAuthenticatesSessionHashedWithOldSecret(t *testing.T) {
	ctx := context.Background()
	st := openAuthTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	org := createAuthTestOrganization(t, ctx, st, "auth-session-rotation")

	oldService := NewSessionService(st.Queries, "old-token-hash-secret")
	member, err := oldService.UpsertMember(ctx, UpsertMemberParams{
		OrgID:       org.ID,
		Email:       "viewer@example.com",
		DisplayName: "Viewer",
		Role:        RoleViewer,
	})
	if err != nil {
		t.Fatalf("UpsertMember returned error: %v", err)
	}
	login, err := oldService.PasswordlessMockLogin(ctx, PasswordlessMockLoginParams{
		OrgSlug: org.Slug,
		Email:   member.User.Email,
	})
	if err != nil {
		t.Fatalf("PasswordlessMockLogin returned error: %v", err)
	}
	keyRing, err := NewTokenHashKeyRing(2, map[int32]string{
		1: "old-token-hash-secret",
		2: "new-token-hash-secret",
	})
	if err != nil {
		t.Fatalf("NewTokenHashKeyRing returned error: %v", err)
	}
	newService := NewSessionService(st.Queries, "new-token-hash-secret", WithSessionHashKeyRing(keyRing))

	principal, err := newService.Authenticate(ctx, login.Token)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if principal.SessionID == nil || *principal.SessionID != login.Session.ID {
		t.Fatalf("principal session id = %v, want %s", principal.SessionID, login.Session.ID)
	}
}

func TestSessionServicePasswordlessMockLoginRejectsUnknownMember(t *testing.T) {
	ctx := context.Background()
	st := openAuthTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	_ = createAuthTestOrganization(t, ctx, st, "auth-missing-member")

	service := NewSessionService(st.Queries, "session-hash-secret")
	_, err := service.PasswordlessMockLogin(ctx, PasswordlessMockLoginParams{
		OrgSlug: "auth-missing-member",
		Email:   "missing@example.com",
	})
	if !errors.Is(err, ErrMembershipNotFound) {
		t.Fatalf("PasswordlessMockLogin error = %v, want %v", err, ErrMembershipNotFound)
	}
}

func TestSessionServiceAuthenticateRejectsInactiveSessions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *testing.T, *store.Store, *SessionService, db.UserSession)
	}{
		{
			name: "revoked session",
			mutate: func(ctx context.Context, t *testing.T, _ *store.Store, service *SessionService, session db.UserSession) {
				t.Helper()
				if _, err := service.Revoke(ctx, session.OrgID, session.ID); err != nil {
					t.Fatalf("Revoke returned error: %v", err)
				}
			},
		},
		{
			name: "expired session",
			mutate: func(_ context.Context, _ *testing.T, _ *store.Store, service *SessionService, _ db.UserSession) {
				service.now = func() time.Time {
					return time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC)
				}
			},
		},
		{
			name: "disabled membership",
			mutate: func(ctx context.Context, t *testing.T, st *store.Store, _ *SessionService, session db.UserSession) {
				t.Helper()
				_, err := sessionStoreExec(ctx, st, session.OrgID, session.UserID, "disabled")
				if err != nil {
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
			org := createAuthTestOrganization(t, ctx, st, "auth-inactive-"+uuid.NewString()[:8])
			now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)

			service := NewSessionService(st.Queries, "session-hash-secret", WithSessionClock(func() time.Time {
				return now
			}), WithSessionTTL(time.Hour))
			if _, err := service.UpsertMember(ctx, UpsertMemberParams{
				OrgID:       org.ID,
				Email:       "viewer@example.com",
				DisplayName: "Viewer",
				Role:        RoleViewer,
			}); err != nil {
				t.Fatalf("UpsertMember returned error: %v", err)
			}
			login, err := service.PasswordlessMockLogin(ctx, PasswordlessMockLoginParams{
				OrgSlug: org.Slug,
				Email:   "viewer@example.com",
			})
			if err != nil {
				t.Fatalf("PasswordlessMockLogin returned error: %v", err)
			}

			tt.mutate(ctx, t, st, service, login.Session)
			if _, err := service.Authenticate(ctx, login.Token); !errors.Is(err, ErrInvalidSession) {
				t.Fatalf("Authenticate error = %v, want %v", err, ErrInvalidSession)
			}
		})
	}
}

func sessionStoreExec(ctx context.Context, st *store.Store, orgID uuid.UUID, userID uuid.UUID, status string) (int64, error) {
	tag, err := st.Pool.Exec(ctx, `
		UPDATE org_memberships
		SET status = $3
		WHERE org_id = $1 AND user_id = $2
	`, orgID, userID, status)
	return tag.RowsAffected(), err
}
