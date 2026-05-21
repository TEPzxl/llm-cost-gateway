package routing

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tep/llm-cost-gateway/internal/costing"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	StrategySingle     = "single"
	StrategyFallback   = "fallback"
	StrategyLowestCost = "lowest_cost"
)

var (
	ErrRouteNotFound       = errors.New("route not found")
	ErrRouteTargetNotFound = errors.New("route target not found")
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type TargetParams struct {
	ProviderID uuid.UUID
	ModelID    uuid.UUID
	Priority   int32
	Weight     int32
}

type CreateRoutePolicyParams struct {
	OrgID      uuid.UUID
	Name       string
	MatchModel string
	Strategy   string
	Config     json.RawMessage
	Targets    []TargetParams
}

type ResolveParams struct {
	OrgID                 uuid.UUID
	RequestedModel        string
	EstimatedPromptTokens int64
	EstimatedMaxTokens    int64
}

type ResolveResult struct {
	RoutePolicy db.RoutePolicy
	Provider    db.Provider
	Model       db.Model
}

type ResolvedTarget struct {
	RouteTarget           db.RouteTarget
	Provider              db.Provider
	Model                 db.Model
	EstimatedCostMicroUSD int64
}

type ResolveTargetsResult struct {
	RoutePolicy db.RoutePolicy
	Targets     []ResolvedTarget
}

type Service struct {
	store *store.Store
}

type Resolver struct {
	queries    *db.Queries
	calculator *costing.Calculator
}

func NewService(st *store.Store) *Service {
	return &Service{store: st}
}

func NewResolver(queries *db.Queries) *Resolver {
	return &Resolver{queries: queries, calculator: costing.NewCalculator()}
}

func (s *Service) CreateRoutePolicy(ctx context.Context, params CreateRoutePolicyParams) (db.RoutePolicy, error) {
	if err := validateCreateRoutePolicyParams(params); err != nil {
		return db.RoutePolicy{}, err
	}

	var created db.RoutePolicy
	now := time.Now().UTC()
	err := s.store.ExecTx(ctx, func(q *db.Queries) error {
		for _, target := range params.Targets {
			if err := validateTargetBelongsToOrg(ctx, q, params.OrgID, target); err != nil {
				return err
			}
		}

		config, err := normalizePolicyConfig(params.Config)
		if err != nil {
			return err
		}
		policy, err := q.CreateRoutePolicy(ctx, db.CreateRoutePolicyParams{
			ID:         uuid.New(),
			OrgID:      params.OrgID,
			Name:       strings.TrimSpace(params.Name),
			MatchModel: strings.TrimSpace(params.MatchModel),
			Strategy:   params.Strategy,
			Config:     config,
			Status:     "active",
			CreatedAt:  now,
			UpdatedAt:  now,
		})
		if err != nil {
			return err
		}

		for _, target := range params.Targets {
			if _, err := q.CreateRouteTarget(ctx, db.CreateRouteTargetParams{
				ID:            uuid.New(),
				OrgID:         params.OrgID,
				RoutePolicyID: policy.ID,
				ProviderID:    target.ProviderID,
				ModelID:       target.ModelID,
				Priority:      target.Priority,
				Weight:        target.Weight,
				CreatedAt:     now,
			}); err != nil {
				return err
			}
		}

		created = policy
		return nil
	})
	if err != nil {
		return db.RoutePolicy{}, err
	}

	return created, nil
}

func (s *Service) ListRoutePolicies(ctx context.Context, orgID uuid.UUID) ([]db.RoutePolicy, error) {
	return s.store.Queries.ListRoutePolicies(ctx, orgID)
}

func (r *Resolver) Resolve(ctx context.Context, params ResolveParams) (ResolveResult, error) {
	result, err := r.ResolveTargets(ctx, params)
	if err != nil {
		return ResolveResult{}, err
	}
	if len(result.Targets) == 0 {
		return ResolveResult{}, ErrRouteNotFound
	}
	target := result.Targets[0]
	return ResolveResult{
		RoutePolicy: result.RoutePolicy,
		Provider:    target.Provider,
		Model:       target.Model,
	}, nil
}

func (r *Resolver) ResolveTargets(ctx context.Context, params ResolveParams) (ResolveTargetsResult, error) {
	requestedModel := strings.TrimSpace(params.RequestedModel)
	if params.OrgID == uuid.Nil || requestedModel == "" {
		return ResolveTargetsResult{}, ErrRouteNotFound
	}

	policy, err := r.queries.GetRoutePolicyByMatchModel(ctx, db.GetRoutePolicyByMatchModelParams{
		OrgID:      params.OrgID,
		MatchModel: requestedModel,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolveTargetsResult{}, ErrRouteNotFound
		}
		return ResolveTargetsResult{}, err
	}
	if !validStrategy(policy.Strategy) {
		return ResolveTargetsResult{}, ErrRouteNotFound
	}

	targets, err := r.queries.ListRouteTargets(ctx, db.ListRouteTargetsParams{
		OrgID:         params.OrgID,
		RoutePolicyID: policy.ID,
	})
	if err != nil {
		return ResolveTargetsResult{}, err
	}
	if len(targets) == 0 {
		return ResolveTargetsResult{}, ErrRouteNotFound
	}

	resolvedTargets := make([]ResolvedTarget, 0, len(targets))
	for _, target := range targets {
		provider, err := r.queries.GetProvider(ctx, db.GetProviderParams{
			OrgID: params.OrgID,
			ID:    target.ProviderID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ResolveTargetsResult{}, ErrRouteNotFound
			}
			return ResolveTargetsResult{}, err
		}
		model, err := r.queries.GetModel(ctx, db.GetModelParams{
			OrgID: params.OrgID,
			ID:    target.ModelID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ResolveTargetsResult{}, ErrRouteNotFound
			}
			return ResolveTargetsResult{}, err
		}
		if provider.Status != "active" || model.Status != "active" || model.ProviderID != provider.ID {
			if policy.Strategy == StrategyLowestCost {
				continue
			}
			return ResolveTargetsResult{}, ErrRouteNotFound
		}
		resolvedTargets = append(resolvedTargets, ResolvedTarget{
			RouteTarget: target,
			Provider:    provider,
			Model:       model,
		})
	}
	if policy.Strategy == StrategyLowestCost {
		resolved, err := r.resolveLowestCostTargets(policy, params, resolvedTargets)
		if err != nil {
			return ResolveTargetsResult{}, err
		}
		resolvedTargets = resolved
	}
	if len(resolvedTargets) == 0 {
		return ResolveTargetsResult{}, ErrRouteNotFound
	}
	return ResolveTargetsResult{RoutePolicy: policy, Targets: resolvedTargets}, nil
}

type PolicyConfig struct {
	MaxEstimatedCostMicroUSD *int64 `json:"max_estimated_cost_micro_usd,omitempty"`
	FallbackToPriority       *bool  `json:"fallback_to_priority,omitempty"`
}

func (r *Resolver) resolveLowestCostTargets(policy db.RoutePolicy, params ResolveParams, targets []ResolvedTarget) ([]ResolvedTarget, error) {
	cfg, err := parsePolicyConfig(policy.Config)
	if err != nil {
		return nil, err
	}
	for index := range targets {
		cost, err := r.estimatedCostMicroUSD(targets[index].Model, params)
		if err != nil {
			return nil, err
		}
		targets[index].EstimatedCostMicroUSD = cost
	}
	sortLowestCostTargets(targets)
	if cfg.MaxEstimatedCostMicroUSD == nil {
		return targets, nil
	}
	eligible := make([]ResolvedTarget, 0, len(targets))
	for _, target := range targets {
		if target.EstimatedCostMicroUSD <= *cfg.MaxEstimatedCostMicroUSD {
			eligible = append(eligible, target)
		}
	}
	if len(eligible) > 0 {
		return eligible, nil
	}
	if cfg.fallbackToPriority() {
		sort.SliceStable(targets, func(i, j int) bool {
			if targets[i].RouteTarget.Priority != targets[j].RouteTarget.Priority {
				return targets[i].RouteTarget.Priority < targets[j].RouteTarget.Priority
			}
			return targets[i].RouteTarget.ID.String() < targets[j].RouteTarget.ID.String()
		})
		return targets, nil
	}
	return nil, ErrRouteNotFound
}

func (r *Resolver) estimatedCostMicroUSD(model db.Model, params ResolveParams) (int64, error) {
	promptTokens := params.EstimatedPromptTokens
	if promptTokens < 0 {
		promptTokens = 0
	}
	maxTokens := params.EstimatedMaxTokens
	if maxTokens < 0 {
		maxTokens = 0
	}
	calculation, err := r.calculator.Calculate(costing.CalculateInput{
		PromptTokens:                   promptTokens,
		CompletionTokens:               maxTokens,
		InputPriceMicroUSDPer1KTokens:  model.InputPriceMicroUsdPer1kTokens,
		OutputPriceMicroUSDPer1KTokens: model.OutputPriceMicroUsdPer1kTokens,
	})
	if err != nil {
		return 0, err
	}
	return calculation.TotalCostMicro, nil
}

func sortLowestCostTargets(targets []ResolvedTarget) {
	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].EstimatedCostMicroUSD != targets[j].EstimatedCostMicroUSD {
			return targets[i].EstimatedCostMicroUSD < targets[j].EstimatedCostMicroUSD
		}
		if targets[i].RouteTarget.Priority != targets[j].RouteTarget.Priority {
			return targets[i].RouteTarget.Priority < targets[j].RouteTarget.Priority
		}
		return targets[i].RouteTarget.ID.String() < targets[j].RouteTarget.ID.String()
	})
}

