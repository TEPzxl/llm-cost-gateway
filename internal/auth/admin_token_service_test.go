package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestAdminTokenServiceCreatesAndAuthenticatesToken(t *testing.T) {
	ctx := context.Background()
	st := openAuthTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	org := createAuthTestOrganization(t, ctx, st, "auth-admin-token")

	service := NewAdminTokenService(st.Queries, "token-hash-secret")

	created, err := service.CreateAdminToken(ctx, CreateAdminTokenParams{
		OrgID:  org.ID,
		Name:   "initial-admin-token",
		Scopes: []string{"admin:*"},
	})
	if err != nil {
		t.Fatalf("CreateAdminToken returned error: %v", err)
	}
	if created.Token == "" {
		t.Fatal("created token is empty")
	}
	if created.AdminToken.TokenPrefix == "" {
		t.Fatal("created token prefix is empty")
	}
	if created.AdminToken.TokenHash == created.Token {
		t.Fatal("stored token hash equals plaintext token")
	}

	stored, err := st.Queries.GetAdminTokenByHash(ctx, NewTokenHasher("token-hash-secret").Hash(created.Token))
	if err != nil {
		t.Fatalf("GetAdminTokenByHash returned error: %v", err)
	}
	if stored.ID != created.AdminToken.ID {
		t.Fatalf("stored token id = %s, want %s", stored.ID, created.AdminToken.ID)
	}

	principal, err := service.Authenticate(ctx, created.Token)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if principal.OrgID != org.ID {
		t.Fatalf("principal org id = %s, want %s", principal.OrgID, org.ID)
	}
	if principal.AdminTokenID != created.AdminToken.ID {
		t.Fatalf("principal token id = %s, want %s", principal.AdminTokenID, created.AdminToken.ID)
	}
	if principal.ActorType != ActorServiceToken || principal.Role != RoleOwner {
		t.Fatalf("principal actor = %s/%s, want %s/%s", principal.ActorType, principal.Role, ActorServiceToken, RoleOwner)
	}
}

func TestAdminTokenServiceAuthenticatesTokenHashedWithOldSecret(t *testing.T) {
	ctx := context.Background()
	st := openAuthTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	org := createAuthTestOrganization(t, ctx, st, "admin-token-rotation")

	oldService := NewAdminTokenService(st.Queries, "old-token-hash-secret")
	created, err := oldService.CreateAdminToken(ctx, CreateAdminTokenParams{
		OrgID: org.ID,
		Name:  "old-token",
	})
	if err != nil {
		t.Fatalf("CreateAdminToken returned error: %v", err)
	}
	keyRing, err := NewTokenHashKeyRing(2, map[int32]string{
		1: "old-token-hash-secret",
		2: "new-token-hash-secret",
	})
	if err != nil {
		t.Fatalf("NewTokenHashKeyRing returned error: %v", err)
	}
	newService := NewAdminTokenService(st.Queries, "new-token-hash-secret", WithAdminTokenHashKeyRing(keyRing))

	principal, err := newService.Authenticate(ctx, created.Token)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if principal.AdminTokenID != created.AdminToken.ID {
		t.Fatalf("principal token id = %s, want %s", principal.AdminTokenID, created.AdminToken.ID)
	}
}

func TestAdminTokenServiceRejectsRevokedToken(t *testing.T) {
	ctx := context.Background()
	st := openAuthTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	org := createAuthTestOrganization(t, ctx, st, "auth-revoked-token")

	service := NewAdminTokenService(st.Queries, "token-hash-secret")
	created, err := service.CreateAdminToken(ctx, CreateAdminTokenParams{
		OrgID:  org.ID,
		Name:   "revoked-admin-token",
		Scopes: []string{"admin:*"},
	})
	if err != nil {
		t.Fatalf("CreateAdminToken returned error: %v", err)
	}

	now := time.Now().UTC()
	if _, err := st.Queries.RevokeAdminToken(ctx, db.RevokeAdminTokenParams{
		OrgID:     org.ID,
		ID:        created.AdminToken.ID,
		RevokedAt: &now,
	}); err != nil {
		t.Fatalf("RevokeAdminToken returned error: %v", err)
	}

	if _, err := service.Authenticate(ctx, created.Token); err == nil {
		t.Fatal("Authenticate returned nil error for revoked token")
	}
}

func openAuthTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()

	pool := testutil.OpenPostgres(t, ctx)
	return store.New(pool)
}

func resetAuthTestDatabase(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()

	_, err := st.Pool.Exec(ctx, `
		TRUNCATE
			user_sessions,
			org_memberships,
			users,
			cost_records,
			usage_records,
			request_logs,
			budgets,
			route_targets,
			route_policies,
			models,
			provider_secrets,
			providers,
			api_keys,
			admin_tokens,
			organizations
		CASCADE
	`)
	if err != nil {
		t.Fatalf("reset test database: %v", err)
	}
}

func createAuthTestOrganization(t *testing.T, ctx context.Context, st *store.Store, slug string) db.Organization {
	t.Helper()

	now := time.Now().UTC()
	org, err := st.Queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		ID:        uuid.New(),
		Name:      slug,
		Slug:      slug,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrganization returned error: %v", err)
	}
	return org
}
