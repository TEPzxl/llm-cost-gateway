package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/observability"
	"github.com/tep/llm-cost-gateway/internal/ratelimit"
	"github.com/tep/llm-cost-gateway/internal/store"
	"github.com/tep/llm-cost-gateway/internal/testutil"
	"go.uber.org/zap"
)

func TestGatewayChatCompletionsEndToEndWithMockProvider(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/chat/completions status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		Object    string    `json:"object"`
		Model     string    `json:"model"`
		RequestID uuid.UUID `json:"request_id"`
		Usage     struct {
			PromptTokens     int32 `json:"prompt_tokens"`
			CompletionTokens int32 `json:"completion_tokens"`
			TotalTokens      int32 `json:"total_tokens"`
		} `json:"usage"`
		Cost struct {
			Currency       string `json:"currency"`
			TotalCostMicro int64  `json:"total_cost_micro"`
		} `json:"cost"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode chat completion response: %v", err)
	}
	if body.Object != "chat.completion" || body.Model != "fast-chat" || body.RequestID == uuid.Nil {
		t.Fatalf("response = %+v", body)
	}
	if body.Usage.TotalTokens != 50 {
		t.Fatalf("total tokens = %d, want 50", body.Usage.TotalTokens)
	}
	if body.Cost.Currency != "USD" || body.Cost.TotalCostMicro != 8 {
		t.Fatalf("cost = %+v, want USD 8 micro", body.Cost)
	}

	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 1)
	assertAppTableCount(t, ctx, st, "cost_records", 1)
}

func TestGatewayChatCompletionsRejectsAuthFailures(t *testing.T) {
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})

	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing"},
		{name: "invalid", authorization: "Bearer llmgw_live_invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`))
			req.Header.Set("Content-Type", "application/json")
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
		})
	}

	revokeAPIKeyViaHTTP(t, router, apiKey.AdminToken, apiKey.ID, http.StatusOK)
	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	assertAppTableCount(t, context.Background(), st, "request_logs", 0)
}

func TestGatewayChatCompletionsStreamsWithMockProvider(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/chat/completions stream status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", contentType)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data: ") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("stream body = %q, want SSE data and [DONE]", body)
	}

	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 1)
	assertAppTableCount(t, ctx, st, "cost_records", 1)
}

