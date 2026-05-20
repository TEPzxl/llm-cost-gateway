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
	"github.com/tep/llm-cost-gateway/internal/budget"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

func TestAdminBudgetRoutesCreateListAndStatus(t *testing.T) {
	ctx := context.Background()
	router, st := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Budget Org", "budget-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	apiKey := createAPIKeyViaHTTP(t, router, adminToken.Token, createAPIKeyRequest{
		Name:     "budget-key",
		Scopes:   []string{"chat.completions"},
		RPMLimit: 60,
	})
	provider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "budget-provider",
		Type:      "mock",
		APIKey:    stringPtr("mock-key"),
		TimeoutMS: 30000,
	})
	model := createModelViaHTTP(t, router, adminToken.Token, createModelRequest{
		ProviderID:                     provider.ID,
		ProviderModelName:              "budget-model",
		DisplayName:                    "Budget Model",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
		ContextWindow:                  int32Ptr(8192),
	})

	created := createBudgetViaHTTP(t, router, adminToken.Token, createBudgetRequest{
		Name:          "daily-budget",
		ScopeType:     "org",
		Period:        "daily",
		LimitMicroUSD: 1000,
		Action:        "block",
	})
	if created.Status != "active" || created.ScopeType != "org" || created.Period != "daily" {
		t.Fatalf("created budget = %+v", created)
	}

	insertAppBudgetCost(t, ctx, st.Queries, orgID, apiKey.ID, provider.ID, model.ID, 250, time.Now().UTC())

	list := listBudgetsViaHTTP(t, router, adminToken.Token)
	if len(list.Items) != 1 || list.Items[0].ID != created.ID {
		t.Fatalf("listed budgets = %+v, want created budget %s", list.Items, created.ID)
	}

	status := getBudgetStatusViaHTTP(t, router, adminToken.Token)
	if len(status.Items) != 1 {
		t.Fatalf("status item count = %d, want 1", len(status.Items))
	}
	item := status.Items[0]
	if item.BudgetID != created.ID || item.UsedMicroUSD != 250 || item.RemainingMicroUSD != 750 || item.Exceeded {
		t.Fatalf("status item = %+v, want used=250 remaining=750 not exceeded", item)
	}
}

func TestAdminBudgetRoutesAreScopedByOrg(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgA := createOrgViaHTTP(t, router, "Budget Org A", "budget-org-a")
	adminA := createAdminTokenViaHTTP(t, router, orgA)
	orgB := createOrgViaHTTP(t, router, "Budget Org B", "budget-org-b")
	adminB := createAdminTokenViaHTTP(t, router, orgB)

	createBudgetViaHTTP(t, router, adminA.Token, createBudgetRequest{
		Name:          "org-a-budget",
		ScopeType:     "org",
		Period:        "monthly",
		LimitMicroUSD: 1000,
		Action:        "warn",
	})

	listB := listBudgetsViaHTTP(t, router, adminB.Token)
	if len(listB.Items) != 0 {
		t.Fatalf("org B listed %d budgets, want 0", len(listB.Items))
	}
}

func TestAdminBudgetRoutesRejectInvalidRequest(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Invalid Budget Org", "invalid-budget-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/budgets",
		strings.NewReader(`{"name":"bad-budget","scope_type":"org","period":"weekly","limit_micro_usd":100,"action":"block"}`),
	)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/v1/admin/budgets status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestAdminBudgetAlertRoutesCreateListAndDeliveries(t *testing.T) {
	ctx := context.Background()
	router, st := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Budget Alert Org", "budget-alert-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	apiKey := createAPIKeyViaHTTP(t, router, adminToken.Token, createAPIKeyRequest{
		Name:     "budget-alert-key",
		Scopes:   []string{"chat.completions"},
		RPMLimit: 60,
	})
	provider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "budget-alert-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	model := createModelViaHTTP(t, router, adminToken.Token, createModelRequest{
		ProviderID:                     provider.ID,
		ProviderModelName:              "budget-alert-model",
		DisplayName:                    "Budget Alert Model",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})
	budgetItem := createBudgetViaHTTP(t, router, adminToken.Token, createBudgetRequest{
		Name:          "alert-budget",
		ScopeType:     "org",
		Period:        "monthly",
		LimitMicroUSD: 1000,
		Action:        "warn",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	alert := createBudgetAlertViaHTTP(t, router, adminToken.Token, createBudgetAlertRequest{
		BudgetID:      budgetItem.ID,
		WebhookURL:    server.URL,
		WebhookSecret: "secret",
		Status:        "active",
	})
	if alert.BudgetID != budgetItem.ID || alert.WebhookURL != server.URL {
		t.Fatalf("created alert = %+v", alert)
	}
	rawList := listBudgetAlertsRawViaHTTP(t, router, adminToken.Token)
	if strings.Contains(rawList, "secret") {
		t.Fatalf("budget alert list leaked webhook secret: %s", rawList)
	}

	insertAppBudgetCost(t, ctx, st.Queries, orgID, apiKey.ID, provider.ID, model.ID, 800, time.Now().UTC())
	alertService := budget.NewAlertService(st)
	if _, err := alertService.CheckAndDeliver(ctx, orgID); err != nil {
		t.Fatalf("CheckAndDeliver returned error: %v", err)
	}

	deliveries := listBudgetAlertDeliveriesViaHTTP(t, router, adminToken.Token)
	if len(deliveries.Items) != 1 || deliveries.Items[0].Threshold != 80 || deliveries.Items[0].Status != "success" {
		t.Fatalf("deliveries = %+v, want one successful 80 delivery", deliveries.Items)
	}
}

type createBudgetRequest struct {
	Name          string     `json:"name"`
	ScopeType     string     `json:"scope_type"`
	ScopeID       *uuid.UUID `json:"scope_id"`
	Period        string     `json:"period"`
	LimitMicroUSD int64      `json:"limit_micro_usd"`
	Action        string     `json:"action"`
}

type budgetRouteResponse struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	ScopeType     string    `json:"scope_type"`
	Period        string    `json:"period"`
	LimitMicroUSD int64     `json:"limit_micro_usd"`
	Action        string    `json:"action"`
	Status        string    `json:"status"`
}

