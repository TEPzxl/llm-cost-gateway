package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/auth"
	promptcache "github.com/tep/llm-cost-gateway/internal/cache"
	"github.com/tep/llm-cost-gateway/internal/embedding"
	"github.com/tep/llm-cost-gateway/internal/observability"
	"github.com/tep/llm-cost-gateway/internal/ratelimit"
	"github.com/tep/llm-cost-gateway/internal/store"
	"github.com/tep/llm-cost-gateway/internal/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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

func TestGatewayChatCompletionsFallbackRetriesNextTarget(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"temporary failure"}}`))
	}))
	defer failingServer.Close()

	failingProvider, failingModel := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "fallback-failing", failingServer.URL, 1)
	successProvider := createProviderViaHTTP(t, router, apiKey.AdminToken, createProviderRequest{
		Name:      "fallback-mock-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	successModel := createModelViaHTTP(t, router, apiKey.AdminToken, createModelRequest{
		ProviderID:                     successProvider.ID,
		ProviderModelName:              "mock-fallback-success",
		DisplayName:                    "Mock Fallback Success",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "fallback-success-policy",
		MatchModel: "fallback-success",
		Strategy:   "fallback",
		Targets: []createRouteTargetRequest{
			{ProviderID: failingProvider.ID, ModelID: failingModel.ID, Priority: 1, Weight: 100},
			{ProviderID: successProvider.ID, ModelID: successModel.ID, Priority: 2, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fallback-success","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("fallback chat status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 1)
	assertAppTableCount(t, ctx, st, "cost_records", 1)
	assertAppRequestLogProvider(t, ctx, st, successProvider.ID)
	assertAppRequestLogFallbackCount(t, ctx, st, 1)
}

func TestGatewayChatCompletionsFallbackStopsOnNonRetryableError(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	unauthorizedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer unauthorizedServer.Close()

	firstProvider, firstModel := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "fallback-unauthorized", unauthorizedServer.URL, 1)
	secondProvider := createProviderViaHTTP(t, router, apiKey.AdminToken, createProviderRequest{
		Name:      "fallback-should-not-run",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	secondModel := createModelViaHTTP(t, router, apiKey.AdminToken, createModelRequest{
		ProviderID:                     secondProvider.ID,
		ProviderModelName:              "mock-should-not-run",
		DisplayName:                    "Mock Should Not Run",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "fallback-non-retryable-policy",
		MatchModel: "fallback-non-retryable",
		Strategy:   "fallback",
		Targets: []createRouteTargetRequest{
			{ProviderID: firstProvider.ID, ModelID: firstModel.ID, Priority: 1, Weight: 100},
			{ProviderID: secondProvider.ID, ModelID: secondModel.ID, Priority: 2, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fallback-non-retryable","messages":[{"role":"user","content":"hello"}]}`)
	assertGatewayError(t, rec, http.StatusBadGateway, "provider_error")

	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 0)
	assertAppTableCount(t, ctx, st, "cost_records", 0)
	assertAppRequestLogProvider(t, ctx, st, firstProvider.ID)
	assertAppRequestLogFallbackCount(t, ctx, st, 0)
}

