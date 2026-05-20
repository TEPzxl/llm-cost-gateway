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
	secretcrypto "github.com/tep/llm-cost-gateway/internal/crypto"
	"github.com/tep/llm-cost-gateway/internal/domain"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

func TestAdminProviderRoutesCreateMockAndOpenAIProvider(t *testing.T) {
	ctx := context.Background()
	router, st := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Provider Org", "provider-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	mockProvider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "mock-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	if mockProvider.Type != "mock" {
		t.Fatalf("mock provider type = %q, want mock", mockProvider.Type)
	}

	openAIProvider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "deepseek-compatible",
		Type:      "openai_compatible",
		BaseURL:   stringPtr("https://api.example.com/v1"),
		APIKey:    stringPtr("provider-secret-key"),
		TimeoutMS: 30000,
	})
	if openAIProvider.APIKey != nil {
		t.Fatal("create provider response returned plaintext api_key")
	}
	if openAIProvider.BaseURL == nil || *openAIProvider.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("base_url = %v, want https://api.example.com/v1", openAIProvider.BaseURL)
	}

	secret, err := st.Queries.GetProviderSecret(ctx, db.GetProviderSecretParams{
		OrgID:      orgID,
		ProviderID: openAIProvider.ID,
	})
	if err != nil {
		t.Fatalf("GetProviderSecret returned error: %v", err)
	}
	if secret.EncryptedApiKey == "provider-secret-key" {
		t.Fatal("provider api key was stored in plaintext")
	}

	box, err := secretcrypto.NewSecretBox("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretBox returned error: %v", err)
	}
	decrypted, err := box.Open(secret.EncryptedApiKey, secret.Nonce)
	if err != nil {
		t.Fatalf("Open provider secret returned error: %v", err)
	}
	if decrypted != "provider-secret-key" {
		t.Fatalf("decrypted provider secret = %q, want provider-secret-key", decrypted)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/providers", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/providers status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var listBody map[string][]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list providers response: %v", err)
	}
	if len(listBody["items"]) != 2 {
		t.Fatalf("listed %d providers, want 2", len(listBody["items"]))
	}
	for _, item := range listBody["items"] {
		if _, ok := item["api_key"]; ok {
			t.Fatal("list providers response includes api_key")
		}
	}
}

