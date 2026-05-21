package anomaly

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	RuleDailyCost       = "daily_cost"
	RuleAPIKeyCostSpike = "api_key_cost_spike"
	RuleModelCostSpike  = "model_cost_spike"
	RuleErrorRateSpike  = "error_rate_spike"

	ScopeOrg    = "org"
	ScopeAPIKey = "api_key"
	ScopeModel  = "model"

	ActionNotify    = "notify"
	ActionDowngrade = "downgrade"
	ActionBlock     = "block"

	StatusActive   = "active"
	StatusDisabled = "disabled"

	defaultCurrentWindowMinutes  = int32(60)
	defaultBaselineWindowMinutes = int32(1440)
	defaultMinRequests           = int32(1)
	defaultSpikeMultiplierBPS    = int32(20000)
	bpsScale                     = int64(10000)
)

type Service struct {
	store *store.Store
	clock func() time.Time
}

type Option func(*Service)

func WithClock(clock func() time.Time) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

func NewService(st *store.Store, opts ...Option) *Service {
	service := &Service{
		store: st,
		clock: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

type CreatePolicyParams struct {
	OrgID                 uuid.UUID
	Name                  string
	RuleType              string
	ScopeType             string
	ScopeID               *uuid.UUID
	ModelAlias            string
	ThresholdMicroUSD     *int64
	ThresholdBPS          *int32
	SpikeMultiplierBPS    int32
	CurrentWindowMinutes  int32
	BaselineWindowMinutes int32
	MinRequests           int32
	Action                string
	FallbackModel         string
}

type EvaluateParams struct {
	OrgID          uuid.UUID
	APIKeyID       uuid.UUID
	RequestedModel string
}

type Decision struct {
	Triggered      bool
	Action         string
	Policy         *db.CostAnomalyPolicy
	OriginalModel  string
	EffectiveModel string
	Reason         string
}

func (d Decision) MetadataBody() map[string]any {
	if !d.Triggered || d.Policy == nil {
		return nil
	}
	body := map[string]any{
		"policy_id":      d.Policy.ID,
		"policy_name":    d.Policy.Name,
		"rule_type":      d.Policy.RuleType,
		"action":         d.Action,
		"reason":         d.Reason,
		"original_model": d.OriginalModel,
	}
	if d.Action == ActionDowngrade {
		body["fallback_model"] = d.EffectiveModel
	}
	return body
}

func (s *Service) CreatePolicy(ctx context.Context, params CreatePolicyParams) (db.CostAnomalyPolicy, error) {
	normalized, err := normalizeCreatePolicyParams(params)
	if err != nil {
		return db.CostAnomalyPolicy{}, err
	}
	now := s.clock()
	return s.store.Queries.CreateCostAnomalyPolicy(ctx, db.CreateCostAnomalyPolicyParams{
		ID:                    uuid.New(),
		OrgID:                 normalized.OrgID,
		Name:                  normalized.Name,
		RuleType:              normalized.RuleType,
		ScopeType:             normalized.ScopeType,
		ScopeID:               normalized.ScopeID,
		ModelAlias:            textValue(normalized.ModelAlias),
		ThresholdMicroUsd:     int8Value(normalized.ThresholdMicroUSD),
		ThresholdBps:          int4Value(normalized.ThresholdBPS),
		SpikeMultiplierBps:    normalized.SpikeMultiplierBPS,
		CurrentWindowMinutes:  normalized.CurrentWindowMinutes,
		BaselineWindowMinutes: normalized.BaselineWindowMinutes,
		MinRequests:           normalized.MinRequests,
		Action:                normalized.Action,
		FallbackModel:         textValue(normalized.FallbackModel),
		Status:                StatusActive,
		CreatedAt:             now,
		UpdatedAt:             now,
	})
}

func (s *Service) ListPolicies(ctx context.Context, orgID uuid.UUID) ([]db.CostAnomalyPolicy, error) {
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("org_id is required")
	}
	return s.store.Queries.ListCostAnomalyPolicies(ctx, orgID)
}

func (s *Service) Evaluate(ctx context.Context, params EvaluateParams) (Decision, error) {
	if params.OrgID == uuid.Nil || strings.TrimSpace(params.RequestedModel) == "" {
		return Decision{}, nil
	}
	policies, err := s.store.Queries.ListActiveCostAnomalyPolicies(ctx, params.OrgID)
	if err != nil {
		return Decision{}, err
	}

	best := Decision{}
	for _, policy := range policies {
		if !policyApplies(policy, params) {
			continue
		}
		triggered, reason, err := s.evaluatePolicy(ctx, policy, params)
		if err != nil {
			return Decision{}, err
		}
		if !triggered {
			continue
		}
		decision := Decision{
			Triggered:      true,
			Action:         policy.Action,
			Policy:         &policy,
			OriginalModel:  strings.TrimSpace(params.RequestedModel),
			EffectiveModel: strings.TrimSpace(params.RequestedModel),
			Reason:         reason,
		}
		if policy.Action == ActionDowngrade && policy.FallbackModel.Valid {
			decision.EffectiveModel = policy.FallbackModel.String
		}
		if shouldReplaceDecision(best, decision) {
			best = decision
		}
	}
	return best, nil
}

func (s *Service) evaluatePolicy(ctx context.Context, policy db.CostAnomalyPolicy, params EvaluateParams) (bool, string, error) {
	switch policy.RuleType {
	case RuleDailyCost:
		return s.evaluateDailyCost(ctx, policy)
	case RuleAPIKeyCostSpike, RuleModelCostSpike:
		return s.evaluateCostSpike(ctx, policy)
	case RuleErrorRateSpike:
		return s.evaluateErrorRateSpike(ctx, policy)
	default:
		return false, "", nil
	}
}

func (s *Service) evaluateDailyCost(ctx context.Context, policy db.CostAnomalyPolicy) (bool, string, error) {
	if !policy.ThresholdMicroUsd.Valid {
		return false, "", nil
	}
	now := s.clock().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	total, _, err := s.costStats(ctx, policy, start, now)
	if err != nil {
		return false, "", err
	}
	if total >= policy.ThresholdMicroUsd.Int64 {
		return true, "daily_cost_threshold", nil
	}
	return false, "", nil
}

func (s *Service) evaluateCostSpike(ctx context.Context, policy db.CostAnomalyPolicy) (bool, string, error) {
	now := s.clock().UTC()
	currentStart := now.Add(-time.Duration(policy.CurrentWindowMinutes) * time.Minute)
	baselineEnd := currentStart
	baselineStart := baselineEnd.Add(-time.Duration(policy.BaselineWindowMinutes) * time.Minute)

	currentCost, currentRequests, err := s.costStats(ctx, policy, currentStart, now)
	if err != nil {
		return false, "", err
	}
	if currentRequests < int64(policy.MinRequests) || currentCost <= 0 {
		return false, "", nil
	}
	if policy.ThresholdMicroUsd.Valid && currentCost < policy.ThresholdMicroUsd.Int64 {
		return false, "", nil
	}
	baselineCost, _, err := s.costStats(ctx, policy, baselineStart, baselineEnd)
	if err != nil {
		return false, "", err
	}
	if baselineCost <= 0 {
		return false, "", nil
	}

	left := currentCost * int64(policy.BaselineWindowMinutes) * bpsScale
	right := baselineCost * int64(policy.CurrentWindowMinutes) * int64(policy.SpikeMultiplierBps)
	if left >= right {
		return true, "cost_spike", nil
	}
	return false, "", nil
}

func (s *Service) evaluateErrorRateSpike(ctx context.Context, policy db.CostAnomalyPolicy) (bool, string, error) {
	if !policy.ThresholdBps.Valid {
		return false, "", nil
	}
	now := s.clock().UTC()
	currentStart := now.Add(-time.Duration(policy.CurrentWindowMinutes) * time.Minute)
	baselineEnd := currentStart
	baselineStart := baselineEnd.Add(-time.Duration(policy.BaselineWindowMinutes) * time.Minute)

	current, err := s.errorStats(ctx, policy, currentStart, now)
	if err != nil {
		return false, "", err
	}
	if current.Requests < int64(policy.MinRequests) || current.Errors == 0 {
		return false, "", nil
	}
	if current.Errors*bpsScale < current.Requests*int64(policy.ThresholdBps.Int32) {
		return false, "", nil
	}

	baseline, err := s.errorStats(ctx, policy, baselineStart, baselineEnd)
	if err != nil {
		return false, "", err
	}
	if baseline.Requests == 0 || baseline.Errors == 0 {
		return true, "error_rate_spike", nil
	}
	left := current.Errors * baseline.Requests * bpsScale
	right := baseline.Errors * current.Requests * int64(policy.SpikeMultiplierBps)
	if left >= right {
		return true, "error_rate_spike", nil
	}
	return false, "", nil
}

func (s *Service) costStats(ctx context.Context, policy db.CostAnomalyPolicy, from time.Time, to time.Time) (int64, int64, error) {
	filter, args := policySQLFilter(policy, 4)
	query := `
		SELECT
		  COALESCE(sum(cr.total_cost_micro), 0)::bigint AS total_cost_micro_usd,
		  count(rl.id)::bigint AS request_count
		FROM request_logs rl
		LEFT JOIN usage_records ur
		  ON ur.org_id = rl.org_id
		 AND ur.request_log_id = rl.id
		LEFT JOIN cost_records cr
		  ON cr.org_id = rl.org_id
		 AND cr.usage_record_id = ur.id
		WHERE rl.org_id = $1
		  AND rl.completed_at >= $2
		  AND rl.completed_at < $3` + filter
	values := append([]any{policy.OrgID, from, to}, args...)
	var total int64
	var requests int64
	if err := s.store.Pool.QueryRow(ctx, query, values...).Scan(&total, &requests); err != nil {
		return 0, 0, err
	}
	return total, requests, nil
}

type errorStats struct {
	Requests int64
	Errors   int64
}

func (s *Service) errorStats(ctx context.Context, policy db.CostAnomalyPolicy, from time.Time, to time.Time) (errorStats, error) {
	filter, args := policySQLFilter(policy, 4)
	query := `
		SELECT
		  count(*)::bigint AS request_count,
		  count(*) FILTER (WHERE status NOT IN ('success', 'budget_warned'))::bigint AS error_count
		FROM request_logs rl
		WHERE rl.org_id = $1
		  AND rl.completed_at >= $2
		  AND rl.completed_at < $3` + filter
	values := append([]any{policy.OrgID, from, to}, args...)
	var stats errorStats
	if err := s.store.Pool.QueryRow(ctx, query, values...).Scan(&stats.Requests, &stats.Errors); err != nil {
		return errorStats{}, err
	}
	return stats, nil
}

func policySQLFilter(policy db.CostAnomalyPolicy, firstArg int) (string, []any) {
	switch policy.ScopeType {
	case ScopeAPIKey:
		return fmt.Sprintf(" AND rl.api_key_id = $%d", firstArg), []any{*policy.ScopeID}
	case ScopeModel:
		return fmt.Sprintf(" AND rl.request_model = $%d", firstArg), []any{policy.ModelAlias.String}
	default:
		return "", nil
	}
}

func normalizeCreatePolicyParams(params CreatePolicyParams) (CreatePolicyParams, error) {
	params.Name = strings.TrimSpace(params.Name)
	params.RuleType = strings.TrimSpace(params.RuleType)
	params.ScopeType = strings.TrimSpace(params.ScopeType)
	params.ModelAlias = strings.TrimSpace(params.ModelAlias)
	params.Action = strings.TrimSpace(params.Action)
	params.FallbackModel = strings.TrimSpace(params.FallbackModel)
	if params.OrgID == uuid.Nil {
		return CreatePolicyParams{}, fmt.Errorf("org_id is required")
	}
	if params.Name == "" {
		return CreatePolicyParams{}, fmt.Errorf("name is required")
	}
	if !validRule(params.RuleType) {
		return CreatePolicyParams{}, fmt.Errorf("invalid rule_type")
	}
	if !validAction(params.Action) {
		return CreatePolicyParams{}, fmt.Errorf("invalid action")
	}
	if params.Action == ActionDowngrade && params.FallbackModel == "" {
		return CreatePolicyParams{}, fmt.Errorf("fallback_model is required for downgrade")
	}
	if params.CurrentWindowMinutes == 0 {
		params.CurrentWindowMinutes = defaultCurrentWindowMinutes
	}
	if params.BaselineWindowMinutes == 0 {
		params.BaselineWindowMinutes = defaultBaselineWindowMinutes
	}
	if params.MinRequests == 0 {
		params.MinRequests = defaultMinRequests
	}
	if params.SpikeMultiplierBPS == 0 {
		params.SpikeMultiplierBPS = defaultSpikeMultiplierBPS
	}
	if params.CurrentWindowMinutes < 0 || params.BaselineWindowMinutes < 0 || params.MinRequests < 0 || params.SpikeMultiplierBPS < 0 {
		return CreatePolicyParams{}, fmt.Errorf("window, min_requests, and spike_multiplier_bps must be positive")
	}
	if err := validateScope(params); err != nil {
		return CreatePolicyParams{}, err
	}
	if err := validateRuleThresholds(params); err != nil {
		return CreatePolicyParams{}, err
	}
	return params, nil
}

func validateScope(params CreatePolicyParams) error {
	switch params.ScopeType {
	case ScopeOrg:
		if params.ScopeID != nil || params.ModelAlias != "" {
			return fmt.Errorf("org scope cannot include scope_id or model_alias")
		}
	case ScopeAPIKey:
		if params.ScopeID == nil || *params.ScopeID == uuid.Nil || params.ModelAlias != "" {
			return fmt.Errorf("api_key scope requires scope_id only")
		}
	case ScopeModel:
		if params.ScopeID != nil || params.ModelAlias == "" {
			return fmt.Errorf("model scope requires model_alias only")
		}
	default:
		return fmt.Errorf("invalid scope_type")
	}
	if params.RuleType == RuleAPIKeyCostSpike && params.ScopeType != ScopeAPIKey {
		return fmt.Errorf("api_key_cost_spike requires api_key scope")
	}
	if params.RuleType == RuleModelCostSpike && params.ScopeType != ScopeModel {
		return fmt.Errorf("model_cost_spike requires model scope")
	}
	if params.RuleType == RuleDailyCost && params.ScopeType != ScopeOrg {
		return fmt.Errorf("daily_cost requires org scope")
	}
	return nil
}

func validateRuleThresholds(params CreatePolicyParams) error {
	if params.ThresholdMicroUSD != nil && *params.ThresholdMicroUSD < 0 {
		return fmt.Errorf("threshold_micro_usd must be non-negative")
	}
	if params.ThresholdBPS != nil && *params.ThresholdBPS <= 0 {
		return fmt.Errorf("threshold_bps must be positive")
	}
	switch params.RuleType {
	case RuleDailyCost:
		if params.ThresholdMicroUSD == nil {
			return fmt.Errorf("threshold_micro_usd is required for daily_cost")
		}
	case RuleErrorRateSpike:
		if params.ThresholdBPS == nil {
			return fmt.Errorf("threshold_bps is required for error_rate_spike")
		}
	}
	return nil
}

func policyApplies(policy db.CostAnomalyPolicy, params EvaluateParams) bool {
	switch policy.ScopeType {
	case ScopeOrg:
		return true
	case ScopeAPIKey:
		return policy.ScopeID != nil && *policy.ScopeID == params.APIKeyID
	case ScopeModel:
		return policy.ModelAlias.Valid && policy.ModelAlias.String == strings.TrimSpace(params.RequestedModel)
	default:
		return false
	}
}

func shouldReplaceDecision(current Decision, next Decision) bool {
	if !current.Triggered {
		return true
	}
	return actionRank(next.Action) > actionRank(current.Action)
}

func actionRank(action string) int {
	switch action {
	case ActionBlock:
		return 3
	case ActionDowngrade:
		return 2
	case ActionNotify:
		return 1
	default:
		return 0
	}
}

func validRule(rule string) bool {
	switch rule {
	case RuleDailyCost, RuleAPIKeyCostSpike, RuleModelCostSpike, RuleErrorRateSpike:
		return true
	default:
		return false
	}
}

func validAction(action string) bool {
	switch action {
	case ActionNotify, ActionDowngrade, ActionBlock:
		return true
	default:
		return false
	}
}

func textValue(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func int8Value(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func int4Value(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}
