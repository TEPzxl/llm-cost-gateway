package analytics

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestUsageAnalyticsServicePostgresQueries(t *testing.T) {
	ctx := context.Background()
	st := openAnalyticsTestStore(t, ctx)
	resetAnalyticsTestDatabase(t, ctx, st)
	fixture := createAnalyticsFixture(t, ctx, st, "analytics-org")
	other := createAnalyticsFixture(t, ctx, st, "analytics-other")
	base := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)

	insertAnalyticsRecord(t, ctx, st, analyticsRecordInput{
		OrgID: fixture.OrgID, APIKeyID: fixture.APIKeyID, ProviderID: fixture.ProviderID, ModelID: fixture.ModelID,
		Status: statusSuccess, PromptTokens: 20, CompletionTokens: 30, CostMicroUSD: 8, LatencyMS: 100, CreatedAt: base,
	})
	insertAnalyticsRecord(t, ctx, st, analyticsRecordInput{
		OrgID: fixture.OrgID, APIKeyID: fixture.APIKeyID, ProviderID: fixture.ProviderID, ModelID: fixture.ModelID,
		Status: "error", LatencyMS: 300, CreatedAt: base.Add(time.Hour),
	})
	insertAnalyticsRecord(t, ctx, st, analyticsRecordInput{
		OrgID: fixture.OrgID, APIKeyID: fixture.APIKeyID, ProviderID: fixture.ProviderID, ModelID: fixture.ModelID,
		Status: statusSuccess, PromptTokens: 40, CompletionTokens: 35, CostMicroUSD: 12, LatencyMS: 200, CreatedAt: base.Add(24 * time.Hour),
	})
	insertAnalyticsRecord(t, ctx, st, analyticsRecordInput{
		OrgID: other.OrgID, APIKeyID: other.APIKeyID, ProviderID: other.ProviderID, ModelID: other.ModelID,
		Status: statusSuccess, PromptTokens: 100, CompletionTokens: 100, CostMicroUSD: 999, LatencyMS: 50, CreatedAt: base,
	})

	service := NewUsageAnalyticsService(st, nil, false)
	window := TimeWindow{From: base.Add(-time.Hour), To: base.Add(48 * time.Hour)}

	daily, err := service.DailyCostTrend(ctx, fixture.OrgID, window)
	if err != nil {
		t.Fatalf("DailyCostTrend returned error: %v", err)
	}
	if len(daily) != 2 {
		t.Fatalf("daily count = %d, want 2: %+v", len(daily), daily)
	}
	if daily[0].Day != "2026-05-20" || daily[0].RequestCount != 2 || daily[0].TotalCostMicroUSD != 8 {
		t.Fatalf("daily[0] = %+v, want day1 request=2 cost=8", daily[0])
	}
	if daily[1].Day != "2026-05-21" || daily[1].RequestCount != 1 || daily[1].TotalCostMicroUSD != 12 {
		t.Fatalf("daily[1] = %+v, want day2 request=1 cost=12", daily[1])
	}

	models, err := service.ModelCostBreakdown(ctx, fixture.OrgID, window)
	if err != nil {
		t.Fatalf("ModelCostBreakdown returned error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("model breakdown count = %d, want 1", len(models))
	}
	if models[0].ModelID != fixture.ModelID || models[0].RequestCount != 2 || models[0].TotalTokens != 125 || models[0].TotalCostMicroUSD != 20 {
		t.Fatalf("model breakdown = %+v, want model requests=2 tokens=125 cost=20", models[0])
	}

	latencies, err := service.ProviderLatency(ctx, fixture.OrgID, window)
	if err != nil {
		t.Fatalf("ProviderLatency returned error: %v", err)
	}
	if len(latencies) != 1 {
		t.Fatalf("provider latency count = %d, want 1", len(latencies))
	}
	if latencies[0].ProviderID != fixture.ProviderID || latencies[0].RequestCount != 3 || latencies[0].AverageLatencyMS != 200 || latencies[0].P50LatencyMS != 200 {
		t.Fatalf("provider latency = %+v, want request=3 avg=200 p50=200", latencies[0])
	}
	if latencies[0].P95LatencyMS <= 200 {
		t.Fatalf("p95 latency = %f, want above 200", latencies[0].P95LatencyMS)
	}

	errors, err := service.ErrorRateTrend(ctx, fixture.OrgID, window)
	if err != nil {
		t.Fatalf("ErrorRateTrend returned error: %v", err)
	}
	if len(errors) != 2 {
		t.Fatalf("error trend count = %d, want 2", len(errors))
	}
	if errors[0].Day != "2026-05-20" || errors[0].RequestCount != 2 || errors[0].ErrorCount != 1 || errors[0].ErrorRate != 0.5 {
		t.Fatalf("errors[0] = %+v, want day1 request=2 error=1 rate=.5", errors[0])
	}
	if errors[1].Day != "2026-05-21" || errors[1].RequestCount != 1 || errors[1].ErrorCount != 0 || errors[1].ErrorRate != 0 {
		t.Fatalf("errors[1] = %+v, want day2 request=1 error=0 rate=0", errors[1])
	}
}

