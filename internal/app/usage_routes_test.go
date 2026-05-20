package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestAdminRequestLogsCursorPagination(t *testing.T) {
	router, _, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	for i := 0; i < 3; i++ {
		rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("chat completion %d status = %d, want %d; body=%s", i, rec.Code, http.StatusOK, rec.Body.String())
		}
		time.Sleep(time.Millisecond)
	}

	first := getRequestLogsViaHTTP(t, router, apiKey.AdminToken, "limit=1")
	if len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatalf("first page = %+v, want one item and next cursor", first)
	}
	second := getRequestLogsViaHTTP(t, router, apiKey.AdminToken, "limit=1&cursor="+url.QueryEscape(*first.NextCursor))
	if len(second.Items) != 1 || second.NextCursor == nil {
		t.Fatalf("second page = %+v, want one item and next cursor", second)
	}
	if second.Items[0].ID == first.Items[0].ID {
		t.Fatalf("second page repeated id %s", second.Items[0].ID)
	}
	third := getRequestLogsViaHTTP(t, router, apiKey.AdminToken, "limit=1&cursor="+url.QueryEscape(*second.NextCursor))
	if len(third.Items) != 1 {
		t.Fatalf("third page count = %d, want 1", len(third.Items))
	}
	if third.Items[0].ID == first.Items[0].ID || third.Items[0].ID == second.Items[0].ID {
		t.Fatalf("third page repeated id %s", third.Items[0].ID)
	}
}

func TestAdminRequestLogsFilterByErrorCodeAndRequestModel(t *testing.T) {
	router, _, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat completion status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	missing := performChatCompletion(t, router, apiKey.Key, `{"model":"missing-model","messages":[{"role":"user","content":"hello"}]}`)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing route status = %d, want %d; body=%s", missing.Code, http.StatusNotFound, missing.Body.String())
	}

	errorLogs := getRequestLogsViaHTTP(t, router, apiKey.AdminToken, "error_code=route_not_found")
	if len(errorLogs.Items) != 1 || errorLogs.Items[0].ErrorCode == nil || *errorLogs.Items[0].ErrorCode != "route_not_found" {
		t.Fatalf("error_code filtered logs = %+v, want one route_not_found", errorLogs.Items)
	}
	modelLogs := getRequestLogsViaHTTP(t, router, apiKey.AdminToken, "request_model=fast-chat")
	if len(modelLogs.Items) != 1 || modelLogs.Items[0].RequestModel == nil || *modelLogs.Items[0].RequestModel != "fast-chat" {
		t.Fatalf("request_model filtered logs = %+v, want one fast-chat", modelLogs.Items)
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
		ID           string  `json:"id"`
		RequestModel *string `json:"request_model"`
		Status       string  `json:"status"`
		StatusCode   int32   `json:"status_code"`
		ErrorCode    *string `json:"error_code"`
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
