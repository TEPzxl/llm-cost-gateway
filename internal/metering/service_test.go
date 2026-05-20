package metering

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/costing"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestServiceRecordSuccessWritesRequestUsageAndCost(t *testing.T) {
	ctx := context.Background()
	st := openMeteringTestStore(t, ctx)
	resetMeteringTestDatabase(t, ctx, st)
	fixture := createMeteringFixture(t, ctx, st)
	service := NewService(st, costing.NewCalculator())

	requestID := uuid.New()
	result, err := service.RecordSuccess(ctx, RecordSuccessInput{
		RequestID:                      requestID,
		OrgID:                          fixture.OrgID,
		APIKeyID:                       fixture.APIKeyID,
		ProviderID:                     fixture.ProviderID,
		ModelID:                        fixture.ModelID,
		RoutePolicyID:                  &fixture.RoutePolicyID,
		Method:                         "POST",
		Path:                           "/v1/chat/completions",
		RequestModel:                   "fast-chat",
		StatusCode:                     200,
		RequestHash:                    "sha256:request-hash",
		ResponseHash:                   "sha256:response-hash",
		LatencyMS:                      321,
		ProviderLatencyMS:              int32Ptr(250),
		PromptTokens:                   20,
		CompletionTokens:               30,
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
		ProviderUsageJSON:              json.RawMessage(`{"prompt_tokens":20,"completion_tokens":30,"total_tokens":50}`),
		StartedAt:                      time.Now().UTC().Add(-time.Second),
		CompletedAt:                    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("RecordSuccess returned error: %v", err)
	}
	if result.RequestLog.ID != requestID {
		t.Fatalf("request log id = %s, want %s", result.RequestLog.ID, requestID)
	}
	if result.UsageRecord.RequestLogID != requestID {
		t.Fatalf("usage request_log_id = %s, want %s", result.UsageRecord.RequestLogID, requestID)
	}
	if result.CostRecord.TotalCostMicro != 8 {
		t.Fatalf("total cost = %d, want 8", result.CostRecord.TotalCostMicro)
	}

	var snapshot struct {
		InputPriceMicroUSDPer1KTokens  int64  `json:"input_price_micro_usd_per_1k_tokens"`
		OutputPriceMicroUSDPer1KTokens int64  `json:"output_price_micro_usd_per_1k_tokens"`
		RoundingMode                   string `json:"rounding_mode"`
	}
	if err := json.Unmarshal(result.CostRecord.PricingSnapshot, &snapshot); err != nil {
		t.Fatalf("decode pricing snapshot: %v", err)
	}
	if snapshot.InputPriceMicroUSDPer1KTokens != 100 || snapshot.OutputPriceMicroUSDPer1KTokens != 200 {
		t.Fatalf("pricing snapshot = %+v, want input=100 output=200", snapshot)
	}
	if snapshot.RoundingMode != costing.RoundingModeTruncateToMicroUSD {
		t.Fatalf("rounding mode = %q, want %q", snapshot.RoundingMode, costing.RoundingModeTruncateToMicroUSD)
	}

	assertTableCount(t, ctx, st, "request_logs", 1)
	assertTableCount(t, ctx, st, "usage_records", 1)
	assertTableCount(t, ctx, st, "cost_records", 1)
}

