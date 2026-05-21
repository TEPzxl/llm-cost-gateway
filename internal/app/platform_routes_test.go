package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/email"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
	"go.uber.org/zap"
)

func TestPlatformRoutesRequireBootstrapToken(t *testing.T) {
	router, _ := newTask5TestRouter(t)

	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing token"},
		{name: "wrong token", authorization: "Bearer wrong-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/platform/orgs", strings.NewReader(`{"name":"Demo Org","slug":"demo"}`))
			req.Header.Set("Content-Type", "application/json")
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestPlatformRoutesCreateOrganizationAndAdminToken(t *testing.T) {
	ctx := context.Background()
	router, st := newTask5TestRouter(t)

	orgID := createOrgViaHTTP(t, router, "Demo Org", "demo-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	if !strings.HasPrefix(adminToken.Token, "llmgw_admin_") {
		t.Fatalf("admin token = %q, want llmgw_admin_ prefix", adminToken.Token)
	}
	if adminToken.TokenPrefix == "" {
		t.Fatal("token_prefix is empty")
	}

	stored, err := st.Queries.GetAdminTokenByHash(ctx, auth.NewTokenHasher("token-hash-secret").Hash(adminToken.Token))
	if err != nil {
		t.Fatalf("GetAdminTokenByHash returned error: %v", err)
	}
	if stored.TokenHash == adminToken.Token {
		t.Fatal("database stored plaintext admin token")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/me status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var me adminMeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode /api/v1/admin/me response: %v", err)
	}
	if me.Org.ID != orgID {
		t.Fatalf("me org id = %s, want %s", me.Org.ID, orgID)
	}
	if me.AdminToken.ID != stored.ID {
		t.Fatalf("me admin token id = %s, want %s", me.AdminToken.ID, stored.ID)
	}
}

func TestRevokedAdminTokenCannotAccessAdminAPI(t *testing.T) {
	ctx := context.Background()
	router, st := newTask5TestRouter(t)

	orgID := createOrgViaHTTP(t, router, "Revoked Org", "revoked-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	stored, err := st.Queries.GetAdminTokenByHash(ctx, auth.NewTokenHasher("token-hash-secret").Hash(adminToken.Token))
	if err != nil {
		t.Fatalf("GetAdminTokenByHash returned error: %v", err)
	}
	now := time.Now().UTC()
	if _, err := st.Queries.RevokeAdminToken(ctx, db.RevokeAdminTokenParams{
		OrgID:     orgID,
		ID:        stored.ID,
		RevokedAt: &now,
	}); err != nil {
		t.Fatalf("RevokeAdminToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/admin/me status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestPasswordlessMockLoginNotRegisteredInProduction(t *testing.T) {
	router, _ := newTask5TestRouterWithEnv(t, "production")

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/sessions/passwordless-mock",
		strings.NewReader(`{"org_slug":"prod-org","email":"owner@example.com"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /api/v1/admin/sessions/passwordless-mock status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestPasswordlessEmailMagicLinkCreatesSession(t *testing.T) {
	ctx := context.Background()
	sender := email.NewFakeSender()
	router, st := newTask5TestRouterWithEmail(t, "production", sender)

	orgID := createOrgViaHTTP(t, router, "Magic Org", "magic-route-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	member := createMemberViaHTTP(t, router, adminToken.Token, createMemberRouteRequest{
		Email:       "owner@example.com",
		DisplayName: "Owner",
		Role:        auth.RoleOwner,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sessions/passwordless/request", strings.NewReader(`{"org_slug":"magic-route-org","email":"owner@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /sessions/passwordless/request status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	messages := sender.Messages()
	if len(messages) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(messages))
	}
	token := extractMagicRouteToken(t, messages[0].TextBody)

	verifyReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sessions/passwordless/verify", strings.NewReader(`{"token":`+strconv.Quote(token)+`}`))
	verifyReq.Header.Set("Content-Type", "application/json")
	verifyRec := httptest.NewRecorder()
	router.ServeHTTP(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("POST /sessions/passwordless/verify status = %d, want %d; body=%s", verifyRec.Code, http.StatusOK, verifyRec.Body.String())
	}
	var login passwordlessMockLoginRouteResponse
	if err := json.Unmarshal(verifyRec.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if login.Token == "" || login.MembershipID != member.MembershipID {
		t.Fatalf("login = %+v, want session for membership %s", login, member.MembershipID)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+login.Token)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/me with magic session status = %d, want %d; body=%s", meRec.Code, http.StatusOK, meRec.Body.String())
	}

	var storedHash string
	if err := st.Pool.QueryRow(ctx, `SELECT token_hash FROM magic_link_tokens WHERE org_id = $1`, orgID).Scan(&storedHash); err != nil {
		t.Fatalf("read magic link token hash: %v", err)
	}
	if strings.Contains(messages[0].TextBody, storedHash) {
		t.Fatal("email body leaked stored magic link token hash")
	}
}

type createAdminTokenResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Token       string    `json:"token"`
	TokenPrefix string    `json:"token_prefix"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type adminMeResponse struct {
	Org struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
		Slug string    `json:"slug"`
	} `json:"org"`
	AdminToken struct {
		ID     uuid.UUID `json:"id"`
		Name   string    `json:"name"`
		Scopes []string  `json:"scopes"`
	} `json:"admin_token"`
}

func newTask5TestRouter(t *testing.T) (*gin.Engine, *store.Store) {
	t.Helper()
	return newTask5TestRouterWithEnv(t, "test")
}

func newTask5TestRouterWithEnv(t *testing.T, appEnv string) (*gin.Engine, *store.Store) {
	t.Helper()
	return newTask5TestRouterWithEmail(t, appEnv, nil)
}

func newTask5TestRouterWithEmail(t *testing.T, appEnv string, sender email.Sender) (*gin.Engine, *store.Store) {
	t.Helper()

	ctx := context.Background()
	pool := testutil.OpenPostgres(t, ctx)
	st := store.New(pool)
	resetTask5TestDatabase(t, ctx, st)

	router := NewRouter(RouterConfig{
		AppEnv:                   appEnv,
		PlatformBootstrapToken:   "bootstrap-token",
		TokenHashSecret:          "token-hash-secret",
		SecretEncryptionKey:      "0123456789abcdef0123456789abcdef",
		Store:                    st,
		PasswordlessEmailEnabled: sender != nil,
		MagicLinkBaseURL:         "https://console.example.com/login",
		EmailSender:              sender,
	}, zap.NewNop())
	return router, st
}

func resetTask5TestDatabase(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()

	_, err := st.Pool.Exec(ctx, `
		TRUNCATE
			user_sessions,
			magic_link_tokens,
			org_memberships,
			users,
			cost_records,
			usage_records,
			request_logs,
			budgets,
			route_targets,
			route_policies,
			models,
			provider_secrets,
			providers,
			api_keys,
			admin_tokens,
			organizations
		CASCADE
	`)
	if err != nil {
		t.Fatalf("reset test database: %v", err)
	}
}

func createOrgViaHTTP(t *testing.T, router http.Handler, name string, slug string) uuid.UUID {
	t.Helper()

	body := []byte(`{"name":` + strconv.Quote(name) + `,"slug":` + strconv.Quote(slug) + `}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/platform/orgs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer bootstrap-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/platform/orgs status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create org response: %v", err)
	}
	return response.ID
}

func extractMagicRouteToken(t *testing.T, body string) string {
	t.Helper()
	for _, field := range strings.Fields(body) {
		if idx := strings.Index(field, "magic_token="); idx >= 0 {
			return strings.TrimSpace(field[idx+len("magic_token="):])
		}
	}
	t.Fatalf("magic_token not found in email body: %q", body)
	return ""
}

func createAdminTokenViaHTTP(t *testing.T, router http.Handler, orgID uuid.UUID) createAdminTokenResponse {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/platform/orgs/"+orgID.String()+"/admin-tokens",
		strings.NewReader(`{"name":"initial-admin-token","scopes":["admin:*"]}`),
	)
	req.Header.Set("Authorization", "Bearer bootstrap-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/platform/orgs/%s/admin-tokens status = %d, want %d; body=%s", orgID, rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response createAdminTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create admin token response: %v", err)
	}
	return response
}