func TestGatewayChatCompletionsStreamUsageMissingWritesRequestLogOnly(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeAppSSE(t, w, `{"choices":[{"delta":{"content":"hello"}}]}`)
		writeAppSSE(t, w, `[DONE]`)
	}))
	defer server.Close()

	provider := createProviderViaHTTP(t, router, apiKey.AdminToken, createProviderRequest{
		Name:      "stream-provider",
		Type:      "openai_compatible",
		BaseURL:   stringPtr(server.URL + "/v1"),
		APIKey:    stringPtr("provider-secret-key"),
		TimeoutMS: 1000,
	})
	model := createModelViaHTTP(t, router, apiKey.AdminToken, createModelRequest{
		ProviderID:                     provider.ID,
		ProviderModelName:              "stream-small",
		DisplayName:                    "Stream Small",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "stream-missing-usage",
		MatchModel: "stream-missing",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"stream-missing","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/chat/completions stream status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 0)
	assertAppTableCount(t, ctx, st, "cost_records", 0)
	assertAppRequestLogErrorCode(t, ctx, st, "usage_missing")
}

func TestGatewayChatCompletionsRouteNotFound(t *testing.T) {
	router, _, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"missing-model","messages":[{"role":"user","content":"hello"}]}`)
	assertGatewayError(t, rec, http.StatusNotFound, "route_not_found")
}

func TestGatewayChatCompletionsRateLimitedWritesRequestLog(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: false}})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	assertGatewayError(t, rec, http.StatusTooManyRequests, "rate_limit_exceeded")

	assertAppRequestLogStatus(t, ctx, st, "rate_limited")
}

func TestGatewayChatCompletionsBudgetBlockWritesRequestLog(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	createBudgetViaHTTP(t, router, apiKey.AdminToken, createBudgetRequest{
		Name:          "block-budget",
		ScopeType:     "org",
		Period:        "daily",
		LimitMicroUSD: 0,
		Action:        "block",
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	assertGatewayError(t, rec, http.StatusPaymentRequired, "budget_exceeded")

	assertAppRequestLogStatus(t, ctx, st, "budget_blocked")
}

func TestGatewayChatCompletionsBudgetWarnRecordsWarning(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	createBudgetViaHTTP(t, router, apiKey.AdminToken, createBudgetRequest{
		Name:          "warn-budget",
		ScopeType:     "org",
		Period:        "daily",
		LimitMicroUSD: 0,
		Action:        "warn",
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/chat/completions status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	assertAppRequestLogStatus(t, ctx, st, "budget_warned")
}

type fakeGatewayLimiter struct {
	decision ratelimit.Decision
	err      error
}

func (f fakeGatewayLimiter) Allow(context.Context, auth.APIKeyPrincipal) (ratelimit.Decision, error) {
	return f.decision, f.err
}

type gatewayChatFixture struct {
	OrgID      uuid.UUID
	ID         uuid.UUID
	Key        string
	AdminToken string
}

func newGatewayChatTestRouter(t *testing.T, limiter fakeGatewayLimiter) (*gin.Engine, *store.Store, gatewayChatFixture) {
	return newGatewayChatTestRouterWithMetrics(t, limiter, nil)
}

func newGatewayChatTestRouterWithMetrics(t *testing.T, limiter fakeGatewayLimiter, metrics *observability.Metrics) (*gin.Engine, *store.Store, gatewayChatFixture) {
	t.Helper()

	ctx := context.Background()
	pool := testutil.OpenPostgres(t, ctx)
	st := store.New(pool)
	resetTask5TestDatabase(t, ctx, st)

	router := NewRouter(RouterConfig{
		AppEnv:                 "test",
		PlatformBootstrapToken: "bootstrap-token",
		TokenHashSecret:        "token-hash-secret",
		SecretEncryptionKey:    "0123456789abcdef0123456789abcdef",
		Store:                  st,
		RateLimiter:            limiter,
		Metrics:                metrics,
	}, zap.NewNop())

	orgID := createOrgViaHTTP(t, router, "Gateway Org", "gateway-org-"+uuid.NewString())
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	apiKey := createAPIKeyViaHTTP(t, router, adminToken.Token, createAPIKeyRequest{
		Name:     "gateway-key",
		Scopes:   []string{"chat.completions"},
		RPMLimit: 60,
	})
	provider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "mock-provider",
		Type:      "mock",
		APIKey:    stringPtr("mock-key"),
		TimeoutMS: 30000,
	})
	model := createModelViaHTTP(t, router, adminToken.Token, createModelRequest{
		ProviderID:                     provider.ID,
		ProviderModelName:              "mock-small",
		DisplayName:                    "Mock Small",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
		ContextWindow:                  int32Ptr(8192),
	})
	createRoutePolicyViaHTTP(t, router, adminToken.Token, createRoutePolicyRequest{
		Name:       "fast-chat",
		MatchModel: "fast-chat",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	return router, st, gatewayChatFixture{
		OrgID:      orgID,
		ID:         apiKey.ID,
		Key:        apiKey.Key,
		AdminToken: adminToken.Token,
	}
}

func performChatCompletion(t *testing.T, router http.Handler, apiKey string, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertGatewayError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Error.Code != wantCode {
		t.Fatalf("error code = %q, want %q; body=%s", response.Error.Code, wantCode, rec.Body.String())
	}
}

func assertAppTableCount(t *testing.T, ctx context.Context, st *store.Store, table string, want int) {
	t.Helper()

	var got int
	if err := st.Pool.QueryRow(ctx, "SELECT count(*)::int FROM "+table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}

func assertAppRequestLogStatus(t *testing.T, ctx context.Context, st *store.Store, want string) {
	t.Helper()

	var got string
	if err := st.Pool.QueryRow(ctx, `SELECT status FROM request_logs ORDER BY started_at DESC LIMIT 1`).Scan(&got); err != nil {
		t.Fatalf("select latest request log status: %v", err)
	}
	if got != want {
		t.Fatalf("request log status = %q, want %q", got, want)
	}
}

func assertAppRequestLogErrorCode(t *testing.T, ctx context.Context, st *store.Store, want string) {
	t.Helper()

	var got string
	if err := st.Pool.QueryRow(ctx, `SELECT error_code FROM request_logs ORDER BY started_at DESC LIMIT 1`).Scan(&got); err != nil {
		t.Fatalf("select latest request log error_code: %v", err)
	}
	if got != want {
		t.Fatalf("request log error_code = %q, want %q", got, want)
	}
}

func writeAppSSE(t *testing.T, w http.ResponseWriter, data string) {
	t.Helper()
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		t.Fatalf("write SSE: %v", err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
