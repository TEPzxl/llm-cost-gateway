package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAdminAuditLogsCreatedForProviderAndBudget(t *testing.T) {
	ctx := context.Background()
	router, st := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Audit Org", "audit-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)

	provider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "audit-provider",
		Type:      "mock",
		TimeoutMS: 30000,
	})
	budget := createBudgetViaHTTP(t, router, adminToken.Token, createBudgetRequest{
		Name:          "audit-budget",
		ScopeType:     "org",
		Period:        "daily",
		LimitMicroUSD: 1000,
		Action:        "warn",
	})

	rows, err := st.Pool.Query(ctx, `
		SELECT actor_admin_token_id, action, resource_type, resource_id, request_id
		FROM admin_audit_logs
		WHERE org_id = $1
	`, orgID)
	if err != nil {
		t.Fatalf("query admin_audit_logs: %v", err)
	}
	defer rows.Close()

	seen := map[string]struct {
		actorAdminTokenID uuid.UUID
		action            string
		resourceID        uuid.UUID
		requestID         string
	}{}
	for rows.Next() {
		var resourceType string
		var item struct {
			actorAdminTokenID uuid.UUID
			action            string
			resourceID        uuid.UUID
			requestID         string
		}
		if err := rows.Scan(&item.actorAdminTokenID, &item.action, &resourceType, &item.resourceID, &item.requestID); err != nil {
			t.Fatalf("scan admin audit log: %v", err)
		}
		seen[resourceType] = item
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate admin audit logs: %v", err)
	}

	assertAuditLog(t, seen, "provider", "create_provider", adminToken.ID, provider.ID)
	assertAuditLog(t, seen, "budget", "create_budget", adminToken.ID, budget.ID)

	var leaked int
	if err := st.Pool.QueryRow(ctx, `
		SELECT count(*)::int
		FROM admin_audit_logs
		WHERE action LIKE '%' || $1 || '%'
		   OR resource_type LIKE '%' || $1 || '%'
		   OR request_id LIKE '%' || $1 || '%'
	`, adminToken.Token).Scan(&leaked); err != nil {
		t.Fatalf("search admin_audit_logs for raw token: %v", err)
	}
	if leaked != 0 {
		t.Fatalf("found %d audit log rows containing raw admin token", leaked)
	}
}

func TestAdminAuditLogsRouteListsAuditLogs(t *testing.T) {
	router, _ := newTask5TestRouter(t)
	orgID := createOrgViaHTTP(t, router, "Audit List Org", "audit-list-org")
	adminToken := createAdminTokenViaHTTP(t, router, orgID)
	provider := createProviderViaHTTP(t, router, adminToken.Token, createProviderRequest{
		Name:      "audit-list-provider",
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
			ActorAdminTokenID uuid.UUID  `json:"actor_admin_token_id"`
			Action            string     `json:"action"`
			ResourceType      string     `json:"resource_type"`
			ResourceID        *uuid.UUID `json:"resource_id"`
			RequestID         string     `json:"request_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode audit logs response: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("audit log count = %d, want 1", len(response.Items))
	}
	item := response.Items[0]
	if item.ActorAdminTokenID != adminToken.ID || item.Action != "create_provider" || item.ResourceType != "provider" {
		t.Fatalf("audit log item = %+v, want create_provider by %s", item, adminToken.ID)
	}
	if item.ResourceID == nil || *item.ResourceID != provider.ID {
		t.Fatalf("audit log resource_id = %v, want %s", item.ResourceID, provider.ID)
	}
	if item.RequestID == "" {
		t.Fatal("audit log request_id is empty")
	}
	if strings.Contains(rec.Body.String(), adminToken.Token) {
		t.Fatal("audit log response contains raw token")
	}
}

func assertAuditLog(t *testing.T, seen map[string]struct {
	actorAdminTokenID uuid.UUID
	action            string
	resourceID        uuid.UUID
	requestID         string
}, resourceType string, wantAction string, wantActor uuid.UUID, wantResource uuid.UUID) {
	t.Helper()

	item, ok := seen[resourceType]
	if !ok {
		t.Fatalf("missing audit log for resource_type %q", resourceType)
	}
	if item.actorAdminTokenID != wantActor {
		t.Fatalf("%s actor_admin_token_id = %s, want %s", resourceType, item.actorAdminTokenID, wantActor)
	}
	if item.action != wantAction {
		t.Fatalf("%s action = %q, want %q", resourceType, item.action, wantAction)
	}
	if item.resourceID != wantResource {
		t.Fatalf("%s resource_id = %s, want %s", resourceType, item.resourceID, wantResource)
	}
	if item.requestID == "" {
		t.Fatalf("%s request_id is empty", resourceType)
	}
}
