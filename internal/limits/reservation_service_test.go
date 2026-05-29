package limits

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestReservationServiceConcurrentReserveAllowsOnlyCapacity(t *testing.T) {
	ctx := context.Background()
	st := openReservationTestStore(t, ctx)
	resetReservationTestDatabase(t, ctx, st)
	fixture := createReservationFixture(t, ctx, st)
	createReservationBudget(t, ctx, st, fixture.OrgID, 1000)

	service := NewReservationService(st, WithClock(func() time.Time {
		return time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	}))

	const workers = 2
	results := make(chan ReserveResult, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := service.Reserve(ctx, ReserveInput{
				RequestID:             uuid.New(),
				OrgID:                 fixture.OrgID,
				APIKeyID:              fixture.APIKeyID,
				EstimatedCostMicroUSD: 600,
			})
			if err != nil {
				t.Errorf("Reserve returned error: %v", err)
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)

	allowed := 0
	blocked := 0
	for result := range results {
		if result.Allowed {
			allowed++
		} else if result.BlockedScope == ScopeTypeOrg {
			blocked++
		}
	}
	if allowed != 1 || blocked != 1 {
		t.Fatalf("allowed=%d blocked=%d, want one allowed and one org block", allowed, blocked)
	}
	reserved, settled := readReservationCounter(t, ctx, st, ScopeTypeOrg, fixture.OrgID)
	if reserved != 600 || settled != 0 {
		t.Fatalf("counter reserved=%d settled=%d, want reserved=600 settled=0", reserved, settled)
	}
}

func TestReservationServiceSettleMovesReservedToSettledUsage(t *testing.T) {
	ctx := context.Background()
	st := openReservationTestStore(t, ctx)
	resetReservationTestDatabase(t, ctx, st)
	fixture := createReservationFixture(t, ctx, st)
	createReservationBudget(t, ctx, st, fixture.OrgID, 1000)

	requestID := uuid.New()
	service := NewReservationService(st, WithClock(func() time.Time {
		return time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	}))
	result, err := service.Reserve(ctx, ReserveInput{
		RequestID:             requestID,
		OrgID:                 fixture.OrgID,
		APIKeyID:              fixture.APIKeyID,
		EstimatedCostMicroUSD: 600,
	})
	if err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}
	if !result.Allowed {
		t.Fatalf("Reserve allowed = false, want true")
	}

	if err := service.Settle(ctx, requestID, 450); err != nil {
		t.Fatalf("Settle returned error: %v", err)
	}

	reserved, settled := readReservationCounter(t, ctx, st, ScopeTypeOrg, fixture.OrgID)
	if reserved != 0 || settled != 450 {
		t.Fatalf("counter reserved=%d settled=%d, want reserved=0 settled=450", reserved, settled)
	}
}

func TestReservationServiceReleaseRemovesOutstandingReservation(t *testing.T) {
	ctx := context.Background()
	st := openReservationTestStore(t, ctx)
	resetReservationTestDatabase(t, ctx, st)
	fixture := createReservationFixture(t, ctx, st)
	createReservationBudget(t, ctx, st, fixture.OrgID, 1000)

	requestID := uuid.New()
	service := NewReservationService(st, WithClock(func() time.Time {
		return time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	}))
	result, err := service.Reserve(ctx, ReserveInput{
		RequestID:             requestID,
		OrgID:                 fixture.OrgID,
		APIKeyID:              fixture.APIKeyID,
		EstimatedCostMicroUSD: 600,
	})
	if err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}
	if !result.Allowed {
		t.Fatalf("Reserve allowed = false, want true")
	}

	if err := service.Release(ctx, requestID); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}

	reserved, settled := readReservationCounter(t, ctx, st, ScopeTypeOrg, fixture.OrgID)
	if reserved != 0 || settled != 0 {
		t.Fatalf("counter reserved=%d settled=%d, want zero", reserved, settled)
	}
}

func TestReservationServiceUsesStrictestBudgetWhenMultipleBudgetsSharePeriod(t *testing.T) {
	ctx := context.Background()
	st := openReservationTestStore(t, ctx)
	resetReservationTestDatabase(t, ctx, st)
	fixture := createReservationFixture(t, ctx, st)
	createReservationBudget(t, ctx, st, fixture.OrgID, 1000)
	createReservationBudget(t, ctx, st, fixture.OrgID, 500)

	service := NewReservationService(st, WithClock(func() time.Time {
		return time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	}))
	first, err := service.Reserve(ctx, ReserveInput{
		RequestID:             uuid.New(),
		OrgID:                 fixture.OrgID,
		APIKeyID:              fixture.APIKeyID,
		EstimatedCostMicroUSD: 400,
	})
	if err != nil {
		t.Fatalf("first Reserve returned error: %v", err)
	}
	if !first.Allowed {
		t.Fatalf("first Reserve allowed = false, want true")
	}
	second, err := service.Reserve(ctx, ReserveInput{
		RequestID:             uuid.New(),
		OrgID:                 fixture.OrgID,
		APIKeyID:              fixture.APIKeyID,
		EstimatedCostMicroUSD: 200,
	})
	if err != nil {
		t.Fatalf("second Reserve returned error: %v", err)
	}
	if second.Allowed || second.LimitMicroUSD != 500 {
		t.Fatalf("second result = %+v, want strictest 500 microUSD budget block", second)
	}
}