func TestUsageAnalyticsServiceClickHouseQueries(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	fake := &fakeClickHouseBackend{
		queryRows: []json.RawMessage{
			json.RawMessage(`{"day":"2026-05-21","request_count":2,"total_cost_micro_usd":20}`),
		},
	}
	service := NewUsageAnalyticsService(nil, fake, true)

	items, err := service.DailyCostTrend(ctx, orgID, TimeWindow{
		From: time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("DailyCostTrend returned error: %v", err)
	}
	if len(items) != 1 || items[0].TotalCostMicroUSD != 20 {
		t.Fatalf("items = %+v, want one cost item", items)
	}
	if len(fake.queries) != 1 || !strings.Contains(fake.queries[0], orgID.String()) || !strings.Contains(fake.queries[0], "usage_events") {
		t.Fatalf("query = %v, want org-scoped usage_events query", fake.queries)
	}
}

func TestUsageAnalyticsServiceWriteUsageRecordUsesClickHouseWhenEnabled(t *testing.T) {
	ctx := context.Background()
	fake := &fakeClickHouseBackend{}
	service := NewUsageAnalyticsService(nil, fake, true)
	record := UsageRecord{RequestID: uuid.New(), OrgID: uuid.New(), Status: statusSuccess}

	if err := service.WriteUsageRecord(ctx, record); err != nil {
		t.Fatalf("WriteUsageRecord returned error: %v", err)
	}
	if len(fake.records) != 1 || fake.records[0].RequestID != record.RequestID {
		t.Fatalf("records = %+v, want written record", fake.records)
	}
}

type analyticsFixture struct {
	OrgID      uuid.UUID
	APIKeyID   uuid.UUID
	ProviderID uuid.UUID
	ModelID    uuid.UUID
}

type analyticsRecordInput struct {
	OrgID            uuid.UUID
	APIKeyID         uuid.UUID
	ProviderID       uuid.UUID
	ModelID          uuid.UUID
	Status           string
	PromptTokens     int32
	CompletionTokens int32
	CostMicroUSD     int64
	LatencyMS        int32
	CreatedAt        time.Time
}

func openAnalyticsTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()

	pool := testutil.OpenPostgres(t, ctx)
	return store.New(pool)
}

func resetAnalyticsTestDatabase(t *testing.T, ctx context.Context, st *store.Store) {
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

func createAnalyticsFixture(t *testing.T, ctx context.Context, st *store.Store, slugPrefix string) analyticsFixture {
	t.Helper()

	now := time.Now().UTC()
	org, err := st.Queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		ID:        uuid.New(),
		Name:      slugPrefix,
		Slug:      slugPrefix + "-" + uuid.NewString(),
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
		Name:        slugPrefix + "-key",
		KeyPrefix:   "llmgw_live_test",
		KeyHash:     slugPrefix + "-hash-" + uuid.NewString(),
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
		Name:      slugPrefix + "-provider",
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
		ProviderModelName:              slugPrefix + "-model",
		DisplayName:                    slugPrefix + " Model",
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
	return analyticsFixture{OrgID: org.ID, APIKeyID: apiKey.ID, ProviderID: provider.ID, ModelID: model.ID}
}

func insertAnalyticsRecord(t *testing.T, ctx context.Context, st *store.Store, input analyticsRecordInput) {
	t.Helper()

	requestID := uuid.New()
	apiKeyID := input.APIKeyID
	providerID := input.ProviderID
	modelID := input.ModelID
	if _, err := st.Queries.InsertRequestLog(ctx, db.InsertRequestLogParams{
		ID:           requestID,
		OrgID:        input.OrgID,
		ApiKeyID:     &apiKeyID,
		ProviderID:   &providerID,
		ModelID:      &modelID,
		Method:       "POST",
		Path:         "/v1/chat/completions",
		RequestModel: pgtype.Text{String: "fast-chat", Valid: true},
		Status:       input.Status,
		StatusCode:   200,
		RequestHash:  pgtype.Text{String: "sha256:request", Valid: true},
		ResponseHash: pgtype.Text{String: "sha256:response", Valid: true},
		LatencyMs:    input.LatencyMS,
		StartedAt:    input.CreatedAt.Add(-time.Second),
		CompletedAt:  input.CreatedAt,
	}); err != nil {
		t.Fatalf("InsertRequestLog returned error: %v", err)
	}
	if input.PromptTokens == 0 && input.CompletionTokens == 0 && input.CostMicroUSD == 0 {
		return
	}
	usageRecord, err := st.Queries.InsertUsageRecord(ctx, db.InsertUsageRecordParams{
		ID:               uuid.New(),
		RequestLogID:     requestID,
		OrgID:            input.OrgID,
		ApiKeyID:         input.APIKeyID,
		ProviderID:       input.ProviderID,
		ModelID:          input.ModelID,
		PromptTokens:     input.PromptTokens,
		CompletionTokens: input.CompletionTokens,
		TotalTokens:      input.PromptTokens + input.CompletionTokens,
		CreatedAt:        input.CreatedAt,
	})
	if err != nil {
		t.Fatalf("InsertUsageRecord returned error: %v", err)
	}
	if _, err := st.Queries.InsertCostRecord(ctx, db.InsertCostRecordParams{
		ID:              uuid.New(),
		UsageRecordID:   usageRecord.ID,
		OrgID:           input.OrgID,
		ProviderID:      input.ProviderID,
		ModelID:         input.ModelID,
		Currency:        "USD",
		InputCostMicro:  input.CostMicroUSD,
		OutputCostMicro: 0,
		TotalCostMicro:  input.CostMicroUSD,
		PricingSnapshot: []byte(`{"source":"test"}`),
		CreatedAt:       input.CreatedAt,
	}); err != nil {
		t.Fatalf("InsertCostRecord returned error: %v", err)
	}
}

type fakeClickHouseBackend struct {
	queries   []string
	queryRows []json.RawMessage
	records   []UsageRecord
}

func (f *fakeClickHouseBackend) InsertUsageRecord(_ context.Context, record UsageRecord) error {
	f.records = append(f.records, record)
	return nil
}

func (f *fakeClickHouseBackend) QueryJSONEachRow(_ context.Context, query string, scan func(json.RawMessage) error) error {
	f.queries = append(f.queries, query)
	for _, row := range f.queryRows {
		if err := scan(row); err != nil {
			return err
		}
	}
	return nil
}
