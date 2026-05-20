package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

func TestAPIKeyQuotaServiceCheck(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 21, 13, 30, 0, 0, time.UTC)

	tests := []struct {
		name            string
		dailyLimit      pgtype.Int8
		monthlyLimit    pgtype.Int8
		action          string
		costs           []quotaCostInput
		wantAllowed     bool
		wantWarning     bool
		wantExceeded    bool
		wantPeriod      string
		wantUsed        int64
		wantRemaining   int64
		includeOtherOrg bool
	}{
		{
			name:          "under quota is allowed",
			dailyLimit:    int8Value(1000),
			action:        "block",
			costs:         []quotaCostInput{{amount: 999, at: now.Add(-time.Hour)}},
			wantAllowed:   true,
			wantRemaining: 1,
			wantUsed:      999,
		},
		{
			name:         "daily quota exceeded blocks",
			dailyLimit:   int8Value(1000),
			action:       "block",
			costs:        []quotaCostInput{{amount: 1000, at: now.Add(-time.Hour)}},
			wantAllowed:  false,
			wantExceeded: true,
			wantPeriod:   "daily",
			wantUsed:     1000,
		},
		{
			name:         "monthly quota exceeded blocks",
			monthlyLimit: int8Value(2000),
			action:       "block",
			costs: []quotaCostInput{
				{amount: 1500, at: now.AddDate(0, 0, -2)},
				{amount: 500, at: now.Add(-time.Hour)},
			},
			wantAllowed:  false,
			wantExceeded: true,
			wantPeriod:   "monthly",
			wantUsed:     2000,
		},
		{
			name:         "warn quota does not block",
			dailyLimit:   int8Value(1000),
			action:       "warn",
			costs:        []quotaCostInput{{amount: 1000, at: now.Add(-time.Hour)}},
			wantAllowed:  true,
			wantWarning:  true,
			wantExceeded: true,
			wantPeriod:   "daily",
			wantUsed:     1000,
		},
		{
			name:            "other org cost is ignored",
			dailyLimit:      int8Value(1000),
			action:          "block",
			includeOtherOrg: true,
			costs:           []quotaCostInput{{amount: 100, at: now.Add(-time.Hour)}},
			wantAllowed:     true,
			wantUsed:        100,
			wantRemaining:   900,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := openAPIKeyTestStore(t, ctx)
			resetAuthTestDatabase(t, ctx, st)
			org := createAuthTestOrganization(t, ctx, st, "quota-"+uuid.NewString())
			fixture := createQuotaFixture(t, ctx, st, org.ID, tt.dailyLimit, tt.monthlyLimit, tt.action)
			for _, cost := range tt.costs {
				insertQuotaCost(t, ctx, st, fixture, cost.amount, cost.at)
			}
			if tt.includeOtherOrg {
				otherOrg := createAuthTestOrganization(t, ctx, st, "quota-other-"+uuid.NewString())
				otherFixture := createQuotaFixture(t, ctx, st, otherOrg.ID, tt.dailyLimit, tt.monthlyLimit, tt.action)
				insertQuotaCost(t, ctx, st, otherFixture, 5000, now.Add(-time.Hour))
			}

			service := NewAPIKeyQuotaService(st.Queries, WithAPIKeyQuotaClock(func() time.Time { return now }))
			result, err := service.Check(ctx, APIKeyPrincipal{
				OrgID:    org.ID,
				APIKeyID: fixture.APIKeyID,
				Scopes:   []string{"chat.completions"},
				RPMLimit: 60,
			})
			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}
			if result.Allowed != tt.wantAllowed || result.Warning != tt.wantWarning || result.Exceeded != tt.wantExceeded {
				t.Fatalf("result flags = allowed:%t warning:%t exceeded:%t, want allowed:%t warning:%t exceeded:%t",
					result.Allowed, result.Warning, result.Exceeded, tt.wantAllowed, tt.wantWarning, tt.wantExceeded)
			}
			if tt.wantPeriod != "" {
				if result.Status == nil {
					t.Fatal("result status is nil")
				}
				if result.Status.Period != tt.wantPeriod {
					t.Fatalf("period = %q, want %q", result.Status.Period, tt.wantPeriod)
				}
				if result.Status.UsedMicroUSD != tt.wantUsed {
					t.Fatalf("used = %d, want %d", result.Status.UsedMicroUSD, tt.wantUsed)
				}
				return
			}
			if result.Status != nil {
				if result.Status.UsedMicroUSD != tt.wantUsed {
					t.Fatalf("used = %d, want %d", result.Status.UsedMicroUSD, tt.wantUsed)
				}
				if result.Status.RemainingMicroUSD != tt.wantRemaining {
					t.Fatalf("remaining = %d, want %d", result.Status.RemainingMicroUSD, tt.wantRemaining)
				}
			}
		})
	}
}

