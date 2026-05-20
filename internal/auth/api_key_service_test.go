package auth

import (
	"context"
	"testing"
	"time"

	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestAPIKeyServiceCreatesAndAuthenticatesKey(t *testing.T) {
	ctx := context.Background()
	st := openAPIKeyTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	org := createAuthTestOrganization(t, ctx, st, "auth-api-key")

	service := NewAPIKeyService(st.Queries, "token-hash-secret")

	created, err := service.CreateAPIKey(ctx, CreateAPIKeyParams{
		OrgID:    org.ID,
		Name:     "docs-assistant-prod",
		Scopes:   []string{"chat.completions"},
		RPMLimit: 60,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	if created.Key == "" {
		t.Fatal("created key is empty")
	}
	if created.APIKey.KeyHash == created.Key {
		t.Fatal("stored key hash equals plaintext key")
	}
	if created.APIKey.KeyPrefix == "" {
		t.Fatal("created key prefix is empty")
	}

	stored, err := st.Queries.GetAPIKeyByHash(ctx, NewTokenHasher("token-hash-secret").Hash(created.Key))
	if err != nil {
		t.Fatalf("GetAPIKeyByHash returned error: %v", err)
	}
	if stored.ID != created.APIKey.ID {
		t.Fatalf("stored api key id = %s, want %s", stored.ID, created.APIKey.ID)
	}

	principal, err := service.Authenticate(ctx, created.Key)
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if principal.OrgID != org.ID {
		t.Fatalf("principal org id = %s, want %s", principal.OrgID, org.ID)
	}
	if principal.APIKeyID != created.APIKey.ID {
		t.Fatalf("principal api key id = %s, want %s", principal.APIKeyID, created.APIKey.ID)
	}
	if principal.RPMLimit != 60 {
		t.Fatalf("principal rpm limit = %d, want 60", principal.RPMLimit)
	}
}

func TestAPIKeyServiceRejectsExpiredKey(t *testing.T) {
	ctx := context.Background()
	st := openAPIKeyTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	org := createAuthTestOrganization(t, ctx, st, "auth-expired-api-key")

	service := NewAPIKeyService(st.Queries, "token-hash-secret")
	expiresAt := time.Now().UTC().Add(-time.Minute)
	created, err := service.CreateAPIKey(ctx, CreateAPIKeyParams{
		OrgID:     org.ID,
		Name:      "expired-key",
		Scopes:    []string{"chat.completions"},
		RPMLimit:  60,
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	if _, err := service.Authenticate(ctx, created.Key); err == nil {
		t.Fatal("Authenticate returned nil error for expired key")
	}
}

func TestAPIKeyServiceRejectsRevokedKey(t *testing.T) {
	ctx := context.Background()
	st := openAPIKeyTestStore(t, ctx)
	resetAuthTestDatabase(t, ctx, st)
	org := createAuthTestOrganization(t, ctx, st, "auth-revoked-api-key")

	service := NewAPIKeyService(st.Queries, "token-hash-secret")
	created, err := service.CreateAPIKey(ctx, CreateAPIKeyParams{
		OrgID:    org.ID,
		Name:     "revoked-key",
		Scopes:   []string{"chat.completions"},
		RPMLimit: 60,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}

	now := time.Now().UTC()
	if _, err := st.Queries.RevokeAPIKey(ctx, db.RevokeAPIKeyParams{
		OrgID:     org.ID,
		ID:        created.APIKey.ID,
		RevokedAt: &now,
	}); err != nil {
		t.Fatalf("RevokeAPIKey returned error: %v", err)
	}

	if _, err := service.Authenticate(ctx, created.Key); err == nil {
		t.Fatal("Authenticate returned nil error for revoked key")
	}
}

func openAPIKeyTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()

	pool := testutil.OpenPostgres(t, ctx)
	return store.New(pool)
}
