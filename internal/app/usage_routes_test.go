package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tep/llm-cost-gateway/internal/ratelimit"
)

func TestAdminUsageRoutesReturnEmptyArrays(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Empty Usage Org", "empty-usage-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	logs := getRequestLogsViaHTTP(t, router, adminToken.Token, "")
	if len(logs.Items) != 0 {
		t.Fatalf("request logs count = %d, want 0", len(logs.Items))
	}

	summary := getUsageSummaryViaHTTP(t, router, adminToken.Token, "group_by=model")
	if summary.GroupBy != "model" || len(summary.Items) != 0 {
		t.Fatalf("summary = %+v, want empty model summary", summary)
	}
}

func TestAdminUsageSummaryAfterSuccessfulGatewayRequest(t *testing.T) {
	router, _, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat completion status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	summary := getUsageSummaryViaHTTP(t, router, apiKey.AdminToken, "group_by=model")
	if summary.GroupBy != "model" || len(summary.Items) != 1 {
		t.Fatalf("summary = %+v, want one model item", summary)
	}
	item := summary.Items[0]
	if item.RequestCount != 1 || item.SuccessCount != 1 || item.TotalTokens != 50 || item.TotalCostMicroUSD != 8 {
		t.Fatalf("summary item = %+v, want request=1 success=1 tokens=50 cost=8", item)
	}
}

func TestAdminUsageSummaryRejectsInvalidGroupBy(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Invalid Usage Org", "invalid-usage-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/summary?group_by=route", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/v1/admin/usage/summary status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestAdminRequestLogsTimeFiltering(t *testing.T) {
	router, _, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat completion status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	future := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	later := time.Now().UTC().Add(25 * time.Hour).Format(time.RFC3339)
	logs := getRequestLogsViaHTTP(t, router, apiKey.AdminToken, "from="+future+"&to="+later)
	if len(logs.Items) != 0 {
		t.Fatalf("future request logs count = %d, want 0", len(logs.Items))
	}
}

func TestAdminUsageRoutesAreScopedByOrg(t *testing.T) {
	router, _, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat completion status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	orgB := createOrgViaHTTP(t, router, "Usage Org B", "usage-org-b")
	adminB := createAdminTokenViaHTTP(t, router, orgB)
	logsB := getRequestLogsViaHTTP(t, router, adminB.Token, "")
	if len(logsB.Items) != 0 {
		t.Fatalf("org B request logs count = %d, want 0", len(logsB.Items))
	}
	summaryB := getUsageSummaryViaHTTP(t, router, adminB.Token, "group_by=model")
	if len(summaryB.Items) != 0 {
		t.Fatalf("org B summary count = %d, want 0", len(summaryB.Items))
	}
}

type requestLogsRouteResponse struct {
	Items []struct {
		Status     string `json:"status"`
		StatusCode int32  `json:"status_code"`
	} `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

type usageSummaryRouteResponse struct {
	GroupBy string `json:"group_by"`
	Items   []struct {
		RequestCount      int64 `json:"request_count"`
		SuccessCount      int64 `json:"success_count"`
		ErrorCount        int64 `json:"error_count"`
		PromptTokens      int64 `json:"prompt_tokens"`
		CompletionTokens  int64 `json:"completion_tokens"`
		TotalTokens       int64 `json:"total_tokens"`
		TotalCostMicroUSD int64 `json:"total_cost_micro_usd"`
	} `json:"items"`
}

func getRequestLogsViaHTTP(t *testing.T, router http.Handler, adminToken string, query string) requestLogsRouteResponse {
	t.Helper()

	path := "/api/v1/admin/request-logs"
	if query != "" {
		path += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d; body=%s", path, rec.Code, http.StatusOK, rec.Body.String())
	}

	var response requestLogsRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode request logs response: %v", err)
	}
	return response
}

func getUsageSummaryViaHTTP(t *testing.T, router http.Handler, adminToken string, query string) usageSummaryRouteResponse {
	t.Helper()

	path := "/api/v1/admin/usage/summary"
	if query != "" {
		path += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d; body=%s", path, rec.Code, http.StatusOK, rec.Body.String())
	}

	var response usageSummaryRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode usage summary response: %v", err)
	}
	return response
}