func TestGatewayChatCompletionsFallbackAllTargetsFail(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer serverB.Close()

	providerA, modelA := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "fallback-fail-a", serverA.URL, 1)
	providerB, modelB := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "fallback-fail-b", serverB.URL, 2)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "fallback-all-fail-policy",
		MatchModel: "fallback-all-fail",
		Strategy:   "fallback",
		Targets: []createRouteTargetRequest{
			{ProviderID: providerA.ID, ModelID: modelA.ID, Priority: 1, Weight: 100},
			{ProviderID: providerB.ID, ModelID: modelB.ID, Priority: 2, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fallback-all-fail","messages":[{"role":"user","content":"hello"}]}`)
	assertGatewayError(t, rec, http.StatusServiceUnavailable, "provider_unavailable")

	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppRequestLogProvider(t, ctx, st, providerB.ID)
	assertAppRequestLogFallbackCount(t, ctx, st, 1)
}

func TestGatewayChatCompletionsLowestCostSelectsCheapestTarget(t *testing.T) {
	router, _, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	expensiveProvider := createProviderViaHTTP(t, router, apiKey.AdminToken, createProviderRequest{
		Name:      "expensive-provider",
		Type:      "mock",
		APIKey:    stringPtr("mock-key"),
		TimeoutMS: 30000,
	})
	expensiveModel := createModelViaHTTP(t, router, apiKey.AdminToken, createModelRequest{
		ProviderID:                     expensiveProvider.ID,
		ProviderModelName:              "expensive-model",
		DisplayName:                    "Expensive Model",
		InputPriceMicroUSDPer1KTokens:  1000,
		OutputPriceMicroUSDPer1KTokens: 1000,
	})
	cheapProvider := createProviderViaHTTP(t, router, apiKey.AdminToken, createProviderRequest{
		Name:      "cheap-provider",
		Type:      "mock",
		APIKey:    stringPtr("mock-key"),
		TimeoutMS: 30000,
	})
	cheapModel := createModelViaHTTP(t, router, apiKey.AdminToken, createModelRequest{
		ProviderID:                     cheapProvider.ID,
		ProviderModelName:              "cheap-model",
		DisplayName:                    "Cheap Model",
		InputPriceMicroUSDPer1KTokens:  10,
		OutputPriceMicroUSDPer1KTokens: 10,
	})
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "cost-chat",
		MatchModel: "cost-chat",
		Strategy:   "lowest_cost",
		Config:     json.RawMessage(`{"fallback_to_priority":true}`),
		Targets: []createRouteTargetRequest{
			{ProviderID: expensiveProvider.ID, ModelID: expensiveModel.ID, Priority: 1, Weight: 100},
			{ProviderID: cheapProvider.ID, ModelID: cheapModel.ID, Priority: 2, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"cost-chat","messages":[{"role":"user","content":"hello world"}],"max_tokens":100}`)
	body := decodeGatewayChatResponse(t, rec)
	if body.Provider.ID != cheapProvider.ID {
		t.Fatalf("provider id = %s, want cheapest provider %s", body.Provider.ID, cheapProvider.ID)
	}
}

func TestGatewayChatCompletionsRetriesRetryableProviderError(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"temporary failure"}}`))
			return
		}
		writeAppOpenAIChatResponse(t, w)
	}))
	defer server.Close()

	provider, model := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "retry-success", server.URL, 100)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "retry-success-policy",
		MatchModel: "retry-success",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"retry-success","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("retry chat status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want 2", got)
	}
	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 1)
	assertAppTableCount(t, ctx, st, "cost_records", 1)
	assertAppRequestLogProvider(t, ctx, st, provider.ID)
	assertAppRequestLogTargetAttempts(t, ctx, st, 2)
}

func TestGatewayChatCompletionsDoesNotRetryNonRetryableProviderError(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer server.Close()

	provider, model := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "retry-unauthorized", server.URL, 100)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "retry-unauthorized-policy",
		MatchModel: "retry-unauthorized",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"retry-unauthorized","messages":[{"role":"user","content":"hello"}]}`)
	assertGatewayError(t, rec, http.StatusBadGateway, "provider_error")

	if got := calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}
	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 0)
	assertAppTableCount(t, ctx, st, "cost_records", 0)
	assertAppRequestLogProvider(t, ctx, st, provider.ID)
}

func TestGatewayChatCompletionsRetriesProviderTimeout(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		time.Sleep(40 * time.Millisecond)
		writeAppOpenAIChatResponse(t, w)
	}))
	defer server.Close()

	provider := createProviderViaHTTP(t, router, apiKey.AdminToken, createProviderRequest{
		Name:      "retry-timeout-provider",
		Type:      "openai_compatible",
		BaseURL:   stringPtr(server.URL + "/v1"),
		APIKey:    stringPtr("provider-secret-key"),
		TimeoutMS: 5,
	})
	model := createModelViaHTTP(t, router, apiKey.AdminToken, createModelRequest{
		ProviderID:                     provider.ID,
		ProviderModelName:              "retry-timeout-model",
		DisplayName:                    "Retry Timeout Model",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 100,
	})
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "retry-timeout-policy",
		MatchModel: "retry-timeout",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"retry-timeout","messages":[{"role":"user","content":"hello"}]}`)
	assertGatewayError(t, rec, http.StatusGatewayTimeout, "provider_timeout")

	if got := calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want 2", got)
	}
	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 0)
	assertAppTableCount(t, ctx, st, "cost_records", 0)
	assertAppRequestLogProvider(t, ctx, st, provider.ID)
	assertAppRequestLogTargetAttempts(t, ctx, st, 2)
}