type listBudgetsRouteResponse struct {
	Items []budgetRouteResponse `json:"items"`
}

type budgetStatusRouteResponse struct {
	Items []struct {
		BudgetID          uuid.UUID `json:"budget_id"`
		Name              string    `json:"name"`
		Period            string    `json:"period"`
		LimitMicroUSD     int64     `json:"limit_micro_usd"`
		UsedMicroUSD      int64     `json:"used_micro_usd"`
		RemainingMicroUSD int64     `json:"remaining_micro_usd"`
		Exceeded          bool      `json:"exceeded"`
		Action            string    `json:"action"`
	} `json:"items"`
}

type createBudgetAlertRequest struct {
	BudgetID      uuid.UUID `json:"budget_id"`
	WebhookURL    string    `json:"webhook_url"`
	WebhookSecret string    `json:"webhook_secret"`
	Status        string    `json:"status"`
}

type budgetAlertRouteResponse struct {
	ID         uuid.UUID `json:"id"`
	BudgetID   uuid.UUID `json:"budget_id"`
	WebhookURL string    `json:"webhook_url"`
	Status     string    `json:"status"`
}

type budgetAlertDeliveryRouteResponse struct {
	ID            uuid.UUID `json:"id"`
	BudgetAlertID uuid.UUID `json:"budget_alert_id"`
	BudgetID      uuid.UUID `json:"budget_id"`
	Threshold     int32     `json:"threshold"`
	Status        string    `json:"status"`
	HTTPStatus    *int32    `json:"http_status"`
}

type listBudgetAlertDeliveriesRouteResponse struct {
	Items []budgetAlertDeliveryRouteResponse `json:"items"`
}

func createBudgetViaHTTP(t *testing.T, router http.Handler, adminToken string, body createBudgetRequest) budgetRouteResponse {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal create budget request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/budgets", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/admin/budgets status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response budgetRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create budget response: %v", err)
	}
	return response
}

func createBudgetAlertViaHTTP(t *testing.T, router http.Handler, adminToken string, body createBudgetAlertRequest) budgetAlertRouteResponse {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal create budget alert request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/budget-alerts", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/admin/budget-alerts status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response budgetAlertRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create budget alert response: %v", err)
	}
	return response
}

func listBudgetAlertsRawViaHTTP(t *testing.T, router http.Handler, adminToken string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/budget-alerts", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/budget-alerts status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	return rec.Body.String()
}

func listBudgetAlertDeliveriesViaHTTP(t *testing.T, router http.Handler, adminToken string) listBudgetAlertDeliveriesRouteResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/budget-alert-deliveries", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/budget-alert-deliveries status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response listBudgetAlertDeliveriesRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode budget alert deliveries response: %v", err)
	}
	return response
}

func listBudgetsViaHTTP(t *testing.T, router http.Handler, adminToken string) listBudgetsRouteResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/budgets", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/budgets status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response listBudgetsRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode list budgets response: %v", err)
	}
	return response
}

func getBudgetStatusViaHTTP(t *testing.T, router http.Handler, adminToken string) budgetStatusRouteResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/budgets/status", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/budgets/status status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response budgetStatusRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode budget status response: %v", err)
	}
	return response
}

func insertAppBudgetCost(t *testing.T, ctx context.Context, queries *db.Queries, orgID uuid.UUID, apiKeyID uuid.UUID, providerID uuid.UUID, modelID uuid.UUID, totalMicroUSD int64, createdAt time.Time) {
	t.Helper()

	requestLogID := uuid.New()
	usageRecordID := uuid.New()
	if _, err := queries.InsertRequestLog(ctx, db.InsertRequestLogParams{
		ID:          requestLogID,
		OrgID:       orgID,
		ApiKeyID:    &apiKeyID,
		ProviderID:  &providerID,
		ModelID:     &modelID,
		Method:      "POST",
		Path:        "/v1/chat/completions",
		Status:      "success",
		StatusCode:  200,
		LatencyMs:   100,
		StartedAt:   createdAt,
		CompletedAt: createdAt,
	}); err != nil {
		t.Fatalf("InsertRequestLog returned error: %v", err)
	}
	if _, err := queries.InsertUsageRecord(ctx, db.InsertUsageRecordParams{
		ID:               usageRecordID,
		RequestLogID:     requestLogID,
		OrgID:            orgID,
		ApiKeyID:         apiKeyID,
		ProviderID:       providerID,
		ModelID:          modelID,
		PromptTokens:     1,
		CompletionTokens: 1,
		TotalTokens:      2,
		CreatedAt:        createdAt,
	}); err != nil {
		t.Fatalf("InsertUsageRecord returned error: %v", err)
	}
	if _, err := queries.InsertCostRecord(ctx, db.InsertCostRecordParams{
		ID:              uuid.New(),
		UsageRecordID:   usageRecordID,
		OrgID:           orgID,
		ProviderID:      providerID,
		ModelID:         modelID,
		Currency:        "USD",
		InputCostMicro:  totalMicroUSD,
		OutputCostMicro: 0,
		TotalCostMicro:  totalMicroUSD,
		PricingSnapshot: []byte(`{"source":"test"}`),
		CreatedAt:       createdAt,
	}); err != nil {
		t.Fatalf("InsertCostRecord returned error: %v", err)
	}
}
