package budget

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

func TestServiceCreateBudgetDefaultsToActiveOrgScope(t *testing.T) {
	ctx := context.Background()
	st := openBudgetTestStore(t, ctx)
	resetBudgetTestDatabase(t, ctx, st)
	orgID := createBudgetTestOrg(t, ctx, st)

	service := NewService(st.Queries)
	created, err := service.CreateBudget(ctx, CreateBudgetParams{
		OrgID:         orgID,
		Name:          "daily guardrail",
		ScopeType:     ScopeTypeOrg,
		Period:        PeriodDaily,
		LimitMicroUSD: 1000,
		Action:        ActionBlock,
	})
	if err != nil {
		t.Fatalf("CreateBudget returned error: %v", err)
	}

	if created.OrgID != orgID || created.ScopeType != ScopeTypeOrg || created.ScopeID != nil {
		t.Fatalf("created budget scope = org_id:%s scope_type:%q scope_id:%v", created.OrgID, created.ScopeType, created.ScopeID)
	}
	if created.Status != StatusActive {
		t.Fatalf("status = %q, want %q", created.Status, StatusActive)
	}
	if created.LimitMicroUsd != 1000 {
		t.Fatalf("limit = %d, want 1000", created.LimitMicroUsd)
	}
}

func TestServiceStatusUsesDailyAndMonthlyWindows(t *testing.T) {
	ctx := context.Background()
	st := openBudgetTestStore(t, ctx)
	resetBudgetTestDatabase(t, ctx, st)
	orgID := createBudgetTestOrg(t, ctx, st)
	fixture := createBudgetCostFixture(t, ctx, st, orgID)
	now := time.Date(2026, 5, 20, 13, 30, 0, 0, time.UTC)
	service := NewService(st.Queries, WithClock(func() time.Time { return now }))

	createBudget(t, ctx, service, orgID, "daily", PeriodDaily, 1000, ActionBlock, StatusActive)
	createBudget(t, ctx, service, orgID, "monthly", PeriodMonthly, 1000, ActionBlock, StatusActive)

	insertBudgetCost(t, ctx, st, fixture, 100, now.Add(-2*time.Hour))
	insertBudgetCost(t, ctx, st, fixture, 200, now.AddDate(0, 0, -2))
	insertBudgetCost(t, ctx, st, fixture, 400, now.AddDate(0, -1, 0))

	statuses, err := service.GetStatus(ctx, orgID)
	if err != nil {
		t.Fatalf("GetStatus returned error: %v", err)
	}

	byName := budgetStatusByName(statuses)
	if byName["daily"].UsedMicroUSD != 100 {
		t.Fatalf("daily used = %d, want 100", byName["daily"].UsedMicroUSD)
	}
	if byName["monthly"].UsedMicroUSD != 300 {
		t.Fatalf("monthly used = %d, want 300", byName["monthly"].UsedMicroUSD)
	}
}

func TestServiceCheckAllowsWhenUnderBudget(t *testing.T) {
	ctx := context.Background()
	st := openBudgetTestStore(t, ctx)
	resetBudgetTestDatabase(t, ctx, st)
	orgID := createBudgetTestOrg(t, ctx, st)
	fixture := createBudgetCostFixture(t, ctx, st, orgID)
	now := time.Date(2026, 5, 20, 13, 30, 0, 0, time.UTC)
	service := NewService(st.Queries, WithClock(func() time.Time { return now }))

	createBudget(t, ctx, service, orgID, "block-under", PeriodDaily, 1000, ActionBlock, StatusActive)
	insertBudgetCost(t, ctx, st, fixture, 999, now.Add(-time.Hour))

	result, err := service.Check(ctx, orgID)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !result.Allowed || result.Exceeded {
		t.Fatalf("result = %+v, want allowed under budget", result)
	}
}

func TestServiceCheckWarnDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	st := openBudgetTestStore(t, ctx)
	resetBudgetTestDatabase(t, ctx, st)
	orgID := createBudgetTestOrg(t, ctx, st)
	fixture := createBudgetCostFixture(t, ctx, st, orgID)
	now := time.Date(2026, 5, 20, 13, 30, 0, 0, time.UTC)
	service := NewService(st.Queries, WithClock(func() time.Time { return now }))

	createBudget(t, ctx, service, orgID, "warn-budget", PeriodDaily, 1000, ActionWarn, StatusActive)
	insertBudgetCost(t, ctx, st, fixture, 1000, now.Add(-time.Hour))

	result, err := service.Check(ctx, orgID)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !result.Allowed || !result.Exceeded || !result.Warning {
		t.Fatalf("result = %+v, want allowed warning", result)
	}
	if result.Action != ActionWarn {
		t.Fatalf("action = %q, want %q", result.Action, ActionWarn)
	}
}

func TestServiceCheckBlockRejectsWhenExceeded(t *testing.T) {
	ctx := context.Background()
	st := openBudgetTestStore(t, ctx)
	resetBudgetTestDatabase(t, ctx, st)
	orgID := createBudgetTestOrg(t, ctx, st)
	fixture := createBudgetCostFixture(t, ctx, st, orgID)
	now := time.Date(2026, 5, 20, 13, 30, 0, 0, time.UTC)
	service := NewService(st.Queries, WithClock(func() time.Time { return now }))

	createBudget(t, ctx, service, orgID, "block-budget", PeriodDaily, 1000, ActionBlock, StatusActive)
	insertBudgetCost(t, ctx, st, fixture, 1000, now.Add(-time.Hour))

	result, err := service.Check(ctx, orgID)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Allowed || !result.Exceeded {
		t.Fatalf("result = %+v, want blocked exceeded budget", result)
	}
	if result.Action != ActionBlock {
		t.Fatalf("action = %q, want %q", result.Action, ActionBlock)
	}
}

func TestServiceCheckIgnoresDisabledBudget(t *testing.T) {
	ctx := context.Background()
	st := openBudgetTestStore(t, ctx)
	resetBudgetTestDatabase(t, ctx, st)
	orgID := createBudgetTestOrg(t, ctx, st)
	fixture := createBudgetCostFixture(t, ctx, st, orgID)
	now := time.Date(2026, 5, 20, 13, 30, 0, 0, time.UTC)
	service := NewService(st.Queries, WithClock(func() time.Time { return now }))

	createBudget(t, ctx, service, orgID, "disabled-budget", PeriodDaily, 1000, ActionBlock, StatusDisabled)
	insertBudgetCost(t, ctx, st, fixture, 5000, now.Add(-time.Hour))

	result, err := service.Check(ctx, orgID)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !result.Allowed || result.Exceeded {
		t.Fatalf("result = %+v, want disabled budget ignored", result)
	}
}

type budgetCostFixture struct {
	OrgID      uuid.UUID
	APIKeyID   uuid.UUID
	ProviderID uuid.UUID
	ModelID    uuid.UUID
}

func openBudgetTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()

	return store.New(testutil.OpenPostgres(t, ctx))
}

func resetBudgetTestDatabase(t *testing.T, ctx context.Context, st *store.Store) {
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

func createBudgetTestOrg(t *testing.T, ctx context.Context, st *store.Store) uuid.UUID {
	t.Helper()

	now := time.Now().UTC()
	org, err := st.Queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		ID:        uuid.New(),
		Name:      "budget-org",
		Slug:      "budget-org-" + uuid.NewString(),
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrganization returned error: %v", err)
	}
	return org.ID
}

func createBudgetCostFixture(t *testing.T, ctx context.Context, st *store.Store, orgID uuid.UUID) budgetCostFixture {
	t.Helper()

	now := time.Now().UTC()
	apiKey, err := st.Queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ID:        uuid.New(),
		OrgID:     orgID,
		Name:      "budget-key",
		KeyPrefix: "llmgw_live_test",
		KeyHash:   "budget-key-hash-" + uuid.NewString(),
		Scopes:    []string{"chat.completions"},
		Status:    "active",
		RpmLimit:  60,
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	provider, err := st.Queries.CreateProvider(ctx, db.CreateProviderParams{
		ID:        uuid.New(),
		OrgID:     orgID,
		Name:      "budget-provider-" + uuid.NewString(),
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
		OrgID:                          orgID,
		ProviderID:                     provider.ID,
		ProviderModelName:              "budget-model-" + uuid.NewString(),
		DisplayName:                    "Budget Model",
		InputPriceMicroUsdPer1kTokens:  100,
		OutputPriceMicroUsdPer1kTokens: 200,
		ContextWindow:                  pgtype.Int4{Int32: 8192, Valid: true},
		Status:                         "active",
		CreatedAt:                      now,
		UpdatedAt:                      now,
	})
	if err != nil {
		t.Fatalf("CreateModel returned error: %v", err)
	}
	return budgetCostFixture{
		OrgID:      orgID,
		APIKeyID:   apiKey.ID,
		ProviderID: provider.ID,
		ModelID:    model.ID,
	}
}