func TestGatewayChatCompletionsTriggersBudgetAlertWithoutBlockingResponse(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	createBudgetViaHTTP(t, router, apiKey.AdminToken, createBudgetRequest{
		Name:          "gateway-alert-budget",
		ScopeType:     "org",
		Period:        "monthly",
		LimitMicroUSD: 8,
		Action:        "warn",
	})
	budgets := listBudgetsViaHTTP(t, router, apiKey.AdminToken)
	if len(budgets.Items) != 1 {
		t.Fatalf("budget count = %d, want 1", len(budgets.Items))
	}
	createBudgetAlertViaHTTP(t, router, apiKey.AdminToken, createBudgetAlertRequest{
		BudgetID:   budgets.Items[0].ID,
		WebhookURL: server.URL,
		Status:     "active",
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if got := calls.Load(); got != 3 {
		t.Fatalf("webhook calls = %d, want 3 thresholds", got)
	}
	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 1)
	assertAppTableCount(t, ctx, st, "cost_records", 1)
	assertAppBudgetAlertDeliveryCount(t, ctx, st, 3)
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

func TestGatewayChatCompletionsAPIKeyQuotaBlockWritesRequestLog(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	blockedKey := createAPIKeyViaHTTP(t, router, apiKey.AdminToken, createAPIKeyRequest{
		Name:                   "blocked-quota-key",
		Scopes:                 []string{"chat.completions"},
		RPMLimit:               60,
		DailyCostLimitMicroUSD: int64Ptr(0),
		QuotaAction:            "block",
	})

	rec := performChatCompletion(t, router, blockedKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	assertGatewayError(t, rec, http.StatusPaymentRequired, "api_key_quota_exceeded")

	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 0)
	assertAppTableCount(t, ctx, st, "cost_records", 0)
	assertAppRequestLogStatus(t, ctx, st, "budget_blocked")
	assertAppRequestLogErrorCode(t, ctx, st, "api_key_quota_exceeded")
	assertAppRequestLogAPIKeyQuota(t, ctx, st, "daily", "block", true)
}

func TestGatewayChatCompletionsAPIKeyQuotaWarnRecordsMetadata(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	warnKey := createAPIKeyViaHTTP(t, router, apiKey.AdminToken, createAPIKeyRequest{
		Name:                   "warn-quota-key",
		Scopes:                 []string{"chat.completions"},
		RPMLimit:               60,
		DailyCostLimitMicroUSD: int64Ptr(0),
		QuotaAction:            "warn",
	})

	rec := performChatCompletion(t, router, warnKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/chat/completions status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 1)
	assertAppTableCount(t, ctx, st, "cost_records", 1)
	assertAppRequestLogStatus(t, ctx, st, "success")
	assertAppRequestLogAPIKeyQuota(t, ctx, st, "daily", "warn", true)
}

func TestGatewayChatCompletionsUsesLatestPricingVersionAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	models, err := st.Queries.ListModels(ctx, apiKey.OrgID)
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("model count = %d, want 1", len(models))
	}

	first := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	firstBody := decodeGatewayChatResponse(t, first)
	if firstBody.Cost.TotalCostMicro != 8 {
		t.Fatalf("first cost = %d, want 8", firstBody.Cost.TotalCostMicro)
	}
	firstCost := readAppCostPricingVersion(t, ctx, st, firstBody.RequestID)
	if firstCost.totalCostMicro != 8 || firstCost.version != 1 {
		t.Fatalf("first cost record = %+v, want cost 8 version 1", firstCost)
	}

	updateModelPricingViaHTTP(t, router, apiKey.AdminToken, models[0].ID, updateModelPricingRequest{
		InputPriceMicroUSDPer1KTokens:  1000,
		OutputPriceMicroUSDPer1KTokens: 1000,
	}, http.StatusOK)

	second := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	secondBody := decodeGatewayChatResponse(t, second)
	if secondBody.Cost.TotalCostMicro != 50 {
		t.Fatalf("second cost = %d, want 50", secondBody.Cost.TotalCostMicro)
	}
	secondCost := readAppCostPricingVersion(t, ctx, st, secondBody.RequestID)
	if secondCost.totalCostMicro != 50 || secondCost.version != 2 {
		t.Fatalf("second cost record = %+v, want cost 50 version 2", secondCost)
	}

	firstCostAgain := readAppCostPricingVersion(t, ctx, st, firstBody.RequestID)
	if firstCostAgain.totalCostMicro != 8 || firstCostAgain.version != 1 || firstCostAgain.pricingVersionID != firstCost.pricingVersionID {
		t.Fatalf("first cost record after price update = %+v, want unchanged cost 8 version 1 id %s", firstCostAgain, firstCost.pricingVersionID)
	}
}

func TestGatewayChatCompletionsExactCacheHitSkipsProvider(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouterWithPromptCache(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeAppOpenAIChatResponse(t, w)
	}))
	defer server.Close()

	provider, model := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "cache-chat", server.URL, 1000)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "cache-chat-policy",
		MatchModel: "cache-chat",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})
	body := `{"model":"cache-chat","messages":[{"role":"user","content":"cache me"}],"temperature":0.1}`

	first := decodeGatewayChatResponse(t, performChatCompletion(t, router, apiKey.Key, body))
	if first.Cost.TotalCostMicro != 5 {
		t.Fatalf("first cost = %d, want 5", first.Cost.TotalCostMicro)
	}
	second := decodeGatewayChatResponse(t, performChatCompletion(t, router, apiKey.Key, body))
	if second.Cost.TotalCostMicro != 0 {
		t.Fatalf("second cost = %d, want cache hit cost 0", second.Cost.TotalCostMicro)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}
	assertAppTableCount(t, ctx, st, "request_logs", 2)
	assertAppTableCount(t, ctx, st, "usage_records", 1)
	assertAppTableCount(t, ctx, st, "cost_records", 1)
	assertAppCacheEventCount(t, ctx, st, "miss", 1)
	assertAppCacheEventCount(t, ctx, st, "store", 1)
	assertAppCacheEventCount(t, ctx, st, "hit", 1)
	assertAppLatestRequestLogCacheStatus(t, ctx, st, "hit")
	assertCacheEventsRouteListsEvents(t, router, apiKey.AdminToken, "hit")
}

