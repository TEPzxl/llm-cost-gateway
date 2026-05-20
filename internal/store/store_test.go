package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestStoreOrganizationCRUD(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t, ctx)
	resetTestDatabase(t, ctx, st)

	orgID := uuid.New()
	created, err := st.Queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		ID:        orgID,
		Name:      "Store Test Org",
		Slug:      "store-test-org",
		Status:    "active",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrganization returned error: %v", err)
	}
	if created.ID != orgID {
		t.Fatalf("created org id = %s, want %s", created.ID, orgID)
	}

	got, err := st.Queries.GetOrganization(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrganization returned error: %v", err)
	}
	if got.Slug != "store-test-org" {
		t.Fatalf("org slug = %q, want store-test-org", got.Slug)
	}

	bySlug, err := st.Queries.GetOrganizationBySlug(ctx, "store-test-org")
	if err != nil {
		t.Fatalf("GetOrganizationBySlug returned error: %v", err)
	}
	if bySlug.ID != orgID {
		t.Fatalf("org by slug id = %s, want %s", bySlug.ID, orgID)
	}
}

func TestAPIKeyQueriesAreScopedByOrg(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t, ctx)
	resetTestDatabase(t, ctx, st)

	orgA := createTestOrganization(t, ctx, st, "api-key-org-a")
	orgB := createTestOrganization(t, ctx, st, "api-key-org-b")

	keyA, err := st.Queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ID:          uuid.New(),
		OrgID:       orgA.ID,
		Name:        "key-a",
		KeyPrefix:   "llmgw_live_a",
		KeyHash:     "hash-a",
		Scopes:      []string{"chat.completions"},
		Status:      "active",
		RpmLimit:    60,
		QuotaAction: "block",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAPIKey org A returned error: %v", err)
	}
	_, err = st.Queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ID:          uuid.New(),
		OrgID:       orgB.ID,
		Name:        "key-b",
		KeyPrefix:   "llmgw_live_b",
		KeyHash:     "hash-b",
		Scopes:      []string{"chat.completions"},
		Status:      "active",
		RpmLimit:    60,
		QuotaAction: "block",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAPIKey org B returned error: %v", err)
	}

	keys, err := st.Queries.ListAPIKeys(ctx, orgA.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys returned error: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("ListAPIKeys returned %d keys, want 1", len(keys))
	}
	if keys[0].ID != keyA.ID {
		t.Fatalf("listed key id = %s, want %s", keys[0].ID, keyA.ID)
	}

	_, err = st.Queries.GetAPIKey(ctx, db.GetAPIKeyParams{
		OrgID: orgB.ID,
		ID:    keyA.ID,
	})
	if err == nil {
		t.Fatal("GetAPIKey with another org returned nil error, want not found")
	}
}

func TestProviderAndModelQueriesAreScopedByOrg(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t, ctx)
	resetTestDatabase(t, ctx, st)

	orgA := createTestOrganization(t, ctx, st, "provider-org-a")
	orgB := createTestOrganization(t, ctx, st, "provider-org-b")

	providerA, err := st.Queries.CreateProvider(ctx, db.CreateProviderParams{
		ID:        uuid.New(),
		OrgID:     orgA.ID,
		Name:      "provider-a",
		Type:      "mock",
		TimeoutMs: 30000,
		Status:    "active",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateProvider org A returned error: %v", err)
	}
	_, err = st.Queries.CreateProvider(ctx, db.CreateProviderParams{
		ID:        uuid.New(),
		OrgID:     orgB.ID,
		Name:      "provider-b",
		Type:      "mock",
		TimeoutMs: 30000,
		Status:    "active",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateProvider org B returned error: %v", err)
	}

	modelA, err := st.Queries.CreateModel(ctx, db.CreateModelParams{
		ID:                             uuid.New(),
		OrgID:                          orgA.ID,
		ProviderID:                     providerA.ID,
		ProviderModelName:              "mock-small",
		DisplayName:                    "Mock Small",
		InputPriceMicroUsdPer1kTokens:  100,
		OutputPriceMicroUsdPer1kTokens: 200,
		Status:                         "active",
		CreatedAt:                      time.Now().UTC(),
		UpdatedAt:                      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateModel returned error: %v", err)
	}

	providers, err := st.Queries.ListProviders(ctx, orgA.ID)
	if err != nil {
		t.Fatalf("ListProviders returned error: %v", err)
	}
	if len(providers) != 1 || providers[0].ID != providerA.ID {
		t.Fatalf("ListProviders returned %+v, want only provider A", providers)
	}

	models, err := st.Queries.ListModels(ctx, orgA.ID)
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}
	if len(models) != 1 || models[0].ID != modelA.ID {
		t.Fatalf("ListModels returned %+v, want only model A", models)
	}

	_, err = st.Queries.GetModel(ctx, db.GetModelParams{
		OrgID: orgB.ID,
		ID:    modelA.ID,
	})
	if err == nil {
		t.Fatal("GetModel with another org returned nil error, want not found")
	}
}

func openTestStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()

	pool := testutil.OpenPostgres(t, ctx)
	st := New(pool)
	t.Cleanup(pool.Close)
	return st
}

func resetTestDatabase(t *testing.T, ctx context.Context, st *Store) {
	t.Helper()

	_, err := st.Pool.Exec(ctx, `
		TRUNCATE
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

func createTestOrganization(t *testing.T, ctx context.Context, st *Store, slug string) db.Organization {
	t.Helper()

	org, err := st.Queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		ID:        uuid.New(),
		Name:      slug,
		Slug:      slug,
		Status:    "active",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrganization %s returned error: %v", slug, err)
	}
	return org
}