func parsePolicyConfig(raw json.RawMessage) (PolicyConfig, error) {
	if len(raw) == 0 {
		return PolicyConfig{}, nil
	}
	var cfg PolicyConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return PolicyConfig{}, err
	}
	if cfg.MaxEstimatedCostMicroUSD != nil && *cfg.MaxEstimatedCostMicroUSD < 0 {
		return PolicyConfig{}, validationError("max_estimated_cost_micro_usd must be non-negative")
	}
	return cfg, nil
}

func (c PolicyConfig) fallbackToPriority() bool {
	if c.FallbackToPriority == nil {
		return true
	}
	return *c.FallbackToPriority
}

func validateCreateRoutePolicyParams(params CreateRoutePolicyParams) error {
	if params.OrgID == uuid.Nil {
		return validationError("org_id is required")
	}
	if strings.TrimSpace(params.Name) == "" {
		return validationError("name is required")
	}
	if strings.TrimSpace(params.MatchModel) == "" {
		return validationError("match_model is required")
	}
	if !validStrategy(params.Strategy) {
		return validationError("strategy must be single, fallback or lowest_cost")
	}
	if params.Strategy == StrategySingle && len(params.Targets) != 1 {
		return validationError("single strategy requires exactly one target")
	}
	if params.Strategy == StrategyFallback && len(params.Targets) < 2 {
		return validationError("fallback strategy requires at least two targets")
	}
	if params.Strategy == StrategyLowestCost && len(params.Targets) == 0 {
		return validationError("lowest_cost strategy requires at least one target")
	}
	for _, target := range params.Targets {
		if target.ProviderID == uuid.Nil {
			return validationError("target provider_id is required")
		}
		if target.ModelID == uuid.Nil {
			return validationError("target model_id is required")
		}
		if target.Priority <= 0 {
			return validationError("target priority must be greater than zero")
		}
		if target.Weight <= 0 {
			return validationError("target weight must be greater than zero")
		}
	}
	return nil
}