func TestGatewayChatCompletionsExactCacheTemperatureMiss(t *testing.T) {
	router, _, apiKey := newGatewayChatTestRouterWithPromptCache(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeAppOpenAIChatResponse(t, w)
	}))
	defer server.Close()

	provider, model := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "cache-temp", server.URL, 1000)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "cache-temp-policy",
		MatchModel: "cache-temp",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	_ = decodeGatewayChatResponse(t, performChatCompletion(t, router, apiKey.Key, `{"model":"cache-temp","messages":[{"role":"user","content":"cache me"}],"temperature":0.1}`))
	_ = decodeGatewayChatResponse(t, performChatCompletion(t, router, apiKey.Key, `{"model":"cache-temp","messages":[{"role":"user","content":"cache me"}],"temperature":0.2}`))
	if got := calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want 2 for different temperature", got)
	}
}

func TestGatewayChatCompletionsStreamSkipsExactCache(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouterWithPromptCache(t)

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("stream status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	assertAppCacheEventCount(t, ctx, st, "skip", 1)
}

func TestGatewayChatCompletionsSemanticCacheHitSkipsProvider(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouterWithSemanticCache(t, 0.99, 0.3)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeAppOpenAIChatResponse(t, w)
	}))
	defer server.Close()

	provider, model := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "semantic-chat", server.URL, 1000)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "semantic-chat-policy",
		MatchModel: "semantic-chat",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	_ = decodeGatewayChatResponse(t, performChatCompletion(t, router, apiKey.Key, `{"model":"semantic-chat","messages":[{"role":"user","content":"Hello, world!"}],"temperature":0.1}`))
	second := decodeGatewayChatResponse(t, performChatCompletion(t, router, apiKey.Key, `{"model":"semantic-chat","messages":[{"role":"user","content":"hello world"}],"temperature":0.1}`))
	if second.Cost.TotalCostMicro != 0 {
		t.Fatalf("second cost = %d, want semantic cache hit cost 0", second.Cost.TotalCostMicro)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}
	assertAppTableCount(t, ctx, st, "request_logs", 2)
	assertAppTableCount(t, ctx, st, "usage_records", 1)
	assertAppTableCount(t, ctx, st, "cost_records", 1)
	assertAppCacheEventCount(t, ctx, st, "semantic_miss", 1)
	assertAppCacheEventCount(t, ctx, st, "semantic_store", 1)
	assertAppCacheEventCount(t, ctx, st, "semantic_hit", 1)
	assertAppLatestRequestLogCacheStatus(t, ctx, st, "semantic_hit")
}

func TestGatewayChatCompletionsSemanticCacheDissimilarMiss(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouterWithSemanticCache(t, 0.9, 0.3)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeAppOpenAIChatResponse(t, w)
	}))
	defer server.Close()

	provider, model := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "semantic-miss", server.URL, 1000)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "semantic-miss-policy",
		MatchModel: "semantic-miss",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	_ = decodeGatewayChatResponse(t, performChatCompletion(t, router, apiKey.Key, `{"model":"semantic-miss","messages":[{"role":"user","content":"hello world"}],"temperature":0.1}`))
	_ = decodeGatewayChatResponse(t, performChatCompletion(t, router, apiKey.Key, `{"model":"semantic-miss","messages":[{"role":"user","content":"goodbye mars"}],"temperature":0.1}`))
	if got := calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want 2 for dissimilar requests", got)
	}
	assertAppCacheEventCount(t, ctx, st, "semantic_miss", 2)
	assertAppCacheEventCount(t, ctx, st, "semantic_hit", 0)
}

func TestGatewayChatCompletionsSemanticCacheSkipsHighTemperature(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouterWithSemanticCache(t, 0.9, 0.3)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeAppOpenAIChatResponse(t, w)
	}))
	defer server.Close()

	provider, model := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "semantic-temp", server.URL, 1000)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "semantic-temp-policy",
		MatchModel: "semantic-temp",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	_ = decodeGatewayChatResponse(t, performChatCompletion(t, router, apiKey.Key, `{"model":"semantic-temp","messages":[{"role":"user","content":"hello world"}],"temperature":0.9}`))
	_ = decodeGatewayChatResponse(t, performChatCompletion(t, router, apiKey.Key, `{"model":"semantic-temp","messages":[{"role":"user","content":"hello world"}],"temperature":0.9}`))
	if got := calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want 2 for high temperature skip", got)
	}
	assertAppCacheEventCount(t, ctx, st, "semantic_skip", 2)
}