type quotaCostInput struct {
	amount int64
	at     time.Time
}

type quotaFixture struct {
	OrgID      uuid.UUID
	APIKeyID   uuid.UUID
	ProviderID uuid.UUID
	ModelID    uuid.UUID
}

func createQuotaFixture(t *testing.T, ctx context.Context, st *store.Store, orgID uuid.UUID, dailyLimit pgtype.Int8, monthlyLimit pgtype.Int8, action string) quotaFixture {
	t.Helper()

	now := time.Now().UTC()
	apiKey, err := st.Queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ID:                       uuid.New(),
		OrgID:                    orgID,
		Name:                     "quota-key",
		KeyPrefix:                "llmgw_live_test",
		KeyHash:                  "quota-key-hash-" + uuid.NewString(),
		Scopes:                   []string{"chat.completions"},
		Status:                   "active",
		RpmLimit:                 60,
		DailyCostLimitMicroUsd:   dailyLimit,
		MonthlyCostLimitMicroUsd: monthlyLimit,
		QuotaAction:              action,
		CreatedAt:                now,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey returned error: %v", err)
	}
	provider, err := st.Queries.CreateProvider(ctx, db.CreateProviderParams{
		ID:        uuid.New(),
		OrgID:     orgID,
		Name:      "quota-provider-" + uuid.NewString(),
		Type:      "mock",
		TimeoutMs: 30000,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateProvider returned error: %v", err)
	}
	model, err := st.Queries.CreateModel(ctx, db.CreateModelParams{
		ID:                             uuid.New(),
		OrgID:                          orgID,
		ProviderID:                     provider.ID,
		ProviderModelName:              "quota-model-" + uuid.NewString(),
		DisplayName:                    "Quota Model",
		InputPriceMicroUsdPer1kTokens:  100,
		OutputPriceMicroUsdPer1kTokens: 200,
		ContextWindow:                  pgtype.Int4{Int32: 8192, Valid: true},
		Status:                         "active",
		CreatedAt:                      now,
		UpdatedAt:                      now,
	})
	if err != nil {
		t.Fatalf("CreateModel returned error: %v", err)
	}
	return quotaFixture{OrgID: orgID, APIKeyID: apiKey.ID, ProviderID: provider.ID, ModelID: model.ID}
}

func insertQuotaCost(t *testing.T, ctx context.Context, st *store.Store, fixture quotaFixture, totalMicroUSD int64, createdAt time.Time) {
	t.Helper()

	requestLogID := uuid.New()
	usageRecordID := uuid.New()
	if _, err := st.Queries.InsertRequestLog(ctx, db.InsertRequestLogParams{
		ID:          requestLogID,
		OrgID:       fixture.OrgID,
		ApiKeyID:    &fixture.APIKeyID,
		ProviderID:  &fixture.ProviderID,
		ModelID:     &fixture.ModelID,
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
	if _, err := st.Queries.InsertUsageRecord(ctx, db.InsertUsageRecordParams{
		ID:               usageRecordID,
		RequestLogID:     requestLogID,
		OrgID:            fixture.OrgID,
		ApiKeyID:         fixture.APIKeyID,
		ProviderID:       fixture.ProviderID,
		ModelID:          fixture.ModelID,
		PromptTokens:     1,
		CompletionTokens: 1,
		TotalTokens:      2,
		CreatedAt:        createdAt,
	}); err != nil {
		t.Fatalf("InsertUsageRecord returned error: %v", err)
	}
	if _, err := st.Queries.InsertCostRecord(ctx, db.InsertCostRecordParams{
		ID:              uuid.New(),
		UsageRecordID:   usageRecordID,
		OrgID:           fixture.OrgID,
		ProviderID:      fixture.ProviderID,
		ModelID:         fixture.ModelID,
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

func int8Value(value int64) pgtype.Int8 {
	return pgtype.Int8{Int64: value, Valid: true}
}