func TestAdminProviderRoutesRejectInvalidOpenAIProvider(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Invalid Provider Org", "invalid-provider-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/providers",
		strings.NewReader(`{"name":"bad-provider","type":"openai_compatible","base_url":"https://api.example.com/v1","timeout_ms":30000}`),
	)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/v1/admin/providers status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAdminProviderHealthRoutesCheckMockAndList(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Provider Health Org", "provider-health-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	provider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "mock-health-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/"+provider.ID.String()+"/health-check", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST provider health status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var checked providerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &checked); err != nil {
		t.Fatalf("decode provider health response: %v", err)
	}
	if checked.LastHealthStatus == nil || *checked.LastHealthStatus != "healthy" {
		t.Fatalf("last_health_status = %v, want healthy", checked.LastHealthStatus)
	}
	if checked.LastHealthCheckedAt == nil {
		t.Fatal("last_health_checked_at is nil, want timestamp")
	}
	if checked.LastErrorCode != nil || checked.LastErrorMessage != nil {
		t.Fatalf("health errors = %v %v, want nil", checked.LastErrorCode, checked.LastErrorMessage)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/providers/health", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken.Token)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("GET provider health status = %d, want %d; body=%s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var listBody struct {
		Items []providerResponse `json:"items"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode provider health list: %v", err)
	}
	if len(listBody.Items) != 1 || listBody.Items[0].LastHealthStatus == nil || *listBody.Items[0].LastHealthStatus != "healthy" {
		t.Fatalf("provider health list = %+v, want one healthy provider", listBody.Items)
	}
}

func TestAdminProviderHealthRoutesOpenAICompatibleFailure(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Provider Health Failure Org", "provider-health-failure-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	provider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "openai-health-provider",
		Type:      "openai_compatible",
		BaseURL:   stringPtr(server.URL + "/v1"),
		APIKey:    stringPtr("provider-secret-key"),
		TimeoutMS: 1000,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/"+provider.ID.String()+"/health-check", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST provider health status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var checked providerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &checked); err != nil {
		t.Fatalf("decode provider health response: %v", err)
	}
	if checked.LastHealthStatus == nil || *checked.LastHealthStatus != "unhealthy" {
		t.Fatalf("last_health_status = %v, want unhealthy", checked.LastHealthStatus)
	}
	if checked.LastErrorCode == nil || *checked.LastErrorCode != domain.CodeProviderUnavailable {
		t.Fatalf("last_error_code = %v, want %s", checked.LastErrorCode, domain.CodeProviderUnavailable)
	}
	if checked.LastErrorMessage == nil || !strings.Contains(*checked.LastErrorMessage, "status 500") {
		t.Fatalf("last_error_message = %v, want status 500", checked.LastErrorMessage)
	}
}

func TestAdminModelRoutesCreateAndListModel(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Model Org", "model-org")
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
		ContextWindow:                  int32Ptr(8192),
	})
	if model.ProviderID != provider.ID {
		t.Fatalf("model provider_id = %s, want %s", model.ProviderID, provider.ID)
	}
	if model.InputPriceMicroUSDPer1KTokens != 100 {
		t.Fatalf("input price = %d, want 100", model.InputPriceMicroUSDPer1KTokens)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/models", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/models status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var listBody struct {
		Items []modelResponse `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list models response: %v", err)
	}
	if len(listBody.Items) != 1 || listBody.Items[0].ID != model.ID {
		t.Fatalf("listed models = %+v, want only %s", listBody.Items, model.ID)
	}
}

func TestAdminModelRoutesUpdatePricingAndListVersions(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Model Pricing Org", "model-pricing-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	provider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "pricing-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	model := createModelViaHTTP(t, router, adminToken.Token, createModelRequest{
		ProviderID:                     provider.ID,
		ProviderModelName:              "pricing-small",
		DisplayName:                    "Pricing Small",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})

	initial := listModelPricingVersionsViaHTTP(t, router, adminToken.Token, model.ID, http.StatusOK)
	if len(initial.Items) != 1 || initial.Items[0].Version != 1 || initial.Items[0].Status != "active" {
		t.Fatalf("initial pricing versions = %+v, want one active version 1", initial.Items)
	}

	updated := updateModelPricingViaHTTP(t, router, adminToken.Token, model.ID, updateModelPricingRequest{
		InputPriceMicroUSDPer1KTokens:  300,
		OutputPriceMicroUSDPer1KTokens: 400,
	}, http.StatusOK)
	if updated.Model.InputPriceMicroUSDPer1KTokens != 300 || updated.Model.OutputPriceMicroUSDPer1KTokens != 400 {
		t.Fatalf("updated model prices = %d/%d, want 300/400", updated.Model.InputPriceMicroUSDPer1KTokens, updated.Model.OutputPriceMicroUSDPer1KTokens)
	}
	if updated.PricingVersion.Version != 2 || updated.PricingVersion.Status != "active" {
		t.Fatalf("updated pricing version = %+v, want version 2 active", updated.PricingVersion)
	}
	if updated.PreviousVersion.Version != 1 || updated.PreviousVersion.Status != "superseded" || updated.PreviousVersion.EffectiveTo == nil {
		t.Fatalf("previous pricing version = %+v, want version 1 superseded with effective_to", updated.PreviousVersion)
	}

	versions := listModelPricingVersionsViaHTTP(t, router, adminToken.Token, model.ID, http.StatusOK)
	if len(versions.Items) != 2 {
		t.Fatalf("pricing version count = %d, want 2", len(versions.Items))
	}
	if versions.Items[0].Version != 2 || versions.Items[0].Status != "active" {
		t.Fatalf("latest pricing version = %+v, want version 2 active", versions.Items[0])
	}
	if versions.Items[1].Version != 1 || versions.Items[1].Status != "superseded" {
		t.Fatalf("old pricing version = %+v, want version 1 superseded", versions.Items[1])
	}
}

