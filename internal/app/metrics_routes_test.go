package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tep/llm-cost-gateway/internal/observability"
	"github.com/tep/llm-cost-gateway/internal/ratelimit"
)

func TestMetricsEndpointIsAccessible(t *testing.T) {
	router, _ := newTask5TestRouter(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "llmgw_requests_total") {
		t.Fatalf("metrics body does not include llmgw_requests_total: %s", rec.Body.String())
	}
}

func TestMetricsIncrementAfterGatewaySuccess(t *testing.T) {
	metrics := observability.NewMetrics()
	router, _, apiKey := newGatewayChatTestRouterWithMetrics(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}}, metrics)

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat completion status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	body := readMetrics(t, router)
	assertMetricContains(t, body, `llmgw_requests_total{model="fast-chat",provider="mock-provider",status="success"} 1`)
	assertMetricContains(t, body, `llmgw_tokens_total{model="fast-chat",type="total"} 50`)
	assertMetricContains(t, body, `llmgw_cost_micro_usd_total{model="fast-chat"} 8`)
}

func TestMetricsIncrementAfterRateLimit(t *testing.T) {
	metrics := observability.NewMetrics()
	router, _, apiKey := newGatewayChatTestRouterWithMetrics(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: false}}, metrics)

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("chat completion status = %d, want %d; body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}

	assertMetricContains(t, readMetrics(t, router), `llmgw_rate_limited_total 1`)
}

func TestMetricsIncrementAfterBudgetBlock(t *testing.T) {
	metrics := observability.NewMetrics()
	router, _, apiKey := newGatewayChatTestRouterWithMetrics(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}}, metrics)
	createBudgetViaHTTP(t, router, apiKey.AdminToken, createBudgetRequest{
		Name:          "block-budget",
		ScopeType:     "org",
		Period:        "daily",
		LimitMicroUSD: 0,
		Action:        "block",
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("chat completion status = %d, want %d; body=%s", rec.Code, http.StatusPaymentRequired, rec.Body.String())
	}

	assertMetricContains(t, readMetrics(t, router), `llmgw_budget_blocked_total 1`)
}

func readMetrics(t *testing.T, router http.Handler) string {
	t.Helper()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	return rec.Body.String()
}

func assertMetricContains(t *testing.T, body string, want string) {
	t.Helper()

	if !strings.Contains(body, want) {
		t.Fatalf("metrics body does not include %q:\n%s", want, body)
	}
}
