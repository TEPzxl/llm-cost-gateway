package metering

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/costing"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/events"
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
	if result.CostRecord.PricingVersionID == nil || *result.CostRecord.PricingVersionID != fixture.PricingVersionID {
		t.Fatalf("pricing_version_id = %v, want %s", result.CostRecord.PricingVersionID, fixture.PricingVersionID)
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

func TestServiceRecordSuccessPublishesUsageEvent(t *testing.T) {
	ctx := context.Background()
	st := openMeteringTestStore(t, ctx)
	resetMeteringTestDatabase(t, ctx, st)
	fixture := createMeteringFixture(t, ctx, st)
	publisher := &capturingPublisher{}
	service := NewService(st, costing.NewCalculator(), WithUsageEventPublisher(publisher))

	requestID := uuid.New()
	_, err := service.RecordSuccess(ctx, RecordSuccessInput{
		RequestID:                      requestID,
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
		LatencyMS:                      100,
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
	if len(publisher.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(publisher.events))
	}
	event := publisher.events[0]
	if event.Type != events.EventUsageRecorded {
		t.Fatalf("event type = %q, want %q", event.Type, events.EventUsageRecorded)
	}
	if event.OrgID != fixture.OrgID || event.RequestID != requestID {
		t.Fatalf("event ids = org %s request %s, want org %s request %s", event.OrgID, event.RequestID, fixture.OrgID, requestID)
	}
	if event.APIKeyID == nil || *event.APIKeyID != fixture.APIKeyID {
		t.Fatalf("api_key_id = %v, want %s", event.APIKeyID, fixture.APIKeyID)
	}
	if event.ProviderID == nil || *event.ProviderID != fixture.ProviderID {
		t.Fatalf("provider_id = %v, want %s", event.ProviderID, fixture.ProviderID)
	}
	if event.ModelID == nil || *event.ModelID != fixture.ModelID {
		t.Fatalf("model_id = %v, want %s", event.ModelID, fixture.ModelID)
	}
	if event.PromptTokens != 20 || event.CompletionTokens != 30 || event.TotalTokens != 50 {
		t.Fatalf("event tokens = %+v, want prompt=20 completion=30 total=50", event)
	}
	if event.TotalCostMicroUSD != 8 {
		t.Fatalf("event total cost = %d, want 8", event.TotalCostMicroUSD)
	}
	if event.Status != StatusSuccess {
		t.Fatalf("event status = %q, want %q", event.Status, StatusSuccess)
	}
}

func TestServicePublishFailureDoesNotRollbackSuccess(t *testing.T) {
	ctx := context.Background()
	st := openMeteringTestStore(t, ctx)
	resetMeteringTestDatabase(t, ctx, st)
	fixture := createMeteringFixture(t, ctx, st)
	service := NewService(st, costing.NewCalculator(), WithUsageEventPublisher(failingPublisher{}))

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
		LatencyMS:                      100,
		PromptTokens:                   20,
		CompletionTokens:               30,
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
		StartedAt:                      time.Now().UTC().Add(-time.Second),
		CompletedAt:                    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("RecordSuccess returned error: %v", err)
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

func TestServiceRecordBudgetBlockPublishesBlockedEvent(t *testing.T) {
	ctx := context.Background()
	st := openMeteringTestStore(t, ctx)
	resetMeteringTestDatabase(t, ctx, st)
	fixture := createMeteringFixture(t, ctx, st)
	publisher := &capturingPublisher{}
	service := NewService(st, costing.NewCalculator(), WithUsageEventPublisher(publisher))

	requestID := uuid.New()
	_, err := service.RecordFailure(ctx, RecordFailureInput{
		RequestID:    requestID,
		OrgID:        fixture.OrgID,
		APIKeyID:     &fixture.APIKeyID,
		ProviderID:   &fixture.ProviderID,
		ModelID:      &fixture.ModelID,
		Method:       "POST",
		Path:         "/v1/chat/completions",
		RequestModel: "fast-chat",
		Status:       StatusBudgetBlocked,
		StatusCode:   402,
		ErrorCode:    domain.CodeBudgetExceeded,
		RequestHash:  "sha256:request-hash",
		LatencyMS:    100,
		StartedAt:    time.Now().UTC().Add(-time.Second),
		CompletedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("RecordFailure returned error: %v", err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(publisher.events))
	}
	event := publisher.events[0]
	if event.Type != events.EventRequestBlocked {
		t.Fatalf("event type = %q, want %q", event.Type, events.EventRequestBlocked)
	}
	if event.OrgID != fixture.OrgID || event.RequestID != requestID {
		t.Fatalf("event ids = org %s request %s, want org %s request %s", event.OrgID, event.RequestID, fixture.OrgID, requestID)
	}
	if event.APIKeyID == nil || *event.APIKeyID != fixture.APIKeyID {
		t.Fatalf("api_key_id = %v, want %s", event.APIKeyID, fixture.APIKeyID)
	}
	if event.ProviderID == nil || *event.ProviderID != fixture.ProviderID {
		t.Fatalf("provider_id = %v, want %s", event.ProviderID, fixture.ProviderID)
	}
	if event.ModelID == nil || *event.ModelID != fixture.ModelID {
		t.Fatalf("model_id = %v, want %s", event.ModelID, fixture.ModelID)
	}
	if event.TotalTokens != 0 || event.TotalCostMicroUSD != 0 {
		t.Fatalf("event totals = tokens %d cost %d, want zero", event.TotalTokens, event.TotalCostMicroUSD)
	}
	if event.Status != StatusBudgetBlocked {
		t.Fatalf("event status = %q, want %q", event.Status, StatusBudgetBlocked)
	}
}

func TestServiceRecordSuccessRollsBackWhenCostingFails(t *testing.T) {
	ctx := context.Background()
	st := openMeteringTestStore(t, ctx)
	resetMeteringTestDatabase(t, ctx, st)
	fixture := createMeteringFixture(t, ctx, st)
	service := NewService(st, costing.NewCalculator())
	if _, err := st.Pool.Exec(ctx, `
		UPDATE model_pricing_versions
		SET input_price_micro_usd_per_1k_tokens = $2,
		    output_price_micro_usd_per_1k_tokens = 0
		WHERE id = $1
	`, fixture.PricingVersionID, int64(math.MaxInt64)); err != nil {
		t.Fatalf("update pricing version to overflow price: %v", err)
	}

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

func TestServiceUsageEventDoesNotContainSensitiveContent(t *testing.T) {
	ctx := context.Background()
	st := openMeteringTestStore(t, ctx)
	resetMeteringTestDatabase(t, ctx, st)
	fixture := createMeteringFixture(t, ctx, st)
	publisher := &capturingPublisher{}
	service := NewService(st, costing.NewCalculator(), WithUsageEventPublisher(publisher))

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

	if len(publisher.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(publisher.events))
	}
	body, err := json.Marshal(publisher.events[0])
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	payload := strings.ToLower(string(body))
	for _, secret := range []string{rawPrompt, rawResponse, "llmgw_live_", "authorization"} {
		if strings.Contains(payload, strings.ToLower(secret)) {
			t.Fatalf("event contains sensitive value %q: %s", secret, payload)
		}
	}
}

type meteringFixture struct {
	OrgID            uuid.UUID
	APIKeyID         uuid.UUID
	ProviderID       uuid.UUID
	ModelID          uuid.UUID
	RoutePolicyID    uuid.UUID
	PricingVersionID uuid.UUID
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
	pricingVersion, err := st.Queries.CreateModelPricingVersion(ctx, db.CreateModelPricingVersionParams{
		ID:                             uuid.New(),
		OrgID:                          org.ID,
		ModelID:                        model.ID,
		Version:                        1,
		InputPriceMicroUsdPer1kTokens:  model.InputPriceMicroUsdPer1kTokens,
		OutputPriceMicroUsdPer1kTokens: model.OutputPriceMicroUsdPer1kTokens,
		Status:                         "active",
		EffectiveFrom:                  now,
		CreatedAt:                      now,
	})
	if err != nil {
		t.Fatalf("CreateModelPricingVersion returned error: %v", err)
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
		OrgID:            org.ID,
		APIKeyID:         apiKey.ID,
		ProviderID:       provider.ID,
		ModelID:          model.ID,
		RoutePolicyID:    policy.ID,
		PricingVersionID: pricingVersion.ID,
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

type capturingPublisher struct {
	events []events.UsageEvent
}

func (p *capturingPublisher) Publish(_ context.Context, event events.UsageEvent) error {
	p.events = append(p.events, event)
	return nil
}

type failingPublisher struct{}

func (failingPublisher) Publish(context.Context, events.UsageEvent) error {
	return errors.New("publish failed")
}