func TestGatewayChatCompletionsPIIRedactsBeforeProvider(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	createContentPolicyViaHTTP(t, router, apiKey.AdminToken, createContentPolicyRequest{
		Name:      "redact-pii",
		PIIAction: "redact",
	})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var providerRequest struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&providerRequest); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		if len(providerRequest.Messages) != 1 {
			t.Fatalf("provider messages count = %d, want 1", len(providerRequest.Messages))
		}
		content := providerRequest.Messages[0].Content
		if strings.Contains(content, "alice@example.com") {
			t.Fatalf("provider request contains raw PII: %q", content)
		}
		if !strings.Contains(content, "[REDACTED:email]") {
			t.Fatalf("provider request content = %q, want redacted email", content)
		}
		writeAppOpenAIChatResponse(t, w)
	}))
	defer server.Close()

	provider, model := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "pii-redact", server.URL, 100)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "pii-redact-policy",
		MatchModel: "pii-redact",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"pii-redact","messages":[{"role":"user","content":"email alice@example.com please"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PII redact chat status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}
	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 1)
	assertAppTableCount(t, ctx, st, "cost_records", 1)
	assertAppRequestLogContentPolicy(t, ctx, st, "redact", "email")
	assertAppRequestLogMetadataOmits(t, ctx, st, "alice@example.com")
}

func TestGatewayChatCompletionsPIIBlockWritesRequestLog(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	createContentPolicyViaHTTP(t, router, apiKey.AdminToken, createContentPolicyRequest{
		Name:      "block-pii",
		PIIAction: "block",
	})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeAppOpenAIChatResponse(t, w)
	}))
	defer server.Close()

	provider, model := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "pii-block", server.URL, 100)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "pii-block-policy",
		MatchModel: "pii-block",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"pii-block","messages":[{"role":"user","content":"email bob@example.com please"}]}`)
	assertGatewayError(t, rec, http.StatusBadRequest, "content_policy_blocked")

	if got := calls.Load(); got != 0 {
		t.Fatalf("provider calls = %d, want 0", got)
	}
	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppTableCount(t, ctx, st, "usage_records", 0)
	assertAppTableCount(t, ctx, st, "cost_records", 0)
	assertAppRequestLogStatus(t, ctx, st, "error")
	assertAppRequestLogErrorCode(t, ctx, st, "content_policy_blocked")
	assertAppRequestLogContentPolicy(t, ctx, st, "block", "email")
	assertAppRequestLogMetadataOmits(t, ctx, st, "bob@example.com")
}

func TestGatewayChatCompletionsStreamPIIBlockSkipsProvider(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	createContentPolicyViaHTTP(t, router, apiKey.AdminToken, createContentPolicyRequest{
		Name:      "block-stream-pii",
		PIIAction: "block",
	})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeAppOpenAIChatResponse(t, w)
	}))
	defer server.Close()

	provider, model := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "pii-stream-block", server.URL, 100)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "pii-stream-block-policy",
		MatchModel: "pii-stream-block",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"pii-stream-block","messages":[{"role":"user","content":"call 415-555-1212"}],"stream":true}`)
	assertGatewayError(t, rec, http.StatusBadRequest, "content_policy_blocked")

	if got := calls.Load(); got != 0 {
		t.Fatalf("provider calls = %d, want 0", got)
	}
	assertAppTableCount(t, ctx, st, "request_logs", 1)
	assertAppRequestLogContentPolicy(t, ctx, st, "block", "phone")
	assertAppRequestLogMetadataOmits(t, ctx, st, "415-555-1212")
}