func TestAdminModelRoutesRejectCrossOrgPricingUpdate(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgA := createOrgViaHTTP(t, router, "Pricing Org A", "pricing-org-a")
	adminA := createAdminTokenViaHTTP(t, router, orgA)
	orgB := createOrgViaHTTP(t, router, "Pricing Org B", "pricing-org-b")
	adminB := createAdminTokenViaHTTP(t, router, orgB)
	providerA := createProviderViaHTTP(t, router, adminA.Token, createProviderRequest{
		Name:      "org-a-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	modelA := createModelViaHTTP(t, router, adminA.Token, createModelRequest{
		ProviderID:                     providerA.ID,
		ProviderModelName:              "org-a-model",
		DisplayName:                    "Org A Model",
		InputPriceMicroUSDPer1KTokens:  100,
		OutputPriceMicroUSDPer1KTokens: 200,
	})

	updateModelPricingViaHTTP(t, router, adminB.Token, modelA.ID, updateModelPricingRequest{
		InputPriceMicroUSDPer1KTokens:  300,
		OutputPriceMicroUSDPer1KTokens: 400,
	}, http.StatusNotFound)
	listModelPricingVersionsViaHTTP(t, router, adminB.Token, modelA.ID, http.StatusNotFound)
}

func TestAdminModelRoutesRejectNegativePrice(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Negative Price Org", "negative-price-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	provider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "mock-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})

	payload := `{"provider_id":"` + provider.ID.String() + `","provider_model_name":"bad-model","display_name":"Bad Model","input_price_micro_usd_per_1k_tokens":-1,"output_price_micro_usd_per_1k_tokens":200}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/v1/admin/models status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAdminModelRoutesRejectCrossOrgProvider(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgA := createOrgViaHTTP(t, router, "Provider Owner Org", "provider-owner-org")
	adminA := createAdminTokenViaHTTP(t, router, orgA)
	orgB := createOrgViaHTTP(t, router, "Model Caller Org", "model-caller-org")
	adminB := createAdminTokenViaHTTP(t, router, orgB)

	providerA := createProviderViaHTTP(t, router, adminA.Token, createProviderRequest{
		Name:      "org-a-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})

	payload := `{"provider_id":"` + providerA.ID.String() + `","provider_model_name":"cross-org-model","display_name":"Cross Org Model","input_price_micro_usd_per_1k_tokens":100,"output_price_micro_usd_per_1k_tokens":200}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+adminB.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /api/v1/admin/models status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

type createProviderRequest struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	BaseURL   *string `json:"base_url"`
	APIKey    *string `json:"api_key"`
	TimeoutMS int32   `json:"timeout_ms"`
}

type providerResponse struct {
	ID                  uuid.UUID  `json:"id"`
	Name                string     `json:"name"`
	Type                string     `json:"type"`
	BaseURL             *string    `json:"base_url"`
	APIKey              *string    `json:"api_key"`
	Status              string     `json:"status"`
	TimeoutMS           int32      `json:"timeout_ms"`
	LastHealthStatus    *string    `json:"last_health_status"`
	LastHealthCheckedAt *time.Time `json:"last_health_checked_at"`
	LastErrorCode       *string    `json:"last_error_code"`
	LastErrorMessage    *string    `json:"last_error_message"`
	CreatedAt           time.Time  `json:"created_at"`
}

type createModelRequest struct {
	ProviderID                     uuid.UUID `json:"provider_id"`
	ProviderModelName              string    `json:"provider_model_name"`
	DisplayName                    string    `json:"display_name"`
	InputPriceMicroUSDPer1KTokens  int64     `json:"input_price_micro_usd_per_1k_tokens"`
	OutputPriceMicroUSDPer1KTokens int64     `json:"output_price_micro_usd_per_1k_tokens"`
	ContextWindow                  *int32    `json:"context_window"`
}

