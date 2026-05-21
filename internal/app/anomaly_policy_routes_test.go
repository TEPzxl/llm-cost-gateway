package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/ratelimit"
	"github.com/tep/llm-cost-gateway/internal/store"
)

func TestAdminAnomalyPoliciesCreateAndList(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Anomaly Admin Org", "anomaly-admin-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	threshold := int64(1000)

	created := createAnomalyPolicyViaHTTP(t, router, adminToken.Token, createAnomalyPolicyRouteRequest{
		Name:              "daily block",
		RuleType:          "daily_cost",
		ScopeType:         "org",
		ThresholdMicroUSD: &threshold,
		Action:            "block",
	})
	if created.Name != "daily block" || created.RuleType != "daily_cost" || created.Action != "block" {
		t.Fatalf("created anomaly policy = %+v", created)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/anomaly-policies", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/anomaly-policies status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Items []anomalyPolicyRouteResponse `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode anomaly policies response: %v", err)
	}
	if len(response.Items) != 1 || response.Items[0].ID != created.ID {
		t.Fatalf("anomaly policies response = %+v, want created policy %s", response.Items, created.ID)
	}
}

func TestAdminAnomalyPoliciesRejectViewer(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Anomaly Viewer Org", "anomaly-viewer-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	member := createMemberViaHTTP(t, router, adminToken.Token, createMemberRouteRequest{
		Email:       "viewer-anomaly@example.com",
		DisplayName: "Viewer",
		Role:        "viewer",
	})
	session := passwordlessMockLoginViaHTTP(t, router, "anomaly-viewer-org", member.Email)
	threshold := int64(1000)

	assertAnomalyPolicyStatus(t, router, session.Token, createAnomalyPolicyRouteRequest{
		Name:              "viewer blocked",
		RuleType:          "daily_cost",
		ScopeType:         "org",
		ThresholdMicroUSD: &threshold,
		Action:            "block",
	}, http.StatusForbidden)
}

func TestGatewayCostAnomalyBlock(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	threshold := int64(0)
	createAnomalyPolicyViaHTTP(t, router, apiKey.AdminToken, createAnomalyPolicyRouteRequest{
		Name:              "block all",
		RuleType:          "daily_cost",
		ScopeType:         "org",
		ThresholdMicroUSD: &threshold,
		Action:            "block",
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	assertGatewayError(t, rec, http.StatusTooManyRequests, domain.CodeCostAnomalyBlocked)
	assertAppRequestLogErrorCode(t, ctx, st, domain.CodeCostAnomalyBlocked)
	assertAppRequestLogAnomalyAction(t, ctx, st, "block")
}

func TestGatewayCostAnomalyDowngrade(t *testing.T) {
	ctx := context.Background()
	router, st, apiKey := newGatewayChatTestRouter(t, fakeGatewayLimiter{decision: ratelimit.Decision{Allowed: true}})
	provider := createProviderViaHTTP(t, router, apiKey.AdminToken, createProviderRequest{
		Name:      "cheap-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	model := createModelViaHTTP(t, router, apiKey.AdminToken, createModelRequest{
		ProviderID:                     provider.ID,
		ProviderModelName:              "mock-cheap",
		DisplayName:                    "Mock Cheap",
		InputPriceMicroUSDPer1KTokens:  10,
		OutputPriceMicroUSDPer1KTokens: 20,
		ContextWindow:                  int32Ptr(8192),
	})
	createRoutePolicyViaHTTP(t, router, apiKey.AdminToken, createRoutePolicyRequest{
		Name:       "cheap-chat",
		MatchModel: "cheap-chat",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{
			{ProviderID: provider.ID, ModelID: model.ID, Priority: 1, Weight: 100},
		},
	})
	threshold := int64(0)
	createAnomalyPolicyViaHTTP(t, router, apiKey.AdminToken, createAnomalyPolicyRouteRequest{
		Name:              "downgrade all",
		RuleType:          "daily_cost",
		ScopeType:         "org",
		ThresholdMicroUSD: &threshold,
		Action:            "downgrade",
		FallbackModel:     "cheap-chat",
	})

	rec := performChatCompletion(t, router, apiKey.Key, `{"model":"fast-chat","messages":[{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /v1/chat/completions status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if response.Model != "cheap-chat" {
		t.Fatalf("response model = %q, want cheap-chat", response.Model)
	}
	assertAppRequestLogAnomalyAction(t, ctx, st, "downgrade")
}

type createAnomalyPolicyRouteRequest struct {
	Name                  string     `json:"name"`
	RuleType              string     `json:"rule_type"`
	ScopeType             string     `json:"scope_type"`
	ScopeID               *uuid.UUID `json:"scope_id"`
	ModelAlias            string     `json:"model_alias"`
	ThresholdMicroUSD     *int64     `json:"threshold_micro_usd"`
	ThresholdBPS          *int32     `json:"threshold_bps"`
	SpikeMultiplierBPS    int32      `json:"spike_multiplier_bps"`
	CurrentWindowMinutes  int32      `json:"current_window_minutes"`
	BaselineWindowMinutes int32      `json:"baseline_window_minutes"`
	MinRequests           int32      `json:"min_requests"`
	Action                string     `json:"action"`
	FallbackModel         string     `json:"fallback_model"`
}

type anomalyPolicyRouteResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	RuleType  string    `json:"rule_type"`
	ScopeType string    `json:"scope_type"`
	Action    string    `json:"action"`
}

func createAnomalyPolicyViaHTTP(t *testing.T, router http.Handler, token string, body createAnomalyPolicyRouteRequest) anomalyPolicyRouteResponse {
	t.Helper()

	rec := assertAnomalyPolicyStatus(t, router, token, body, http.StatusCreated)
	var response anomalyPolicyRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode anomaly policy response: %v", err)
	}
	return response
}

func assertAnomalyPolicyStatus(t *testing.T, router http.Handler, token string, body createAnomalyPolicyRouteRequest, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal anomaly policy request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/anomaly-policies", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("POST /api/v1/admin/anomaly-policies status = %d, want %d; body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	return rec
}

func assertAppRequestLogAnomalyAction(t *testing.T, ctx context.Context, st *store.Store, want string) {
	t.Helper()

	var got string
	if err := st.Pool.QueryRow(ctx, `
		SELECT COALESCE(metadata->'cost_anomaly'->>'action', '')
		FROM request_logs
		ORDER BY started_at DESC
		LIMIT 1
	`).Scan(&got); err != nil {
		t.Fatalf("select latest request log anomaly action: %v", err)
	}
	if got != want {
		t.Fatalf("anomaly action = %q, want %q", got, want)
	}
}