func insertBudgetCost(t *testing.T, ctx context.Context, st *store.Store, fixture budgetCostFixture, totalMicroUSD int64, createdAt time.Time) {
	t.Helper()

	requestLogID := uuid.New()
	usageRecordID := uuid.New()
	if _, err := st.Queries.InsertRequestLog(ctx, db.InsertRequestLogParams{
		ID:          requestLogID,
		OrgID:       fixture.OrgID,
		ApiKeyID:    &fixture.APIKeyID,
		ProviderID:  &fixture.ProviderID,
		ModelID:     &fixture.ModelID,
		Method:      "POST",
		Path:        "/v1/chat/completions",
		Status:      "success",
		StatusCode:  200,
		LatencyMs:   100,
		StartedAt:   createdAt,
		CompletedAt: createdAt,
	}); err != nil {
		t.Fatalf("InsertRequestLog returned error: %v", err)
	}
	if _, err := st.Queries.InsertUsageRecord(ctx, db.InsertUsageRecordParams{
		ID:               usageRecordID,
		RequestLogID:     requestLogID,
		OrgID:            fixture.OrgID,
		ApiKeyID:         fixture.APIKeyID,
		ProviderID:       fixture.ProviderID,
		ModelID:          fixture.ModelID,
		PromptTokens:     1,
		CompletionTokens: 1,
		TotalTokens:      2,
		CreatedAt:        createdAt,
	}); err != nil {
		t.Fatalf("InsertUsageRecord returned error: %v", err)
	}
	if _, err := st.Queries.InsertCostRecord(ctx, db.InsertCostRecordParams{
		ID:              uuid.New(),
		UsageRecordID:   usageRecordID,
		OrgID:           fixture.OrgID,
		ProviderID:      fixture.ProviderID,
		ModelID:         fixture.ModelID,
		Currency:        "USD",
		InputCostMicro:  totalMicroUSD,
		OutputCostMicro: 0,
		TotalCostMicro:  totalMicroUSD,
		PricingSnapshot: []byte(`{"source":"test"}`),
		CreatedAt:       createdAt,
	}); err != nil {
		t.Fatalf("InsertCostRecord returned error: %v", err)
	}
}

func createBudget(t *testing.T, ctx context.Context, service *Service, orgID uuid.UUID, name string, period string, limitMicroUSD int64, action string, status string) {
	t.Helper()

	if status == StatusActive {
		if _, err := service.CreateBudget(ctx, CreateBudgetParams{
			OrgID:         orgID,
			Name:          name,
			ScopeType:     ScopeTypeOrg,
			Period:        period,
			LimitMicroUSD: limitMicroUSD,
			Action:        action,
		}); err != nil {
			t.Fatalf("CreateBudget returned error: %v", err)
		}
		return
	}

	now := service.clock()
	if _, err := service.queries.CreateBudget(ctx, db.CreateBudgetParams{
		ID:            uuid.New(),
		OrgID:         orgID,
		Name:          name,
		ScopeType:     ScopeTypeOrg,
		Period:        period,
		LimitMicroUsd: limitMicroUSD,
		Action:        action,
		Status:        status,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateBudget with status %q returned error: %v", status, err)
	}
}

func budgetStatusByName(statuses []Status) map[string]Status {
	byName := make(map[string]Status, len(statuses))
	for _, status := range statuses {
		byName[status.Budget.Name] = status
	}
	return byName
}
