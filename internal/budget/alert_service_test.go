package budget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

func TestValidateWebhookURLRejectsUnsafePublicOutboundURL(t *testing.T) {
	if err := validateWebhookURL("https://127.0.0.1/hook", true); err == nil {
		t.Fatal("validateWebhookURL returned nil error, want unsafe URL rejection")
	}
}

func TestAlertServiceDeliversThresholdOnceAndNextThresholdLater(t *testing.T) {
	ctx := context.Background()
	st := openBudgetTestStore(t, ctx)
	resetBudgetTestDatabase(t, ctx, st)
	orgID := createBudgetTestOrg(t, ctx, st)
	fixture := createBudgetCostFixture(t, ctx, st, orgID)
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	var signatures []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		signatures = append(signatures, r.Header.Get("X-LLMGW-Signature"))
		var payload webhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode webhook payload: %v", err)
		}
		if payload.OrgID != orgID || payload.Threshold == 0 || payload.UsedMicroUSD == 0 {
			t.Fatalf("unexpected payload = %+v", payload)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	budgetService := NewService(st.Queries, WithClock(func() time.Time { return now }))
	budgetItem, err := budgetService.CreateBudget(ctx, CreateBudgetParams{
		OrgID:         orgID,
		Name:          "monthly budget",
		ScopeType:     ScopeTypeOrg,
		Period:        PeriodMonthly,
		LimitMicroUSD: 1000,
		Action:        ActionWarn,
	})
	if err != nil {
		t.Fatalf("CreateBudget returned error: %v", err)
	}
	alertService := NewAlertService(st, WithAlertClock(func() time.Time { return now }))
	if _, err := alertService.CreateAlert(ctx, CreateAlertParams{
		OrgID:         orgID,
		BudgetID:      budgetItem.ID,
		WebhookURL:    server.URL,
		WebhookSecret: "secret",
		Status:        StatusActive,
	}); err != nil {
		t.Fatalf("CreateAlert returned error: %v", err)
	}

	insertBudgetCost(t, ctx, st, fixture, 800, now.Add(-time.Hour))
	result, err := alertService.CheckAndDeliver(ctx, orgID)
	if err != nil {
		t.Fatalf("CheckAndDeliver returned error: %v", err)
	}
	if len(result.Deliveries) != 1 || result.Deliveries[0].Threshold != 80 {
		t.Fatalf("deliveries = %+v, want one 80 delivery", result.Deliveries)
	}
	if calls.Load() != 1 {
		t.Fatalf("webhook calls = %d, want 1", calls.Load())
	}
	if len(signatures) != 1 || signatures[0] == "" {
		t.Fatalf("signatures = %+v, want one signature", signatures)
	}

	result, err = alertService.CheckAndDeliver(ctx, orgID)
	if err != nil {
		t.Fatalf("repeat CheckAndDeliver returned error: %v", err)
	}
	if len(result.Deliveries) != 0 {
		t.Fatalf("repeat deliveries = %+v, want none", result.Deliveries)
	}
	if calls.Load() != 1 {
		t.Fatalf("webhook calls after repeat = %d, want 1", calls.Load())
	}

	insertBudgetCost(t, ctx, st, fixture, 100, now.Add(-30*time.Minute))
	result, err = alertService.CheckAndDeliver(ctx, orgID)
	if err != nil {
		t.Fatalf("second threshold CheckAndDeliver returned error: %v", err)
	}
	if len(result.Deliveries) != 1 || result.Deliveries[0].Threshold != 90 {
		t.Fatalf("deliveries = %+v, want one 90 delivery", result.Deliveries)
	}
	if calls.Load() != 2 {
		t.Fatalf("webhook calls after 90 = %d, want 2", calls.Load())
	}
}

func TestAlertServiceRecordsFailedDelivery(t *testing.T) {
	ctx := context.Background()
	st := openBudgetTestStore(t, ctx)
	resetBudgetTestDatabase(t, ctx, st)
	orgID := createBudgetTestOrg(t, ctx, st)
	fixture := createBudgetCostFixture(t, ctx, st, orgID)
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	budgetItem := createAlertTestBudget(t, ctx, st, orgID, now)
	alertService := NewAlertService(st, WithAlertClock(func() time.Time { return now }))
	if _, err := alertService.CreateAlert(ctx, CreateAlertParams{
		OrgID:      orgID,
		BudgetID:   budgetItem.ID,
		WebhookURL: server.URL,
		Status:     StatusActive,
	}); err != nil {
		t.Fatalf("CreateAlert returned error: %v", err)
	}
	insertBudgetCost(t, ctx, st, fixture, 1000, now.Add(-time.Hour))

	result, err := alertService.CheckAndDeliver(ctx, orgID)
	if err != nil {
		t.Fatalf("CheckAndDeliver returned error: %v", err)
	}
	if len(result.Deliveries) != 3 {
		t.Fatalf("deliveries = %d, want 3 thresholds", len(result.Deliveries))
	}
	for _, delivery := range result.Deliveries {
		if delivery.Status != AlertDeliveryFailed {
			t.Fatalf("delivery status = %q, want failed", delivery.Status)
		}
		if !delivery.HttpStatus.Valid || delivery.HttpStatus.Int32 != http.StatusInternalServerError {
			t.Fatalf("http status = %+v, want 500", delivery.HttpStatus)
		}
	}
}

func TestAlertServiceSkipsDisabledAlert(t *testing.T) {
	ctx := context.Background()
	st := openBudgetTestStore(t, ctx)
	resetBudgetTestDatabase(t, ctx, st)
	orgID := createBudgetTestOrg(t, ctx, st)
	fixture := createBudgetCostFixture(t, ctx, st, orgID)
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	budgetItem := createAlertTestBudget(t, ctx, st, orgID, now)
	alertService := NewAlertService(st, WithAlertClock(func() time.Time { return now }))
	if _, err := alertService.CreateAlert(ctx, CreateAlertParams{
		OrgID:      orgID,
		BudgetID:   budgetItem.ID,
		WebhookURL: server.URL,
		Status:     StatusDisabled,
	}); err != nil {
		t.Fatalf("CreateAlert returned error: %v", err)
	}
	insertBudgetCost(t, ctx, st, fixture, 1000, now.Add(-time.Hour))

	result, err := alertService.CheckAndDeliver(ctx, orgID)
	if err != nil {
		t.Fatalf("CheckAndDeliver returned error: %v", err)
	}
	if len(result.Deliveries) != 0 {
		t.Fatalf("deliveries = %+v, want none", result.Deliveries)
	}
	if calls.Load() != 0 {
		t.Fatalf("webhook calls = %d, want 0", calls.Load())
	}
}

func createAlertTestBudget(t *testing.T, ctx context.Context, st *store.Store, orgID uuid.UUID, now time.Time) db.Budget {
	t.Helper()

	service := NewService(st.Queries, WithClock(func() time.Time { return now }))
	budgetItem, err := service.CreateBudget(ctx, CreateBudgetParams{
		OrgID:         orgID,
		Name:          "alert budget",
		ScopeType:     ScopeTypeOrg,
		Period:        PeriodMonthly,
		LimitMicroUSD: 1000,
		Action:        ActionWarn,
	})
	if err != nil {
		t.Fatalf("CreateBudget returned error: %v", err)
	}
	return budgetItem
}
