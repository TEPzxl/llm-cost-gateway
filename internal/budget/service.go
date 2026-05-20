package budget

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	ScopeTypeOrg = "org"

	PeriodDaily   = "daily"
	PeriodMonthly = "monthly"

	ActionWarn  = "warn"
	ActionBlock = "block"

	StatusActive   = "active"
	StatusDisabled = "disabled"
)

type Service struct {
	queries *db.Queries
	clock   func() time.Time
}

type Option func(*Service)

func WithClock(clock func() time.Time) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

func NewService(queries *db.Queries, opts ...Option) *Service {
	service := &Service{
		queries: queries,
		clock:   func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

type CreateBudgetParams struct {
	OrgID         uuid.UUID
	Name          string
	ScopeType     string
	Period        string
	LimitMicroUSD int64
	Action        string
}

type Status struct {
	Budget            db.Budget
	UsedMicroUSD      int64
	RemainingMicroUSD int64
	Exceeded          bool
	WindowStart       time.Time
	WindowEnd         time.Time
}

type CheckResult struct {
	Allowed  bool
	Exceeded bool
	Warning  bool
	Action   string
	Status   *Status
}

func (s *Service) CreateBudget(ctx context.Context, params CreateBudgetParams) (db.Budget, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return db.Budget{}, fmt.Errorf("budget name is required")
	}
	if params.OrgID == uuid.Nil {
		return db.Budget{}, fmt.Errorf("org_id is required")
	}

	scopeType := params.ScopeType
	if scopeType == "" {
		scopeType = ScopeTypeOrg
	}
	if scopeType != ScopeTypeOrg {
		return db.Budget{}, fmt.Errorf("scope_type must be org")
	}
	if params.Period != PeriodDaily && params.Period != PeriodMonthly {
		return db.Budget{}, fmt.Errorf("period must be daily or monthly")
	}
	action := params.Action
	if action == "" {
		action = ActionBlock
	}
	if action != ActionWarn && action != ActionBlock {
		return db.Budget{}, fmt.Errorf("action must be warn or block")
	}
	if params.LimitMicroUSD < 0 {
		return db.Budget{}, fmt.Errorf("limit_micro_usd must be non-negative")
	}

	now := s.clock()
	return s.queries.CreateBudget(ctx, db.CreateBudgetParams{
		ID:            uuid.New(),
		OrgID:         params.OrgID,
		Name:          name,
		ScopeType:     scopeType,
		Period:        params.Period,
		LimitMicroUsd: params.LimitMicroUSD,
		Action:        action,
		Status:        StatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
}

func (s *Service) ListBudgets(ctx context.Context, orgID uuid.UUID) ([]db.Budget, error) {
	return s.queries.ListBudgets(ctx, orgID)
}

func (s *Service) GetStatus(ctx context.Context, orgID uuid.UUID) ([]Status, error) {
	budgets, err := s.queries.ListBudgets(ctx, orgID)
	if err != nil {
		return nil, err
	}

	statuses := make([]Status, 0, len(budgets))
	for _, item := range budgets {
		status, err := s.statusForBudget(ctx, item)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (s *Service) Check(ctx context.Context, orgID uuid.UUID) (CheckResult, error) {
	statuses, err := s.GetStatus(ctx, orgID)
	if err != nil {
		return CheckResult{}, err
	}

	result := CheckResult{Allowed: true}
	for _, status := range statuses {
		if status.Budget.Status != StatusActive || !status.Exceeded {
			continue
		}
		current := status
		switch status.Budget.Action {
		case ActionBlock:
			return CheckResult{
				Allowed:  false,
				Exceeded: true,
				Warning:  false,
				Action:   ActionBlock,
				Status:   &current,
			}, nil
		case ActionWarn:
			if !result.Warning {
				result.Exceeded = true
				result.Warning = true
				result.Action = ActionWarn
				result.Status = &current
			}
		}
	}
	return result, nil
}

func (s *Service) statusForBudget(ctx context.Context, item db.Budget) (Status, error) {
	start, end, err := s.periodWindow(item.Period)
	if err != nil {
		return Status{}, err
	}
	used, err := s.queries.GetBudgetUsedAmount(ctx, db.GetBudgetUsedAmountParams{
		OrgID:       item.OrgID,
		CreatedAt:   start,
		CreatedAt_2: end,
	})
	if err != nil {
		return Status{}, err
	}

	remaining := item.LimitMicroUsd - used
	if remaining < 0 {
		remaining = 0
	}
	return Status{
		Budget:            item,
		UsedMicroUSD:      used,
		RemainingMicroUSD: remaining,
		Exceeded:          used >= item.LimitMicroUsd,
		WindowStart:       start,
		WindowEnd:         end,
	}, nil
}

func (s *Service) periodWindow(period string) (time.Time, time.Time, error) {
	now := s.clock().UTC()
	switch period {
	case PeriodDaily:
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 0, 1), nil
	case PeriodMonthly:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0), nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported budget period %q", period)
	}
}
