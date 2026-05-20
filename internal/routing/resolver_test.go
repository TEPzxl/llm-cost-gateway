package routing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestResolverResolvesSingleRoutePolicy(t *testing.T) {
	ctx := context.Background()
	st := openRoutingTestStore(t, ctx)
	resetRoutingTestDatabase(t, ctx, st)

	org := createRoutingTestOrg(t, ctx, st, "resolver-org")
	provider := createRoutingTestProvider(t, ctx, st, org.ID, "mock-provider", "active")
	model := createRoutingTestModel(t, ctx, st, org.ID, provider.ID, "mock-small", "active")
	createRoutingTestPolicy(t, ctx, st, org.ID, "fast-chat-policy", "fast-chat", provider.ID, model.ID)

	resolver := NewResolver(st.Queries)
	result, err := resolver.Resolve(ctx, ResolveParams{
		OrgID:          org.ID,
		RequestedModel: "fast-chat",
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if result.RoutePolicy.MatchModel != "fast-chat" {
		t.Fatalf("match model = %q, want fast-chat", result.RoutePolicy.MatchModel)
	}
	if result.Provider.ID != provider.ID {
		t.Fatalf("provider id = %s, want %s", result.Provider.ID, provider.ID)
	}
	if result.Model.ID != model.ID {
		t.Fatalf("model id = %s, want %s", result.Model.ID, model.ID)
	}
}

func TestResolverResolveTargetsReturnsFallbackTargetsByPriority(t *testing.T) {
	ctx := context.Background()
	st := openRoutingTestStore(t, ctx)
	resetRoutingTestDatabase(t, ctx, st)

	org := createRoutingTestOrg(t, ctx, st, "resolver-fallback-org")
	providerA := createRoutingTestProvider(t, ctx, st, org.ID, "mock-provider-a", "active")
	modelA := createRoutingTestModel(t, ctx, st, org.ID, providerA.ID, "mock-small-a", "active")
	providerB := createRoutingTestProvider(t, ctx, st, org.ID, "mock-provider-b", "active")
	modelB := createRoutingTestModel(t, ctx, st, org.ID, providerB.ID, "mock-small-b", "active")
	createRoutingTestFallbackPolicy(t, ctx, st, org.ID, "fallback-policy", "fallback-chat", []routingTestTarget{
		{ProviderID: providerB.ID, ModelID: modelB.ID, Priority: 2},
		{ProviderID: providerA.ID, ModelID: modelA.ID, Priority: 1},
	})

	resolver := NewResolver(st.Queries)
	result, err := resolver.ResolveTargets(ctx, ResolveParams{
		OrgID:          org.ID,
		RequestedModel: "fallback-chat",
	})
	if err != nil {
		t.Fatalf("ResolveTargets returned error: %v", err)
	}
	if result.RoutePolicy.Strategy != StrategyFallback {
		t.Fatalf("strategy = %q, want fallback", result.RoutePolicy.Strategy)
	}
	if len(result.Targets) != 2 {
		t.Fatalf("targets len = %d, want 2", len(result.Targets))
	}
	if result.Targets[0].Provider.ID != providerA.ID || result.Targets[1].Provider.ID != providerB.ID {
		t.Fatalf("target providers = %s/%s, want %s/%s", result.Targets[0].Provider.ID, result.Targets[1].Provider.ID, providerA.ID, providerB.ID)
	}
}

func TestResolverReturnsRouteNotFoundForMissingAlias(t *testing.T) {
	ctx := context.Background()
	st := openRoutingTestStore(t, ctx)
	resetRoutingTestDatabase(t, ctx, st)
	org := createRoutingTestOrg(t, ctx, st, "resolver-missing-org")

	resolver := NewResolver(st.Queries)
	_, err := resolver.Resolve(ctx, ResolveParams{
		OrgID:          org.ID,
		RequestedModel: "missing-model",
	})
	if !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("Resolve error = %v, want ErrRouteNotFound", err)
	}
}

func TestResolverReturnsRouteNotFoundForDisabledProviderOrModel(t *testing.T) {
	tests := []struct {
		name           string
		providerStatus string
		modelStatus    string
	}{
		{name: "disabled provider", providerStatus: "disabled", modelStatus: "active"},
		{name: "disabled model", providerStatus: "active", modelStatus: "disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := openRoutingTestStore(t, ctx)
			resetRoutingTestDatabase(t, ctx, st)

			org := createRoutingTestOrg(t, ctx, st, "resolver-disabled-"+tt.name)
			provider := createRoutingTestProvider(t, ctx, st, org.ID, "mock-provider", tt.providerStatus)
			model := createRoutingTestModel(t, ctx, st, org.ID, provider.ID, "mock-small", tt.modelStatus)
			createRoutingTestPolicy(t, ctx, st, org.ID, "fast-chat-policy", "fast-chat", provider.ID, model.ID)

			resolver := NewResolver(st.Queries)
			_, err := resolver.Resolve(ctx, ResolveParams{
				OrgID:          org.ID,
				RequestedModel: "fast-chat",
			})
			if !errors.Is(err, ErrRouteNotFound) {
				t.Fatalf("Resolve error = %v, want ErrRouteNotFound", err)
			}
		})
	}
}

func openRoutingTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()

	pool := testutil.OpenPostgres(t, ctx)
	return store.New(pool)
}

func resetRoutingTestDatabase(t *testing.T, ctx context.Context, st *store.Store) {
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

func createRoutingTestOrg(t *testing.T, ctx context.Context, st *store.Store, slug string) db.Organization {
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

func createRoutingTestProvider(t *testing.T, ctx context.Context, st *store.Store, orgID uuid.UUID, name string, status string) db.Provider {
	t.Helper()

	now := time.Now().UTC()
	provider, err := st.Queries.CreateProvider(ctx, db.CreateProviderParams{
		ID:        uuid.New(),
		OrgID:     orgID,
		Name:      name,
		Type:      "mock",
		TimeoutMs: 30000,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateProvider returned error: %v", err)
	}
	return provider
}

func createRoutingTestModel(t *testing.T, ctx context.Context, st *store.Store, orgID uuid.UUID, providerID uuid.UUID, providerModelName string, status string) db.Model {
	t.Helper()

	now := time.Now().UTC()
	model, err := st.Queries.CreateModel(ctx, db.CreateModelParams{
		ID:                             uuid.New(),
		OrgID:                          orgID,
		ProviderID:                     providerID,
		ProviderModelName:              providerModelName,
		DisplayName:                    providerModelName,
		InputPriceMicroUsdPer1kTokens:  100,
		OutputPriceMicroUsdPer1kTokens: 200,
		ContextWindow:                  pgtype.Int4{Int32: 8192, Valid: true},
		Status:                         status,
		CreatedAt:                      now,
		UpdatedAt:                      now,
	})
	if err != nil {
		t.Fatalf("CreateModel returned error: %v", err)
	}
	return model
}

func createRoutingTestPolicy(t *testing.T, ctx context.Context, st *store.Store, orgID uuid.UUID, name string, matchModel string, providerID uuid.UUID, modelID uuid.UUID) db.RoutePolicy {
	t.Helper()

	now := time.Now().UTC()
	policy, err := st.Queries.CreateRoutePolicy(ctx, db.CreateRoutePolicyParams{
		ID:         uuid.New(),
		OrgID:      orgID,
		Name:       name,
		MatchModel: matchModel,
		Strategy:   "single",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateRoutePolicy returned error: %v", err)
	}
	if _, err := st.Queries.CreateRouteTarget(ctx, db.CreateRouteTargetParams{
		ID:            uuid.New(),
		OrgID:         orgID,
		RoutePolicyID: policy.ID,
		ProviderID:    providerID,
		ModelID:       modelID,
		Priority:      1,
		Weight:        100,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateRouteTarget returned error: %v", err)
	}
	return policy
}

type routingTestTarget struct {
	ProviderID uuid.UUID
	ModelID    uuid.UUID
	Priority   int32
}

func createRoutingTestFallbackPolicy(t *testing.T, ctx context.Context, st *store.Store, orgID uuid.UUID, name string, matchModel string, targets []routingTestTarget) db.RoutePolicy {
	t.Helper()

	now := time.Now().UTC()
	policy, err := st.Queries.CreateRoutePolicy(ctx, db.CreateRoutePolicyParams{
		ID:         uuid.New(),
		OrgID:      orgID,
		Name:       name,
		MatchModel: matchModel,
		Strategy:   "fallback",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateRoutePolicy returned error: %v", err)
	}
	for _, target := range targets {
		if _, err := st.Queries.CreateRouteTarget(ctx, db.CreateRouteTargetParams{
			ID:            uuid.New(),
			OrgID:         orgID,
			RoutePolicyID: policy.ID,
			ProviderID:    target.ProviderID,
			ModelID:       target.ModelID,
			Priority:      target.Priority,
			Weight:        100,
			CreatedAt:     now,
		}); err != nil {
			t.Fatalf("CreateRouteTarget returned error: %v", err)
		}
	}
	return policy
}
