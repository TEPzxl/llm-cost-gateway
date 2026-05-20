package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

func TestAdminRoutePolicyRoutesCreateAndListPolicy(t *testing.T) {
	ctx := context.Background()
	router, st := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Route Policy Org", "route-policy-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	provider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "mock-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	model := createModelViaHTTP(t, router, adminToken.Token, createModelRequest{
		ProviderID:                     provider.ID,
		ProviderModelName:              "mock-small",
		DisplayName:                    "Mock Small",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})

	policy := createRoutePolicyViaHTTP(t, router, adminToken.Token, createRoutePolicyRequest{
		Name:       "fast-chat-policy",
		MatchModel: "fast-chat",
		Strategy:   "single",
		Targets: []createRouteTargetRequest{{
			ProviderID: provider.ID,
			ModelID:    model.ID,
			Priority:   1,
			Weight:     100,
		}},
	})
	if policy.MatchModel != "fast-chat" {
		t.Fatalf("match_model = %q, want fast-chat", policy.MatchModel)
	}

	targets, err := st.Queries.ListRouteTargets(ctx, db.ListRouteTargetsParams{
		OrgID:         orgID,
		RoutePolicyID: policy.ID,
	})
	if err != nil {
		t.Fatalf("ListRouteTargets returned error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("created %d route targets, want 1", len(targets))
	}
	if targets[0].ProviderID != provider.ID || targets[0].ModelID != model.ID {
		t.Fatalf("route target = %+v, want provider %s model %s", targets[0], provider.ID, model.ID)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/route-policies", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/route-policies status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var listBody struct {
		Items []routePolicyResponse `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list route policies response: %v", err)
	}
	if len(listBody.Items) != 1 || listBody.Items[0].ID != policy.ID {
		t.Fatalf("listed route policies = %+v, want only %s", listBody.Items, policy.ID)
	}
}

func TestAdminRoutePolicyRoutesCreateFallbackPolicyWithMultipleTargets(t *testing.T) {
	ctx := context.Background()
	router, st := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Fallback Route Org", "fallback-route-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	providerA := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "mock-provider-a",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	modelA := createModelViaHTTP(t, router, adminToken.Token, createModelRequest{
		ProviderID:                     providerA.ID,
		ProviderModelName:              "mock-small-a",
		DisplayName:                    "Mock Small A",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})
	providerB := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "mock-provider-b",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	modelB := createModelViaHTTP(t, router, adminToken.Token, createModelRequest{
		ProviderID:                     providerB.ID,
		ProviderModelName:              "mock-small-b",
		DisplayName:                    "Mock Small B",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})

	policy := createRoutePolicyViaHTTP(t, router, adminToken.Token, createRoutePolicyRequest{
		Name:       "fallback-policy",
		MatchModel: "fallback-chat",
		Strategy:   "fallback",
		Targets: []createRouteTargetRequest{
			{ProviderID: providerA.ID, ModelID: modelA.ID, Priority: 1, Weight: 100},
			{ProviderID: providerB.ID, ModelID: modelB.ID, Priority: 2, Weight: 100},
		},
	})
	if policy.Strategy != "fallback" {
		t.Fatalf("strategy = %q, want fallback", policy.Strategy)
	}

	targets, err := st.Queries.ListRouteTargets(ctx, db.ListRouteTargetsParams{
		OrgID:         orgID,
		RoutePolicyID: policy.ID,
	})
	if err != nil {
		t.Fatalf("ListRouteTargets returned error: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("created %d route targets, want 2", len(targets))
	}
	if targets[0].Priority != 1 || targets[1].Priority != 2 {
		t.Fatalf("target priorities = %d/%d, want 1/2", targets[0].Priority, targets[1].Priority)
	}
}

func TestAdminRoutePolicyRoutesRejectFallbackWithSingleTarget(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Fallback Single Org", "fallback-single-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	provider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "mock-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	model := createModelViaHTTP(t, router, adminToken.Token, createModelRequest{
		ProviderID:                     provider.ID,
		ProviderModelName:              "mock-small",
		DisplayName:                    "Mock Small",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})

	payload := `{"name":"bad-fallback","match_model":"fallback-chat","strategy":"fallback","targets":[{"provider_id":"` + provider.ID.String() + `","model_id":"` + model.ID.String() + `","priority":1,"weight":100}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/route-policies", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/v1/admin/route-policies status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestAdminRoutePolicyRoutesRejectCrossOrgTarget(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgA := createOrgViaHTTP(t, router, "Route Owner Org", "route-owner-org")
	adminA := createAdminTokenViaHTTP(t, router, orgA)
	orgB := createOrgViaHTTP(t, router, "Route Caller Org", "route-caller-org")
	adminB := createAdminTokenViaHTTP(t, router, orgB)

	providerA := createProviderViaHTTP(t, router, adminA.Token, createProviderRequest{
		Name:      "org-a-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	modelA := createModelViaHTTP(t, router, adminA.Token, createModelRequest{
		ProviderID:                     providerA.ID,
		ProviderModelName:              "mock-small",
		DisplayName:                    "Mock Small",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})

	payload := `{"name":"bad-policy","match_model":"fast-chat","strategy":"single","targets":[{"provider_id":"` + providerA.ID.String() + `","model_id":"` + modelA.ID.String() + `","priority":1,"weight":100}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/route-policies", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+adminB.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /api/v1/admin/route-policies status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAdminRoutePolicyRoutesRequireSingleTarget(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Route Target Org", "route-target-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/route-policies",
		strings.NewReader(`{"name":"bad-policy","match_model":"fast-chat","strategy":"single","targets":[]}`),
	)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/v1/admin/route-policies status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

type createRoutePolicyRequest struct {
	Name       string                     `json:"name"`
	MatchModel string                     `json:"match_model"`
	Strategy   string                     `json:"strategy"`
	Targets    []createRouteTargetRequest `json:"targets"`
}

type createRouteTargetRequest struct {
	ProviderID uuid.UUID `json:"provider_id"`
	ModelID    uuid.UUID `json:"model_id"`
	Priority   int32     `json:"priority"`
	Weight     int32     `json:"weight"`
}

type routePolicyResponse struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	MatchModel string    `json:"match_model"`
	Strategy   string    `json:"strategy"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

func createRoutePolicyViaHTTP(t *testing.T, router http.Handler, adminToken string, body createRoutePolicyRequest) routePolicyResponse {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal create route policy request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/route-policies", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/admin/route-policies status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response routePolicyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create route policy response: %v", err)
	}
	return response
}
