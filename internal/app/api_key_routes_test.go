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
	"github.com/tep/llm-cost-gateway/internal/auth"
)

func TestAdminAPIKeyRoutesCreateListAndRevokeKey(t *testing.T) {
	ctx := context.Background()
	router, st := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "API Key Org", "api-key-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	created := createAPIKeyViaHTTP(t, router, adminToken.Token, createAPIKeyRequest{
		Name:                     "docs-assistant-prod",
		Scopes:                   []string{"chat.completions"},
		RPMLimit:                 60,
		DailyCostLimitMicroUSD:   int64Ptr(1000),
		MonthlyCostLimitMicroUSD: int64Ptr(10_000),
		QuotaAction:              "warn",
	})
	if !strings.HasPrefix(created.Key, "llmgw_live_") {
		t.Fatalf("api key = %q, want llmgw_live_ prefix", created.Key)
	}
	if created.KeyPrefix == "" {
		t.Fatal("key_prefix is empty")
	}

	stored, err := st.Queries.GetAPIKeyByHash(ctx, auth.NewTokenHasher("token-hash-secret").Hash(created.Key))
	if err != nil {
		t.Fatalf("GetAPIKeyByHash returned error: %v", err)
	}
	if stored.KeyHash == created.Key {
		t.Fatal("database stored plaintext API key")
	}
	if stored.OrgID != orgID {
		t.Fatalf("stored org id = %s, want %s", stored.OrgID, orgID)
	}
	if !stored.DailyCostLimitMicroUsd.Valid || stored.DailyCostLimitMicroUsd.Int64 != 1000 {
		t.Fatalf("stored daily limit = %+v, want 1000", stored.DailyCostLimitMicroUsd)
	}
	if !stored.MonthlyCostLimitMicroUsd.Valid || stored.MonthlyCostLimitMicroUsd.Int64 != 10_000 {
		t.Fatalf("stored monthly limit = %+v, want 10000", stored.MonthlyCostLimitMicroUsd)
	}
	if stored.QuotaAction != "warn" {
		t.Fatalf("stored quota action = %q, want warn", stored.QuotaAction)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/api-keys status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var listBody map[string][]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list api keys response: %v", err)
	}
	items := listBody["items"]
	if len(items) != 1 {
		t.Fatalf("listed %d api keys, want 1", len(items))
	}
	if _, ok := items[0]["key"]; ok {
		t.Fatal("list response includes plaintext key")
	}
	if items[0]["key_prefix"] != created.KeyPrefix {
		t.Fatalf("listed key_prefix = %v, want %s", items[0]["key_prefix"], created.KeyPrefix)
	}
	if items[0]["daily_cost_limit_micro_usd"] != float64(1000) {
		t.Fatalf("listed daily limit = %v, want 1000", items[0]["daily_cost_limit_micro_usd"])
	}
	if items[0]["monthly_cost_limit_micro_usd"] != float64(10_000) {
		t.Fatalf("listed monthly limit = %v, want 10000", items[0]["monthly_cost_limit_micro_usd"])
	}
	if items[0]["quota_action"] != "warn" {
		t.Fatalf("listed quota_action = %v, want warn", items[0]["quota_action"])
	}

	revoked := revokeAPIKeyViaHTTP(t, router, adminToken.Token, created.ID, http.StatusOK)
	if revoked.Status != "revoked" {
		t.Fatalf("revoked status = %q, want revoked", revoked.Status)
	}
}

func TestAdminAPIKeyRoutesAreScopedByOrg(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgA := createOrgViaHTTP(t, router, "API Key Org A", "api-key-org-a")
	adminA := createAdminTokenViaHTTP(t, router, orgA)
	orgB := createOrgViaHTTP(t, router, "API Key Org B", "api-key-org-b")
	adminB := createAdminTokenViaHTTP(t, router, orgB)

	created := createAPIKeyViaHTTP(t, router, adminA.Token, createAPIKeyRequest{
		Name:     "org-a-key",
		Scopes:   []string{"chat.completions"},
		RPMLimit: 60,
	})

	revokeAPIKeyViaHTTP(t, router, adminB.Token, created.ID, http.StatusNotFound)
}

