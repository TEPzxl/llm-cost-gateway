package costing

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestPricingServiceCreatesInitialVersionAndUpdatesPricing(t *testing.T) {
	ctx := context.Background()
	st := openPricingTestStore(t, ctx)
	resetPricingTestDatabase(t, ctx, st)
	model := createPricingTestModel(t, ctx, st, 100, 200)
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	service := NewPricingService(st, WithPricingClock(func() time.Time { return now }))

	initial, err := service.CreateInitialVersion(ctx, model, now)
	if err != nil {
		t.Fatalf("CreateInitialVersion returned error: %v", err)
	}
	if initial.Version != 1 || initial.Status != PricingStatusActive {
		t.Fatalf("initial version = version:%d status:%q, want version 1 active", initial.Version, initial.Status)
	}
	if initial.InputPriceMicroUsdPer1kTokens != 100 || initial.OutputPriceMicroUsdPer1kTokens != 200 {
		t.Fatalf("initial prices = %d/%d, want 100/200", initial.InputPriceMicroUsdPer1kTokens, initial.OutputPriceMicroUsdPer1kTokens)
	}

	updated, err := service.UpdateModelPricing(ctx, UpdateModelPricingParams{
		OrgID:                          model.OrgID,
		ModelID:                        model.ID,
		InputPriceMicroUSDPer1KTokens:  300,
		OutputPriceMicroUSDPer1KTokens: 400,
	})
	if err != nil {
		t.Fatalf("UpdateModelPricing returned error: %v", err)
	}
	if updated.Model.InputPriceMicroUsdPer1kTokens != 300 || updated.Model.OutputPriceMicroUsdPer1kTokens != 400 {
		t.Fatalf("model prices = %d/%d, want 300/400", updated.Model.InputPriceMicroUsdPer1kTokens, updated.Model.OutputPriceMicroUsdPer1kTokens)
	}
	if updated.PricingVersion.Version != 2 || updated.PricingVersion.Status != PricingStatusActive {
		t.Fatalf("new version = version:%d status:%q, want version 2 active", updated.PricingVersion.Version, updated.PricingVersion.Status)
	}
	if updated.PreviousVersion.Status != PricingStatusSuperseded || updated.PreviousVersion.EffectiveTo == nil {
		t.Fatalf("previous version = status:%q effective_to:%v, want superseded with effective_to", updated.PreviousVersion.Status, updated.PreviousVersion.EffectiveTo)
	}

	versions, err := service.ListModelPricingVersions(ctx, model.OrgID, model.ID)
	if err != nil {
		t.Fatalf("ListModelPricingVersions returned error: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("version count = %d, want 2", len(versions))
	}
	if versions[0].Version != 2 || versions[0].Status != PricingStatusActive {
		t.Fatalf("latest version = %+v, want version 2 active", versions[0])
	}
	if versions[1].Version != 1 || versions[1].Status != PricingStatusSuperseded {
		t.Fatalf("old version = %+v, want version 1 superseded", versions[1])
	}
}

func TestPricingServiceRejectsInvalidPrice(t *testing.T) {
	ctx := context.Background()
	st := openPricingTestStore(t, ctx)
	resetPricingTestDatabase(t, ctx, st)
	model := createPricingTestModel(t, ctx, st, 100, 200)
	service := NewPricingService(st)

	_, err := service.UpdateModelPricing(ctx, UpdateModelPricingParams{
		OrgID:                          model.OrgID,
		ModelID:                        model.ID,
		InputPriceMicroUSDPer1KTokens:  -1,
		OutputPriceMicroUSDPer1KTokens: 400,
	})
	if err == nil {
		t.Fatal("UpdateModelPricing returned nil error for negative price")
	}
}

func openPricingTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()

	return store.New(testutil.OpenPostgres(t, ctx))
}

func resetPricingTestDatabase(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()

	_, err := st.Pool.Exec(ctx, `
		TRUNCATE
			cost_records,
			usage_records,
			request_logs,
			model_pricing_versions,
			budget_alert_deliveries,
			budget_alerts,
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

func createPricingTestModel(t *testing.T, ctx context.Context, st *store.Store, inputPrice int64, outputPrice int64) db.Model {
	t.Helper()

	now := time.Now().UTC()
	org, err := st.Queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		ID:        uuid.New(),
		Name:      "pricing-org",
		Slug:      "pricing-org-" + uuid.NewString(),
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrganization returned error: %v", err)
	}
	provider, err := st.Queries.CreateProvider(ctx, db.CreateProviderParams{
		ID:        uuid.New(),
		OrgID:     org.ID,
		Name:      "pricing-provider-" + uuid.NewString(),
		Type:      "mock",
		TimeoutMs: 30000,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateProvider returned error: %v", err)
	}
	model, err := st.Queries.CreateModel(ctx, db.CreateModelParams{
		ID:                             uuid.New(),
		OrgID:                          org.ID,
		ProviderID:                     provider.ID,
		ProviderModelName:              "pricing-model-" + uuid.NewString(),
		DisplayName:                    "Pricing Model",
		InputPriceMicroUsdPer1kTokens:  inputPrice,
		OutputPriceMicroUsdPer1kTokens: outputPrice,
		ContextWindow:                  pgtype.Int4{Int32: 8192, Valid: true},
		Status:                         "active",
		CreatedAt:                      now,
		UpdatedAt:                      now,
	})
	if err != nil {
		t.Fatalf("CreateModel returned error: %v", err)
	}
	return model
}