type modelResponse struct {
	ID                             uuid.UUID `json:"id"`
	ProviderID                     uuid.UUID `json:"provider_id"`
	ProviderModelName              string    `json:"provider_model_name"`
	DisplayName                    string    `json:"display_name"`
	InputPriceMicroUSDPer1KTokens  int64     `json:"input_price_micro_usd_per_1k_tokens"`
	OutputPriceMicroUSDPer1KTokens int64     `json:"output_price_micro_usd_per_1k_tokens"`
	ContextWindow                  *int32    `json:"context_window"`
	Status                         string    `json:"status"`
	CreatedAt                      time.Time `json:"created_at"`
}

type updateModelPricingRequest struct {
	InputPriceMicroUSDPer1KTokens  int64 `json:"input_price_micro_usd_per_1k_tokens"`
	OutputPriceMicroUSDPer1KTokens int64 `json:"output_price_micro_usd_per_1k_tokens"`
}

type modelPricingVersionResponse struct {
	ID                             uuid.UUID  `json:"id"`
	ModelID                        uuid.UUID  `json:"model_id"`
	Version                        int32      `json:"version"`
	InputPriceMicroUSDPer1KTokens  int64      `json:"input_price_micro_usd_per_1k_tokens"`
	OutputPriceMicroUSDPer1KTokens int64      `json:"output_price_micro_usd_per_1k_tokens"`
	Status                         string     `json:"status"`
	EffectiveFrom                  time.Time  `json:"effective_from"`
	EffectiveTo                    *time.Time `json:"effective_to"`
	CreatedAt                      time.Time  `json:"created_at"`
}

type updateModelPricingResponse struct {
	Model           modelResponse               `json:"model"`
	PreviousVersion modelPricingVersionResponse `json:"previous_version"`
	PricingVersion  modelPricingVersionResponse `json:"pricing_version"`
}

type listModelPricingVersionsResponse struct {
	Items []modelPricingVersionResponse `json:"items"`
}

func createProviderViaHTTP(t *testing.T, router http.Handler, adminToken string, body createProviderRequest) providerResponse {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal create provider request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/admin/providers status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response providerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create provider response: %v", err)
	}
	return response
}

func createModelViaHTTP(t *testing.T, router http.Handler, adminToken string, body createModelRequest) modelResponse {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal create model request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/admin/models status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response modelResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create model response: %v", err)
	}
	return response
}

func updateModelPricingViaHTTP(t *testing.T, router http.Handler, adminToken string, modelID uuid.UUID, body updateModelPricingRequest, wantStatus int) updateModelPricingResponse {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal update model pricing request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/models/"+modelID.String()+"/pricing", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != wantStatus {
		t.Fatalf("PATCH /api/v1/admin/models/%s/pricing status = %d, want %d; body=%s", modelID, rec.Code, wantStatus, rec.Body.String())
	}
	if wantStatus != http.StatusOK {
		return updateModelPricingResponse{}
	}

	var response updateModelPricingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode update model pricing response: %v", err)
	}
	return response
}

func listModelPricingVersionsViaHTTP(t *testing.T, router http.Handler, adminToken string, modelID uuid.UUID, wantStatus int) listModelPricingVersionsResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/models/"+modelID.String()+"/pricing-versions", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != wantStatus {
		t.Fatalf("GET /api/v1/admin/models/%s/pricing-versions status = %d, want %d; body=%s", modelID, rec.Code, wantStatus, rec.Body.String())
	}
	if wantStatus != http.StatusOK {
		return listModelPricingVersionsResponse{}
	}

	var response listModelPricingVersionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode list model pricing versions response: %v", err)
	}
	return response
}

func stringPtr(value string) *string {
	return &value
}

func int32Ptr(value int32) *int32 {
	return &value
}
