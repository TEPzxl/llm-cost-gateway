package anomaly

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/store"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestServiceEvaluatePolicies(t *testing.T) {
	tests := []struct {
		name      string
		seed      func(context.Context, *testing.T, *store.Store, anomalyFixture, time.Time)
		policy    func(anomalyFixture) CreatePolicyParams
		evaluate  func(anomalyFixture) EvaluateParams
		triggered bool
		action    string
		model     string
	}{
		{
			name: "daily cost threshold blocks",
			seed: func(ctx context.Context, t *testing.T, st *store.Store, fx anomalyFixture, now time.Time) {
				t.Helper()
				insertAnomalyUsage(t, ctx, st, fx, "fast-chat", now.Add(-time.Hour), 1200, "success")
			},
			policy: func(fx anomalyFixture) CreatePolicyParams {
				threshold := int64(1000)
				return CreatePolicyParams{
					OrgID:             fx.OrgID,
					Name:              "daily block",
					RuleType:          RuleDailyCost,
					ScopeType:         ScopeOrg,
					ThresholdMicroUSD: &threshold,
					Action:            ActionBlock,
				}
			},
			evaluate: func(fx anomalyFixture) EvaluateParams {
				return EvaluateParams{OrgID: fx.OrgID, APIKeyID: fx.APIKeyID, RequestedModel: "fast-chat"}
			},
			triggered: true,
			action:    ActionBlock,
			model:     "fast-chat",
		},
		{
			name: "api key cost spike downgrades",
			seed: func(ctx context.Context, t *testing.T, st *store.Store, fx anomalyFixture, now time.Time) {
				t.Helper()
				insertAnomalyUsage(t, ctx, st, fx, "fast-chat", now.Add(-90*time.Minute), 100, "success")
				insertAnomalyUsage(t, ctx, st, fx, "fast-chat", now.Add(-30*time.Minute), 500, "success")
			},
			policy: func(fx anomalyFixture) CreatePolicyParams {
				return CreatePolicyParams{
					OrgID:                 fx.OrgID,
					Name:                  "api key spike",
					RuleType:              RuleAPIKeyCostSpike,
					ScopeType:             ScopeAPIKey,
					ScopeID:               &fx.APIKeyID,
					CurrentWindowMinutes:  60,
					BaselineWindowMinutes: 60,
					SpikeMultiplierBPS:    20000,
					Action:                ActionDowngrade,
					FallbackModel:         "cheap-chat",
				}
			},
			evaluate: func(fx anomalyFixture) EvaluateParams {
				return EvaluateParams{OrgID: fx.OrgID, APIKeyID: fx.APIKeyID, RequestedModel: "fast-chat"}
			},
			triggered: true,
			action:    ActionDowngrade,
			model:     "cheap-chat",
		},
		{
			name: "model cost spike notifies",
			seed: func(ctx context.Context, t *testing.T, st *store.Store, fx anomalyFixture, now time.Time) {
				t.Helper()
				insertAnomalyUsage(t, ctx, st, fx, "expensive-chat", now.Add(-90*time.Minute), 100, "success")
				insertAnomalyUsage(t, ctx, st, fx, "expensive-chat", now.Add(-15*time.Minute), 350, "success")
				insertAnomalyUsage(t, ctx, st, fx, "other-chat", now.Add(-15*time.Minute), 1000, "success")
			},
			policy: func(fx anomalyFixture) CreatePolicyParams {
				return CreatePolicyParams{
					OrgID:                 fx.OrgID,
					Name:                  "model spike",
					RuleType:              RuleModelCostSpike,
					ScopeType:             ScopeModel,
					ModelAlias:            "expensive-chat",
					CurrentWindowMinutes:  60,
					BaselineWindowMinutes: 60,
					SpikeMultiplierBPS:    20000,
					Action:                ActionNotify,
				}
			},
			evaluate: func(fx anomalyFixture) EvaluateParams {
				return EvaluateParams{OrgID: fx.OrgID, APIKeyID: fx.APIKeyID, RequestedModel: "expensive-chat"}
			},
			triggered: true,
			action:    ActionNotify,
			model:     "expensive-chat",
		},
		{
			name: "error rate spike blocks",
			seed: func(ctx context.Context, t *testing.T, st *store.Store, fx anomalyFixture, now time.Time) {
				t.Helper()
				for i := 0; i < 9; i++ {
					insertAnomalyRequest(t, ctx, st, fx, "fast-chat", now.Add(-90*time.Minute), "success")
				}
				insertAnomalyRequest(t, ctx, st, fx, "fast-chat", now.Add(-90*time.Minute), "error")
				for i := 0; i < 5; i++ {
					insertAnomalyRequest(t, ctx, st, fx, "fast-chat", now.Add(-10*time.Minute), "success")
					insertAnomalyRequest(t, ctx, st, fx, "fast-chat", now.Add(-10*time.Minute), "error")
				}
			},
			policy: func(fx anomalyFixture) CreatePolicyParams {
				threshold := int32(3000)
				return CreatePolicyParams{
					OrgID:                 fx.OrgID,
					Name:                  "error spike",
					RuleType:              RuleErrorRateSpike,
					ScopeType:             ScopeOrg,
					ThresholdBPS:          &threshold,
					CurrentWindowMinutes:  60,
					BaselineWindowMinutes: 60,
					SpikeMultiplierBPS:    20000,
					MinRequests:           10,
					Action:                ActionBlock,
				}
			},
			evaluate: func(fx anomalyFixture) EvaluateParams {
				return EvaluateParams{OrgID: fx.OrgID, APIKeyID: fx.APIKeyID, RequestedModel: "fast-chat"}
			},
			triggered: true,
			action:    ActionBlock,
			model:     "fast-chat",
		},
		{
			name: "normal traffic does not trigger spike",
			seed: func(ctx context.Context, t *testing.T, st *store.Store, fx anomalyFixture, now time.Time) {
				t.Helper()
				insertAnomalyUsage(t, ctx, st, fx, "fast-chat", now.Add(-90*time.Minute), 100, "success")
				insertAnomalyUsage(t, ctx, st, fx, "fast-chat", now.Add(-10*time.Minute), 120, "success")
			},
			policy: func(fx anomalyFixture) CreatePolicyParams {
				return CreatePolicyParams{
					OrgID:                 fx.OrgID,
					Name:                  "normal traffic",
					RuleType:              RuleAPIKeyCostSpike,
					ScopeType:             ScopeAPIKey,
					ScopeID:               &fx.APIKeyID,
					CurrentWindowMinutes:  60,
					BaselineWindowMinutes: 60,
					SpikeMultiplierBPS:    20000,
					Action:                ActionBlock,
				}
			},
			evaluate: func(fx anomalyFixture) EvaluateParams {
				return EvaluateParams{OrgID: fx.OrgID, APIKeyID: fx.APIKeyID, RequestedModel: "fast-chat"}
			},
			triggered: false,
			model:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := openAnomalyTestStore(t, ctx)
			fx := createAnomalyFixture(t, ctx, st)
			now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
			service := NewService(st, WithClock(func() time.Time { return now }))

			tt.seed(ctx, t, st, fx, now)
			if _, err := service.CreatePolicy(ctx, tt.policy(fx)); err != nil {
				t.Fatalf("CreatePolicy returned error: %v", err)
			}

			decision, err := service.Evaluate(ctx, tt.evaluate(fx))
			if err != nil {
				t.Fatalf("Evaluate returned error: %v", err)
			}
			if decision.Triggered != tt.triggered {
				t.Fatalf("triggered = %v, want %v; decision=%+v", decision.Triggered, tt.triggered, decision)
			}
			if tt.triggered {
				if decision.Action != tt.action {
					t.Fatalf("action = %q, want %q", decision.Action, tt.action)
				}
				if decision.EffectiveModel != tt.model {
					t.Fatalf("effective model = %q, want %q", decision.EffectiveModel, tt.model)
				}
				if decision.MetadataBody() == nil {
					t.Fatal("metadata body is nil for triggered decision")
				}
			}
		})
	}
}