func TestReservationServiceAPIKeyQuotaBlocksIndependently(t *testing.T) {
	ctx := context.Background()
	st := openReservationTestStore(t, ctx)
	resetReservationTestDatabase(t, ctx, st)
	fixture := createReservationFixture(t, ctx, st)
	setReservationAPIKeyDailyLimit(t, ctx, st, fixture.OrgID, fixture.APIKeyID, 1000)

	service := NewReservationService(st, WithClock(func() time.Time {
		return time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	}))
	first, err := service.Reserve(ctx, ReserveInput{
		RequestID:             uuid.New(),
		OrgID:                 fixture.OrgID,
		APIKeyID:              fixture.APIKeyID,
		EstimatedCostMicroUSD: 700,
	})
	if err != nil {
		t.Fatalf("first Reserve returned error: %v", err)
	}
	if !first.Allowed {
		t.Fatalf("first Reserve allowed = false, want true")
	}
	second, err := service.Reserve(ctx, ReserveInput{
		RequestID:             uuid.New(),
		OrgID:                 fixture.OrgID,
		APIKeyID:              fixture.APIKeyID,
		EstimatedCostMicroUSD: 400,
	})
	if err != nil {
		t.Fatalf("second Reserve returned error: %v", err)
	}
	if second.Allowed || second.BlockedScope != ScopeTypeAPIKey {
		t.Fatalf("second result = %+v, want api key quota block", second)
	}
}

type reservationFixture struct {
	OrgID      uuid.UUID
	APIKeyID   uuid.UUID
	ProviderID uuid.UUID
	ModelID    uuid.UUID
}

func openReservationTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()
	return store.New(testutil.OpenPostgres(t, ctx))
}

func resetReservationTestDatabase(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()
	_, err := st.Pool.Exec(ctx, `
		TRUNCATE
			cost_reservations,
			cost_limit_counters,
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

func createReservationFixture(t *testing.T, ctx context.Context, st *store.Store) reservationFixture {
	t.Helper()
	now := time.Now().UTC()
	org, err := st.Queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		ID:        uuid.New(),
		Name:      "reservation-org",
		Slug:      "reservation-org-" + uuid.NewString(),
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrganization returned error: %v", err)
	}
	apiKey, err := st.Queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ID:          uuid.New(),
		OrgID:       org.ID,
		Name:        "reservation-key",
		KeyPrefix:   "llmgw_live_test",
		KeyHash:     "reservation-key-hash-" + uuid.NewString(),
		Scopes:      []string{"chat.completions"},
		Status:      "active",
		RpmLimit:    60,
		QuotaAction: "block",
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	provider, err := st.Queries.CreateProvider(ctx, db.CreateProviderParams{
		ID:        uuid.New(),
		OrgID:     org.ID,
		Name:      "reservation-provider-" + uuid.NewString(),
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
		ProviderModelName:              "reservation-model-" + uuid.NewString(),
		DisplayName:                    "Reservation Model",
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
	return reservationFixture{OrgID: org.ID, APIKeyID: apiKey.ID, ProviderID: provider.ID, ModelID: model.ID}
}

func createReservationBudget(t *testing.T, ctx context.Context, st *store.Store, orgID uuid.UUID, limit int64) {
	t.Helper()
	now := time.Now().UTC()
	_, err := st.Queries.CreateBudget(ctx, db.CreateBudgetParams{
		ID:            uuid.New(),
		OrgID:         orgID,
		Name:          "reservation-budget",
		ScopeType:     ScopeTypeOrg,
		Period:        PeriodDaily,
		LimitMicroUsd: limit,
		Action:        ActionBlock,
		Status:        StatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("CreateBudget returned error: %v", err)
	}
}

func setReservationAPIKeyDailyLimit(t *testing.T, ctx context.Context, st *store.Store, orgID uuid.UUID, apiKeyID uuid.UUID, limit int64) {
	t.Helper()
	_, err := st.Pool.Exec(ctx, `
		UPDATE api_keys
		SET daily_cost_limit_micro_usd = $3,
		    quota_action = 'block'
		WHERE org_id = $1 AND id = $2
	`, orgID, apiKeyID, limit)
	if err != nil {
		t.Fatalf("update api key quota: %v", err)
	}
}

func readReservationCounter(t *testing.T, ctx context.Context, st *store.Store, scopeType string, scopeID uuid.UUID) (reserved int64, settled int64) {
	t.Helper()
	if err := st.Pool.QueryRow(ctx, `
		SELECT reserved_micro_usd, settled_micro_usd
		FROM cost_limit_counters
		WHERE scope_type = $1 AND scope_id = $2
	`, scopeType, scopeID).Scan(&reserved, &settled); err != nil {
		t.Fatalf("read cost counter: %v", err)
	}
	return reserved, settled
}