func normalizePolicyConfig(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if !json.Valid(raw) {
		return nil, validationError("config must be valid JSON")
	}
	cfg, err := parsePolicyConfig(raw)
	if err != nil {
		return nil, err
	}
	if cfg.MaxEstimatedCostMicroUSD == nil && cfg.FallbackToPriority == nil {
		return append(json.RawMessage(nil), raw...), nil
	}
	return append(json.RawMessage(nil), raw...), nil
}

func validStrategy(strategy string) bool {
	switch strategy {
	case StrategySingle, StrategyFallback, StrategyLowestCost:
		return true
	default:
		return false
	}
}

func validateTargetBelongsToOrg(ctx context.Context, q *db.Queries, orgID uuid.UUID, target TargetParams) error {
	provider, err := q.GetProvider(ctx, db.GetProviderParams{
		OrgID: orgID,
		ID:    target.ProviderID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRouteTargetNotFound
		}
		return err
	}
	model, err := q.GetModel(ctx, db.GetModelParams{
		OrgID: orgID,
		ID:    target.ModelID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRouteTargetNotFound
		}
		return err
	}
	if model.ProviderID != provider.ID {
		return ErrRouteTargetNotFound
	}
	if provider.Status != "active" {
		return validationError("target provider must be active")
	}
	if model.Status != "active" {
		return validationError("target model must be active")
	}
	return nil
}

func IsValidationError(err error) (*ValidationError, bool) {
	var validation *ValidationError
	if errors.As(err, &validation) {
		return validation, true
	}
	return nil, false
}

func IsUniqueViolation(err error) bool {
	type sqlState interface {
		SQLState() string
	}
	var state sqlState
	if errors.As(err, &state) {
		return state.SQLState() == "23505"
	}
	return false
}

func validationError(message string) *ValidationError {
	return &ValidationError{Message: message}
}