func TestGatewayChatCompletionsRecordsTracingSpansWithoutSensitiveAttributes(t *testing.T) {
	oldProvider := otel.GetTracerProvider()
	oldPropagator := otel.GetTextMapPropagator()
	recorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() {
		otel.SetTracerProvider(oldProvider)
		otel.SetTextMapPropagator(oldPropagator)
		_ = tracerProvider.Shutdown(context.Background())
	})

	router, _, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Traceparent") == "" {
			t.Fatal("provider request missing traceparent header")
		}
		writeAppOpenAIChatResponse(t, w)
	}))
	defer server.Close()

	provider, model := createOpenAICompatibleRouteTarget(t, router, apiKey.AdminToken, "trace-chat", server.URL, 100)
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "trace-chat-policy",
		MatchModel: "trace-chat",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"trace-chat","messages":[{"role":"user","content":"trace me without leaking"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("trace chat status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	spans := recorder.Ended()
	for _, name := range []string{
		"gateway.chat",
		"gateway.budget_check",
		"gateway.api_key_quota_check",
		"gateway.routing",
		"gateway.provider_attempt",
		"provider.openai.chat",
	} {
		if !hasSpanNamed(spans, name) {
			t.Fatalf("span %q not recorded; spans=%v", name, spanNames(spans))
		}
	}
	if spanAttributesContain(spans, "trace me without leaking") {
		t.Fatal("span attributes contain prompt text")
	}
	if spanAttributesContain(spans, "provider-secret-key") {
		t.Fatal("span attributes contain provider api key")
	}
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

func newGatewayChatTestRouterWithPromptCache(t *testing.T) (*gin.Engine, *store.Store, gatewayChatFixture) {
	t.Helper()

	ctx := context.Background()
	redisClient := testutil.OpenRedis(t, ctx)
	promptCache := promptcache.NewPromptCache(redisClient, time.Minute)
	return newGatewayChatTestRouterWithOptions(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}}, nil, promptCache)
}

func newGatewayChatTestRouterWithSemanticCache(t *testing.T, threshold float64, maxTemp float64) (*gin.Engine, *store.Store, gatewayChatFixture) {
	t.Helper()

	ctx := context.Background()
	redisClient := testutil.OpenRedis(t, ctx)
	semanticCache := promptcache.NewSemanticCache(redisClient, time.Minute)
	return newGatewayChatTestRouterWithSemanticOptions(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}}, nil, semanticCache, threshold, maxTemp)
}

func newGatewayChatTestRouterWithMetrics(t *testing.T, limiter fakeGatewayLimiter, metrics *observability.Metrics) (*gin.Engine, *store.Store, gatewayChatFixture) {
	return newGatewayChatTestRouterWithOptions(t, limiter, metrics, nil)
}

func newGatewayChatTestRouterWithOptions(t *testing.T, limiter fakeGatewayLimiter, metrics *observability.Metrics, promptCache *promptcache.PromptCache) (*gin.Engine, *store.Store, gatewayChatFixture) {
	return newGatewayChatTestRouterWithFullOptions(t, limiter, metrics, promptCache, nil, 0, 0)
}

func newGatewayChatTestRouterWithSemanticOptions(t *testing.T, limiter fakeGatewayLimiter, metrics *observability.Metrics, semanticCache *promptcache.SemanticCache, threshold float64, maxTemp float64) (*gin.Engine, *store.Store, gatewayChatFixture) {
	return newGatewayChatTestRouterWithFullOptions(t, limiter, metrics, nil, semanticCache, threshold, maxTemp)
}

func newGatewayChatTestRouterWithFullOptions(t *testing.T, limiter fakeGatewayLimiter, metrics *observability.Metrics, promptCache *promptcache.PromptCache, semanticCache *promptcache.SemanticCache, threshold float64, maxTemp float64) (*gin.Engine, *store.Store, gatewayChatFixture) {
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
		MaxRetries:             1,
		RetryBackoffMS:         1,
		Store:                  st,
		RateLimiter:            limiter,
		Metrics:                metrics,
		PromptCache:            promptCache,
		SemanticCache:          semanticCache,
		EmbeddingAdapter:       embedding.NewMockAdapter(),
		SemanticCacheThreshold: threshold,
		SemanticCacheMaxTemp:   maxTemp,
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

type gatewayChatResponseBody struct {
	RequestID uuid.UUID `json:"request_id"`
	Provider  struct {
		ID uuid.UUID `json:"id"`
	} `json:"provider"`
	Cost struct {
		TotalCostMicro int64 `json:"total_cost_micro"`
	} `json:"cost"`
}

func decodeGatewayChatResponse(t *testing.T, rec *httptest.ResponseRecorder) gatewayChatResponseBody {
	t.Helper()

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/chat/completions status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body gatewayChatResponseBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode chat completion response: %v", err)
	}
	return body
}

type appCostPricingVersion struct {
	totalCostMicro   int64
	pricingVersionID uuid.UUID
	version          int32
}

