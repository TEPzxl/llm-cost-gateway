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
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	BaseURL   *string   `json:"base_url"`
	APIKey    *string   `json:"api_key"`
	Status    string    `json:"status"`
	TimeoutMS int32     `json:"timeout_ms"`
	CreatedAt time.Time `json:"created_at"`
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

func stringPtr(value string) *string {
	return &value
}

func int32Ptr(value int32) *int32 {
	return &value
}