func TestServiceRecordFailureWritesOnlyRequestLog(t *testing.T) {
	ctx := context.Background()
	st := openMeteringTestStore(t, ctx)
	resetMeteringTestDatabase(t, ctx, st)
	fixture := createMeteringFixture(t, ctx, st)
	service := NewService(st, costing.NewCalculator())

	requestID := uuid.New()
	requestLog, err := service.RecordFailure(ctx, RecordFailureInput{
		RequestID:    requestID,
		OrgID:        fixture.OrgID,
		APIKeyID:     &fixture.APIKeyID,
		Method:       "POST",
		Path:         "/v1/chat/completions",
		RequestModel: "fast-chat",
		Status:       StatusError,
		StatusCode:   502,
		ErrorCode:    "provider_error",
		RequestHash:  "sha256:request-hash",
		LatencyMS:    100,
		StartedAt:    time.Now().UTC().Add(-time.Second),
		CompletedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("RecordFailure returned error: %v", err)
	}
	if requestLog.ID != requestID {
		t.Fatalf("request log id = %s, want %s", requestLog.ID, requestID)
	}
	if requestLog.Status != StatusError {
		t.Fatalf("request log status = %q, want %q", requestLog.Status, StatusError)
	}

	assertTableCount(t, ctx, st, "request_logs", 1)
	assertTableCount(t, ctx, st, "usage_records", 0)
	assertTableCount(t, ctx, st, "cost_records", 0)
}

func TestServiceRecordSuccessRollsBackWhenCostingFails(t *testing.T) {
	ctx := context.Background()
	st := openMeteringTestStore(t, ctx)
	resetMeteringTestDatabase(t, ctx, st)
	fixture := createMeteringFixture(t, ctx, st)
	service := NewService(st, costing.NewCalculator())

	_, err := service.RecordSuccess(ctx, RecordSuccessInput{
		RequestID:                      uuid.New(),
		OrgID:                          fixture.OrgID,
		APIKeyID:                       fixture.APIKeyID,
		ProviderID:                     fixture.ProviderID,
		ModelID:                        fixture.ModelID,
		Method:                         "POST",
		Path:                           "/v1/chat/completions",
		RequestModel:                   "fast-chat",
		StatusCode:                     200,
		RequestHash:                    "sha256:request-hash",
		ResponseHash:                   "sha256:response-hash",
		LatencyMS:                      321,
		PromptTokens:                   2000,
		CompletionTokens:               0,
		InputPriceMicroUSDPer1KTokens:  math.MaxInt64,
		OutputPriceMicroUSDPer1KTokens: 0,
		StartedAt:                      time.Now().UTC().Add(-time.Second),
		CompletedAt:                    time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("RecordSuccess returned nil error, want costing overflow")
	}

	assertTableCount(t, ctx, st, "request_logs", 0)
	assertTableCount(t, ctx, st, "usage_records", 0)
	assertTableCount(t, ctx, st, "cost_records", 0)
}

func TestServiceDoesNotPersistPromptOrResponseContent(t *testing.T) {
	ctx := context.Background()
	st := openMeteringTestStore(t, ctx)
	resetMeteringTestDatabase(t, ctx, st)
	fixture := createMeteringFixture(t, ctx, st)
	service := NewService(st, costing.NewCalculator())

	rawPrompt := "secret prompt text"
	rawResponse := "secret response text"
	if _, err := service.RecordSuccess(ctx, RecordSuccessInput{
		RequestID:                      uuid.New(),
		OrgID:                          fixture.OrgID,
		APIKeyID:                       fixture.APIKeyID,
		ProviderID:                     fixture.ProviderID,
		ModelID:                        fixture.ModelID,
		Method:                         "POST",
		Path:                           "/v1/chat/completions",
		RequestModel:                   "fast-chat",
		StatusCode:                     200,
		RequestHash:                    "sha256:not-raw-prompt",
		ResponseHash:                   "sha256:not-raw-response",
		LatencyMS:                      100,
		PromptTokens:                   1,
		CompletionTokens:               1,
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
		ProviderUsageJSON:              json.RawMessage(`{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}`),
		StartedAt:                      time.Now().UTC().Add(-time.Second),
		CompletedAt:                    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("RecordSuccess returned error: %v", err)
	}

	var found int
	if err := st.Pool.QueryRow(ctx, `
		SELECT count(*)::int
		FROM request_logs
		WHERE request_hash LIKE '%' || $1 || '%'
		   OR response_hash LIKE '%' || $2 || '%'
		   OR COALESCE(metadata::text, '') LIKE '%' || $1 || '%'
		   OR COALESCE(metadata::text, '') LIKE '%' || $2 || '%'
	`, rawPrompt, rawResponse).Scan(&found); err != nil {
		t.Fatalf("search request_logs for raw content: %v", err)
	}
	if found != 0 {
		t.Fatalf("found %d rows containing raw prompt/response content", found)
	}
}

type meteringFixture struct {
	OrgID         uuid.UUID
	APIKeyID      uuid.UUID
	ProviderID    uuid.UUID
	ModelID       uuid.UUID
	RoutePolicyID uuid.UUID
}

func openMeteringTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()

	pool := testutil.OpenPostgres(t, ctx)
	return store.New(pool)
}

func resetMeteringTestDatabase(t *testing.T, ctx context.Context, st *store.Store) {
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

func createMeteringFixture(t *testing.T, ctx context.Context, st *store.Store) meteringFixture {
	t.Helper()

	now := time.Now().UTC()
	org, err := st.Queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		ID:        uuid.New(),
		Name:      "metering-org",
		Slug:      "metering-org-" + uuid.NewString(),
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
		Name:        "metering-key",
		KeyPrefix:   "llmgw_live_test",
		KeyHash:     "metering-key-hash-" + uuid.NewString(),
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
		Name:      "mock-provider-" + uuid.NewString(),
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
		ProviderModelName:              "mock-small-" + uuid.NewString(),
		DisplayName:                    "Mock Small",
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
	policy, err := st.Queries.CreateRoutePolicy(ctx, db.CreateRoutePolicyParams{
		ID:         uuid.New(),
		OrgID:      org.ID,
		Name:       "metering-route-policy-" + uuid.NewString(),
		MatchModel: "fast-chat-" + uuid.NewString(),
		Strategy:   "single",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreateRoutePolicy returned error: %v", err)
	}

	return meteringFixture{
		OrgID:         org.ID,
		APIKeyID:      apiKey.ID,
		ProviderID:    provider.ID,
		ModelID:       model.ID,
		RoutePolicyID: policy.ID,
	}
}

func assertTableCount(t *testing.T, ctx context.Context, st *store.Store, table string, want int) {
	t.Helper()

	var got int
	if err := st.Pool.QueryRow(ctx, "SELECT count(*)::int FROM "+table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}

func int32Ptr(value int32) *int32 {
	return &value
}