func readAppCostPricingVersion(t *testing.T, ctx context.Context, st *store.Store, requestID uuid.UUID) appCostPricingVersion {
	t.Helper()

	var result appCostPricingVersion
	if err := st.Pool.QueryRow(ctx, `
		SELECT cr.total_cost_micro, cr.pricing_version_id, mpv.version
		FROM cost_records cr
		JOIN usage_records ur
		  ON ur.org_id = cr.org_id
		 AND ur.id = cr.usage_record_id
		JOIN model_pricing_versions mpv
		  ON mpv.org_id = cr.org_id
		 AND mpv.id = cr.pricing_version_id
		WHERE ur.request_log_id = $1
	`, requestID).Scan(&result.totalCostMicro, &result.pricingVersionID, &result.version); err != nil {
		t.Fatalf("read cost pricing version for request %s: %v", requestID, err)
	}
	return result
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

func assertAppRequestLogProvider(t *testing.T, ctx context.Context, st *store.Store, want uuid.UUID) {
	t.Helper()

	var got uuid.UUID
	if err := st.Pool.QueryRow(ctx, `SELECT provider_id FROM request_logs ORDER BY started_at DESC LIMIT 1`).Scan(&got); err != nil {
		t.Fatalf("select latest request log provider_id: %v", err)
	}
	if got != want {
		t.Fatalf("request log provider_id = %s, want %s", got, want)
	}
}

func assertAppRequestLogFallbackCount(t *testing.T, ctx context.Context, st *store.Store, want int) {
	t.Helper()

	var got int
	if err := st.Pool.QueryRow(ctx, `SELECT COALESCE((metadata->>'fallback_count')::int, 0) FROM request_logs ORDER BY started_at DESC LIMIT 1`).Scan(&got); err != nil {
		t.Fatalf("select latest request log fallback_count: %v", err)
	}
	if got != want {
		t.Fatalf("fallback_count = %d, want %d", got, want)
	}
}

func assertAppRequestLogTargetAttempts(t *testing.T, ctx context.Context, st *store.Store, want int) {
	t.Helper()

	var got int
	if err := st.Pool.QueryRow(ctx, `SELECT COALESCE((metadata->'attempted_targets'->0->>'attempts')::int, 0) FROM request_logs ORDER BY started_at DESC LIMIT 1`).Scan(&got); err != nil {
		t.Fatalf("select latest request log target attempts: %v", err)
	}
	if got != want {
		t.Fatalf("target attempts = %d, want %d", got, want)
	}
}

func assertAppRequestLogAPIKeyQuota(t *testing.T, ctx context.Context, st *store.Store, wantPeriod string, wantAction string, wantExceeded bool) {
	t.Helper()

	var period string
	var action string
	var exceeded bool
	if err := st.Pool.QueryRow(ctx, `
		SELECT
			metadata->'api_key_quota'->>'period',
			metadata->'api_key_quota'->>'action',
			(metadata->'api_key_quota'->>'exceeded')::boolean
		FROM request_logs
		ORDER BY started_at DESC
		LIMIT 1
	`).Scan(&period, &action, &exceeded); err != nil {
		t.Fatalf("select latest request log api_key_quota metadata: %v", err)
	}
	if period != wantPeriod || action != wantAction || exceeded != wantExceeded {
		t.Fatalf("api_key_quota metadata = period:%q action:%q exceeded:%t, want period:%q action:%q exceeded:%t",
			period, action, exceeded, wantPeriod, wantAction, wantExceeded)
	}
}

func assertAppRequestLogContentPolicy(t *testing.T, ctx context.Context, st *store.Store, wantAction string, wantType string) {
	t.Helper()

	var action string
	var policyHit bool
	var hasType bool
	if err := st.Pool.QueryRow(ctx, `
		SELECT
			metadata->'content_policy'->>'action',
			(metadata->'content_policy'->>'policy_hit')::boolean,
			metadata->'content_policy'->'types' ? $1
		FROM request_logs
		ORDER BY started_at DESC
		LIMIT 1
	`, wantType).Scan(&action, &policyHit, &hasType); err != nil {
		t.Fatalf("select latest request log content_policy metadata: %v", err)
	}
	if action != wantAction || !policyHit || !hasType {
		t.Fatalf("content_policy metadata = action:%q policy_hit:%t has_type:%t, want action:%q hit:true type:%q",
			action, policyHit, hasType, wantAction, wantType)
	}
}

func assertAppRequestLogMetadataOmits(t *testing.T, ctx context.Context, st *store.Store, raw string) {
	t.Helper()

	var count int
	if err := st.Pool.QueryRow(ctx, `
		SELECT count(*)::int
		FROM request_logs
		WHERE COALESCE(metadata::text, '') LIKE '%' || $1 || '%'
	`, raw).Scan(&count); err != nil {
		t.Fatalf("search request log metadata for raw content: %v", err)
	}
	if count != 0 {
		t.Fatalf("request log metadata contains raw content %q", raw)
	}
}

func hasSpanNamed(spans []sdktrace.ReadOnlySpan, name string) bool {
	for _, span := range spans {
		if span.Name() == name {
			return true
		}
	}
	return false
}

func spanNames(spans []sdktrace.ReadOnlySpan) []string {
	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name())
	}
	return names
}

