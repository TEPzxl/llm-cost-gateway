package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

const healthTestSecretKey = "0123456789abcdef0123456789abcdef"

func TestHealthServiceCheckMockProviderUpdatesHealthy(t *testing.T) {
	ctx := context.Background()
	st := newHealthTestStore(t, ctx)
	orgID := createHealthTestOrg(t, ctx, st)
	providerService := NewService(st, healthTestSecretKey)
	provider, err := providerService.CreateProvider(ctx, CreateProviderParams{
		OrgID:     orgID,
		Name:      "mock-health-provider",
		Type:      TypeMock,
		TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("CreateProvider returned error: %v", err)
	}

	healthService := NewHealthService(st, healthTestSecretKey)
	checked, err := healthService.Check(ctx, orgID, provider.ID)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if !checked.LastHealthStatus.Valid || checked.LastHealthStatus.String != HealthStatusHealthy {
		t.Fatalf("last health status = %+v, want healthy", checked.LastHealthStatus)
	}
	if checked.LastHealthCheckedAt == nil {
		t.Fatal("last health checked at is nil, want timestamp")
	}
	if checked.LastErrorCode.Valid || checked.LastErrorMessage.Valid {
		t.Fatalf("last errors = %+v %+v, want empty", checked.LastErrorCode, checked.LastErrorMessage)
	}
}

func TestHealthServiceCheckOpenAICompatibleFailureUpdatesUnhealthy(t *testing.T) {
	ctx := context.Background()
	st := newHealthTestStore(t, ctx)
	orgID := createHealthTestOrg(t, ctx, st)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	providerService := NewService(st, healthTestSecretKey)
	provider, err := providerService.CreateProvider(ctx, CreateProviderParams{
		OrgID:     orgID,
		Name:      "openai-health-provider",
		Type:      TypeOpenAICompatible,
		BaseURL:   stringPtr(server.URL + "/v1"),
		APIKey:    stringPtr("provider-secret-key"),
		TimeoutMS: 1000,
	})
	if err != nil {
		t.Fatalf("CreateProvider returned error: %v", err)
	}

	healthService := NewHealthService(st, healthTestSecretKey)
	checked, err := healthService.Check(ctx, orgID, provider.ID)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if !checked.LastHealthStatus.Valid || checked.LastHealthStatus.String != HealthStatusUnhealthy {
		t.Fatalf("last health status = %+v, want unhealthy", checked.LastHealthStatus)
	}
	if !checked.LastErrorCode.Valid || checked.LastErrorCode.String != domain.CodeProviderUnavailable {
		t.Fatalf("last error code = %+v, want %s", checked.LastErrorCode, domain.CodeProviderUnavailable)
	}
	if !checked.LastErrorMessage.Valid || !strings.Contains(checked.LastErrorMessage.String, "status 500") {
		t.Fatalf("last error message = %+v, want status 500", checked.LastErrorMessage)
	}
}

func newHealthTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()

	st := store.New(testutil.OpenPostgres(t, ctx))
	_, err := st.Pool.Exec(ctx, `
		TRUNCATE
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
	return st
}

func createHealthTestOrg(t *testing.T, ctx context.Context, st *store.Store) uuid.UUID {
	t.Helper()

	now := time.Now().UTC()
	orgID := uuid.New()
	_, err := st.Queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		ID:        orgID,
		Name:      "Health Test Org",
		Slug:      "health-test-" + uuid.NewString(),
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrganization returned error: %v", err)
	}
	return orgID
}

func stringPtr(value string) *string {
	return &value
}
