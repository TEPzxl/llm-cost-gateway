package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"
)

func TestAdminRBACSessionRoles(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "RBAC Org", "rbac-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	owner := createMemberViaHTTP(t, router, adminToken.Token, createMemberRouteRequest{
		Email:       "owner@example.com",
		DisplayName: "Owner",
		Role:        "owner",
	})
	adminMember := createMemberViaHTTP(t, router, adminToken.Token, createMemberRouteRequest{
		Email:       "admin@example.com",
		DisplayName: "Admin",
		Role:        "admin",
	})
	viewer := createMemberViaHTTP(t, router, adminToken.Token, createMemberRouteRequest{
		Email:       "viewer@example.com",
		DisplayName: "Viewer",
		Role:        "viewer",
	})

	ownerSession := passwordlessMockLoginViaHTTP(t, router, "rbac-org", owner.Email)
	adminSession := passwordlessMockLoginViaHTTP(t, router, "rbac-org", adminMember.Email)
	viewerSession := passwordlessMockLoginViaHTTP(t, router, "rbac-org", viewer.Email)

	assertAdminRouteStatus(t, router, ownerSession.Token, http.MethodPost, "/api/v1/admin/members", `{"email":"new-viewer@example.com","display_name":"New Viewer","role":"viewer"}`, http.StatusCreated)
	assertAdminRouteStatus(t, router, adminSession.Token, http.MethodGet, "/api/v1/admin/members", "", http.StatusOK)
	assertAdminRouteStatus(t, router, adminSession.Token, http.MethodPost, "/api/v1/admin/members", `{"email":"blocked@example.com","display_name":"Blocked","role":"viewer"}`, http.StatusForbidden)

	createProviderViaHTTP(t, router, adminSession.Token, createProviderRequest{
		Name:      "rbac-admin-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	assertAdminRouteStatus(t, router, viewerSession.Token, http.MethodGet, "/api/v1/admin/request-logs", "", http.StatusOK)
	assertAdminRouteStatus(t, router, viewerSession.Token, http.MethodGet, "/api/v1/admin/providers", "", http.StatusForbidden)
	assertAdminRouteStatus(t, router, viewerSession.Token, http.MethodPost, "/api/v1/admin/providers", `{"name":"viewer-provider","type":"mock","timeout_ms":30000}`, http.StatusForbidden)

	createAPIKeyViaHTTP(t, router, adminToken.Token, createAPIKeyRequest{
		Name:     "rbac-service-token-api-key",
		Scopes:   []string{"chat:completions"},
		RPMLimit: 60,
	})
}

func TestPasswordlessMockLoginRejectsCrossOrgMember(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgA := createOrgViaHTTP(t, router, "RBAC Org A", "rbac-org-a")
	_ = createOrgViaHTTP(t, router, "RBAC Org B", "rbac-org-b")
	adminA := createAdminTokenViaHTTP(t, router, orgA)
	createMemberViaHTTP(t, router, adminA.Token, createMemberRouteRequest{
		Email:       "member@example.com",
		DisplayName: "Member",
		Role:        "viewer",
	})

	assertPasswordlessLoginStatus(t, router, "rbac-org-a", "member@example.com", http.StatusOK)
	assertPasswordlessLoginStatus(t, router, "rbac-org-b", "member@example.com", http.StatusUnauthorized)
}

func TestAdminAuditRecordsUserSessionActor(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "RBAC Audit Org", "rbac-audit-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	member := createMemberViaHTTP(t, router, adminToken.Token, createMemberRouteRequest{
		Email:       "admin-audit@example.com",
		DisplayName: "Admin Audit",
		Role:        "admin",
	})
	session := passwordlessMockLoginViaHTTP(t, router, "rbac-audit-org", member.Email)

	createProviderViaHTTP(t, router, session.Token, createProviderRequest{
		Name:      "rbac-audit-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/audit-logs status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response struct {
		Items []struct {
			ActorAdminTokenID *uuid.UUID `json:"actor_admin_token_id"`
			ActorUserID       *uuid.UUID `json:"actor_user_id"`
			Action            string     `json:"action"`
			ResourceType      string     `json:"resource_type"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode audit logs response: %v", err)
	}
	if len(response.Items) == 0 {
		t.Fatal("audit logs response is empty")
	}
	item := response.Items[0]
	if item.ActorAdminTokenID != nil || item.ActorUserID == nil || *item.ActorUserID != member.UserID {
		t.Fatalf("audit actor = token:%v user:%v, want user %s", item.ActorAdminTokenID, item.ActorUserID, member.UserID)
	}
	if item.Action != "create_provider" || item.ResourceType != "provider" {
		t.Fatalf("audit item = %+v, want create_provider", item)
	}
}

type createMemberRouteRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type memberRouteResponse struct {
	MembershipID uuid.UUID `json:"membership_id"`
	OrgID        uuid.UUID `json:"org_id"`
	UserID       uuid.UUID `json:"user_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
}

type passwordlessMockLoginRouteResponse struct {
	Token        string    `json:"token"`
	TokenType    string    `json:"token_type"`
	OrgID        uuid.UUID `json:"org_id"`
	OrgSlug      string    `json:"org_slug"`
	UserID       uuid.UUID `json:"user_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	MembershipID uuid.UUID `json:"membership_id"`
	Role         string    `json:"role"`
}

func createMemberViaHTTP(t *testing.T, router http.Handler, token string, body createMemberRouteRequest) memberRouteResponse {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal member request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/members", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/admin/members status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var response memberRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode member response: %v", err)
	}
	return response
}

func passwordlessMockLoginViaHTTP(t *testing.T, router http.Handler, orgSlug string, email string) passwordlessMockLoginRouteResponse {
	t.Helper()

	rec := assertPasswordlessLoginStatus(t, router, orgSlug, email, http.StatusOK)
	var response passwordlessMockLoginRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode passwordless login response: %v", err)
	}
	if response.Token == "" {
		t.Fatal("passwordless login token is empty")
	}
	return response
}

func assertPasswordlessLoginStatus(t *testing.T, router http.Handler, orgSlug string, email string, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()

	body := `{"org_slug":` + strconv.Quote(orgSlug) + `,"email":` + strconv.Quote(email) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sessions/passwordless-mock", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("POST /api/v1/admin/sessions/passwordless-mock status = %d, want %d; body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	return rec
}

func assertAdminRouteStatus(t *testing.T, router http.Handler, token string, method string, path string, body string, wantStatus int) {
	t.Helper()

	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
}