func spanAttributesContain(spans []sdktrace.ReadOnlySpan, value string) bool {
	for _, span := range spans {
		for _, attr := range span.Attributes() {
			if strings.Contains(attr.Value.AsString(), value) {
				return true
			}
		}
	}
	return false
}

func assertAppCacheEventCount(t *testing.T, ctx context.Context, st *store.Store, eventType string, want int) {
	t.Helper()

	var got int
	if err := st.Pool.QueryRow(ctx, `SELECT count(*)::int FROM cache_events WHERE event_type = $1`, eventType).Scan(&got); err != nil {
		t.Fatalf("count cache_events %s: %v", eventType, err)
	}
	if got != want {
		t.Fatalf("cache_events %s count = %d, want %d", eventType, got, want)
	}
}

func assertAppLatestRequestLogCacheStatus(t *testing.T, ctx context.Context, st *store.Store, want string) {
	t.Helper()

	var got string
	if err := st.Pool.QueryRow(ctx, `
		SELECT metadata->'cache'->>'status'
		FROM request_logs
		ORDER BY started_at DESC
		LIMIT 1
	`).Scan(&got); err != nil {
		t.Fatalf("select latest request log cache metadata: %v", err)
	}
	if got != want {
		t.Fatalf("cache metadata status = %q, want %q", got, want)
	}
}

func assertCacheEventsRouteListsEvents(t *testing.T, router http.Handler, adminToken string, eventType string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/cache-events?event_type="+eventType, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/cache-events status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Items []struct {
			EventType      string `json:"event_type"`
			RequestedModel string `json:"requested_model"`
			CacheKeyHash   string `json:"cache_key_hash"`
			MessagesHash   string `json:"messages_hash"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode cache events response: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("cache event count = %d, want 1; body=%s", len(response.Items), rec.Body.String())
	}
	item := response.Items[0]
	if item.EventType != eventType || item.RequestedModel != "cache-chat" || item.CacheKeyHash == "" || item.MessagesHash == "" {
		t.Fatalf("cache event item = %+v", item)
	}
	if strings.Contains(rec.Body.String(), "cache me") {
		t.Fatal("cache events response contains prompt text")
	}
}

type createContentPolicyRequest struct {
	Name      string `json:"name"`
	PIIAction string `json:"pii_action"`
}

type contentPolicyResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	PIIAction string    `json:"pii_action"`
	Status    string    `json:"status"`
}

type listContentPoliciesResponse struct {
	Items []contentPolicyResponse `json:"items"`
}

func createContentPolicyViaHTTP(t *testing.T, router http.Handler, adminToken string, body createContentPolicyRequest) contentPolicyResponse {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal create content policy request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/content-policies", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/admin/content-policies status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response contentPolicyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create content policy response: %v", err)
	}
	if response.ID == uuid.Nil || response.Name != body.Name || response.PIIAction != body.PIIAction || response.Status != "active" {
		t.Fatalf("content policy response = %+v, want name=%q pii_action=%q active", response, body.Name, body.PIIAction)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/content-policies", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/content-policies status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var listResponse listContentPoliciesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listResponse); err != nil {
		t.Fatalf("decode list content policies response: %v", err)
	}
	if len(listResponse.Items) != 1 || listResponse.Items[0].ID != response.ID {
		t.Fatalf("content policy list = %+v, want created policy %s", listResponse.Items, response.ID)
	}
	return response
}

func assertAppBudgetAlertDeliveryCount(t *testing.T, ctx context.Context, st *store.Store, want int) {
	t.Helper()

	var got int
	if err := st.Pool.QueryRow(ctx, `SELECT count(*)::int FROM budget_alert_deliveries`).Scan(&got); err != nil {
		t.Fatalf("count budget_alert_deliveries: %v", err)
	}
	if got != want {
		t.Fatalf("budget_alert_deliveries count = %d, want %d", got, want)
	}
}

func createOpenAICompatibleRouteTarget(t *testing.T, router http.Handler, adminToken string, name string, baseURL string, price int64) (providerResponse, modelResponse) {
	t.Helper()

	provider := createProviderViaHTTP(t, router, adminToken, createProviderRequest{
		Name:      name + "-provider",
		Type:      "openai_compatible",
		BaseURL:   stringPtr(baseURL + "/v1"),
		APIKey:    stringPtr("provider-secret-key"),
		TimeoutMS: 1000,
	})
	model := createModelViaHTTP(t, router, adminToken, createModelRequest{
		ProviderID:                     provider.ID,
		ProviderModelName:              name + "-model",
		DisplayName:                    name + " Model",
		InputPriceMicroUSDPer1KTokens:  price,
		OutputPriceMicroUSDPer1KTokens: price,
	})
	return provider, model
}

func writeAppOpenAIChatResponse(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write([]byte(`{"id":"chatcmpl_test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`))
	if err != nil {
		t.Fatalf("write chat response: %v", err)
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