func TestAdminAPIKeyRoutesRejectInvalidRPM(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Invalid RPM Org", "invalid-rpm-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/api-keys",
		strings.NewReader(`{"name":"bad-key","scopes":["chat.completions"],"rpm_limit":0}`),
	)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/v1/admin/api-keys status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAdminAPIKeyRoutesRejectInvalidQuota(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Invalid Quota Org", "invalid-quota-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	tests := []struct {
		name string
		body string
	}{
		{
			name: "negative daily limit",
			body: `{"name":"bad-key","scopes":["chat.completions"],"rpm_limit":60,"daily_cost_limit_micro_usd":-1}`,
		},
		{
			name: "invalid action",
			body: `{"name":"bad-key","scopes":["chat.completions"],"rpm_limit":60,"quota_action":"drop"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+adminToken.Token)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("POST /api/v1/admin/api-keys status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

type createAPIKeyRequest struct {
	Name                     string     `json:"name"`
	Scopes                   []string   `json:"scopes"`
	RPMLimit                 int32      `json:"rpm_limit"`
	DailyCostLimitMicroUSD   *int64     `json:"daily_cost_limit_micro_usd"`
	MonthlyCostLimitMicroUSD *int64     `json:"monthly_cost_limit_micro_usd"`
	QuotaAction              string     `json:"quota_action"`
	ExpiresAt                *time.Time `json:"expires_at"`
}

type apiKeyCreateResponse struct {
	ID                       uuid.UUID `json:"id"`
	Name                     string    `json:"name"`
	Key                      string    `json:"key"`
	KeyPrefix                string    `json:"key_prefix"`
	Status                   string    `json:"status"`
	RPMLimit                 int32     `json:"rpm_limit"`
	DailyCostLimitMicroUSD   *int64    `json:"daily_cost_limit_micro_usd"`
	MonthlyCostLimitMicroUSD *int64    `json:"monthly_cost_limit_micro_usd"`
	QuotaAction              string    `json:"quota_action"`
	CreatedAt                time.Time `json:"created_at"`
}

type apiKeyResponse struct {
	ID                       uuid.UUID  `json:"id"`
	Name                     string     `json:"name"`
	KeyPrefix                string     `json:"key_prefix"`
	Status                   string     `json:"status"`
	RPMLimit                 int32      `json:"rpm_limit"`
	DailyCostLimitMicroUSD   *int64     `json:"daily_cost_limit_micro_usd"`
	MonthlyCostLimitMicroUSD *int64     `json:"monthly_cost_limit_micro_usd"`
	QuotaAction              string     `json:"quota_action"`
	ExpiresAt                *time.Time `json:"expires_at"`
	LastUsedAt               *time.Time `json:"last_used_at"`
	CreatedAt                time.Time  `json:"created_at"`
}

func createAPIKeyViaHTTP(t *testing.T, router http.Handler, adminToken string, body createAPIKeyRequest) apiKeyCreateResponse {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal create API key request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/admin/api-keys status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response apiKeyCreateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create api key response: %v", err)
	}
	return response
}

func revokeAPIKeyViaHTTP(t *testing.T, router http.Handler, adminToken string, apiKeyID uuid.UUID, wantStatus int) apiKeyResponse {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/api-keys/"+apiKeyID.String()+"/revoke",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != wantStatus {
		t.Fatalf("POST /api/v1/admin/api-keys/%s/revoke status = %d, want %d; body=%s", apiKeyID, rec.Code, wantStatus, rec.Body.String())
	}
	if wantStatus != http.StatusOK {
		return apiKeyResponse{}
	}

	var response apiKeyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode revoke api key response: %v", err)
	}
	return response
}

func int64Ptr(value int64) *int64 {
	return &value
}