func TestServiceCreatePolicyValidatesInputs(t *testing.T) {
	ctx := context.Background()
	st := openAnomalyTestStore(t, ctx)
	fx := createAnomalyFixture(t, ctx, st)
	service := NewService(st)

	_, err := service.CreatePolicy(ctx, CreatePolicyParams{
		OrgID:     fx.OrgID,
		Name:      "bad downgrade",
		RuleType:  RuleAPIKeyCostSpike,
		ScopeType: ScopeAPIKey,
		ScopeID:   &fx.APIKeyID,
		Action:    ActionDowngrade,
	})
	if err == nil {
		t.Fatal("CreatePolicy returned nil error for downgrade without fallback_model")
	}
}

type anomalyFixture struct {
	OrgID      uuid.UUID
	APIKeyID   uuid.UUID
	ProviderID uuid.UUID
	ModelID    uuid.UUID
}

func openAnomalyTestStore(t *testing.T, ctx context.Context) *store.Store {
	t.Helper()

	pool := testutil.OpenPostgres(t, ctx)
	return store.New(pool)
}

func createAnomalyFixture(t *testing.T, ctx context.Context, st *store.Store) anomalyFixture {
	t.Helper()

	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	orgID := uuid.New()
	apiKeyID := uuid.New()
	providerID := uuid.New()
	modelID := uuid.New()
	if _, err := st.Pool.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, status, created_at, updated_at)
		VALUES ($1, 'Anomaly Org', $2, 'active', $3, $3)
	`, orgID, "anomaly-"+uuid.NewString(), now); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `
		INSERT INTO api_keys (id, org_id, name, key_prefix, key_hash, scopes, status, rpm_limit, created_at)
		VALUES ($1, $2, 'test-key', 'llmgw_live_test', $3, ARRAY['chat.completions'], 'active', 60, $4)
	`, apiKeyID, orgID, "hash-"+uuid.NewString(), now); err != nil {
		t.Fatalf("insert api key: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `
		INSERT INTO providers (id, org_id, name, type, timeout_ms, status, created_at, updated_at)
		VALUES ($1, $2, 'mock-provider', 'mock', 30000, 'active', $3, $3)
	`, providerID, orgID, now); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `
		INSERT INTO models (
		  id,
		  org_id,
		  provider_id,
		  provider_model_name,
		  display_name,
		  input_price_micro_usd_per_1k_tokens,
		  output_price_micro_usd_per_1k_tokens,
		  status,
		  created_at,
		  updated_at
		) VALUES ($1, $2, $3, 'mock-model', 'Mock Model', 100, 100, 'active', $4, $4)
	`, modelID, orgID, providerID, now); err != nil {
		t.Fatalf("insert model: %v", err)
	}
	return anomalyFixture{OrgID: orgID, APIKeyID: apiKeyID, ProviderID: providerID, ModelID: modelID}
}

func insertAnomalyUsage(t *testing.T, ctx context.Context, st *store.Store, fx anomalyFixture, requestModel string, completedAt time.Time, costMicro int64, status string) {
	t.Helper()

	requestLogID := insertAnomalyRequest(t, ctx, st, fx, requestModel, completedAt, status)
	usageID := uuid.New()
	if _, err := st.Pool.Exec(ctx, `
		INSERT INTO usage_records (
		  id,
		  request_log_id,
		  org_id,
		  api_key_id,
		  provider_id,
		  model_id,
		  prompt_tokens,
		  completion_tokens,
		  total_tokens,
		  created_at
		) VALUES ($1, $2, $3, $4, $5, $6, 10, 10, 20, $7)
	`, usageID, requestLogID, fx.OrgID, fx.APIKeyID, fx.ProviderID, fx.ModelID, completedAt); err != nil {
		t.Fatalf("insert usage record: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `
		INSERT INTO cost_records (
		  id,
		  usage_record_id,
		  org_id,
		  provider_id,
		  model_id,
		  currency,
		  input_cost_micro,
		  output_cost_micro,
		  total_cost_micro,
		  pricing_snapshot,
		  created_at
		) VALUES ($1, $2, $3, $4, $5, 'USD', 0, $6, $6, '{}', $7)
	`, uuid.New(), usageID, fx.OrgID, fx.ProviderID, fx.ModelID, costMicro, completedAt); err != nil {
		t.Fatalf("insert cost record: %v", err)
	}
}

func insertAnomalyRequest(t *testing.T, ctx context.Context, st *store.Store, fx anomalyFixture, requestModel string, completedAt time.Time, status string) uuid.UUID {
	t.Helper()

	requestLogID := uuid.New()
	statusCode := 200
	errorCode := (*string)(nil)
	if status != "success" && status != "budget_warned" {
		statusCode = 502
		code := "provider_error"
		errorCode = &code
	}
	if _, err := st.Pool.Exec(ctx, `
		INSERT INTO request_logs (
		  id,
		  org_id,
		  api_key_id,
		  provider_id,
		  model_id,
		  method,
		  path,
		  request_model,
		  status,
		  status_code,
		  error_code,
		  latency_ms,
		  started_at,
		  completed_at
		) VALUES ($1, $2, $3, $4, $5, 'POST', '/v1/chat/completions', $6, $7, $8, $9, 10, $10, $10)
	`, requestLogID, fx.OrgID, fx.APIKeyID, fx.ProviderID, fx.ModelID, requestModel, status, statusCode, errorCode, completedAt); err != nil {
		t.Fatalf("insert request log: %v", err)
	}
	return requestLogID
}
