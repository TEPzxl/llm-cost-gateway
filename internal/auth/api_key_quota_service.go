package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	QuotaPeriodDaily   = "daily"
	QuotaPeriodMonthly = "monthly"

	QuotaActionWarn  = "warn"
	QuotaActionBlock = "block"
)

type APIKeyQuotaService struct {
	queries *db.Queries
	clock   func() time.Time
}

type APIKeyQuotaOption func(*APIKeyQuotaService)

func WithAPIKeyQuotaClock(clock func() time.Time) APIKeyQuotaOption {
	return func(s *APIKeyQuotaService) {
		if clock != nil {
			s.clock = clock
		}
	}
}

func NewAPIKeyQuotaService(queries *db.Queries, opts ...APIKeyQuotaOption) *APIKeyQuotaService {
	service := &APIKeyQuotaService{
		queries: queries,
		clock:   func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

type APIKeyQuotaStatus struct {
	APIKey            db.ApiKey
	Period            string
	LimitMicroUSD     int64
	UsedMicroUSD      int64
	RemainingMicroUSD int64
	Exceeded          bool
	WindowStart       time.Time
	WindowEnd         time.Time
}

type APIKeyQuotaCheckResult struct {
	Allowed  bool
	Exceeded bool
	Warning  bool
	Action   string
	Status   *APIKeyQuotaStatus
}

func (s *APIKeyQuotaService) Check(ctx context.Context, principal APIKeyPrincipal) (APIKeyQuotaCheckResult, error) {
	apiKey, err := s.queries.GetAPIKey(ctx, db.GetAPIKeyParams{
		OrgID: principal.OrgID,
		ID:    principal.APIKeyID,
	})
	if err != nil {
		return APIKeyQuotaCheckResult{}, err
	}

	result := APIKeyQuotaCheckResult{Allowed: true}
	for _, period := range []struct {
		name  string
		limit pgtype.Int8
	}{
		{name: QuotaPeriodDaily, limit: apiKey.DailyCostLimitMicroUsd},
		{name: QuotaPeriodMonthly, limit: apiKey.MonthlyCostLimitMicroUsd},
	} {
		limit, ok := quotaLimitValue(period.limit)
		if !ok {
			continue
		}
		status, err := s.statusForPeriod(ctx, apiKey, period.name, limit)
		if err != nil {
			return APIKeyQuotaCheckResult{}, err
		}
		if !status.Exceeded {
			continue
		}
		current := status
		switch quotaAction(apiKey.QuotaAction) {
		case QuotaActionBlock:
			return APIKeyQuotaCheckResult{
				Allowed:  false,
				Exceeded: true,
				Action:   QuotaActionBlock,
				Status:   &current,
			}, nil
		case QuotaActionWarn:
			if !result.Warning {
				result.Exceeded = true
				result.Warning = true
				result.Action = QuotaActionWarn
				result.Status = &current
			}
		}
	}

	return result, nil
}

func (s *APIKeyQuotaService) statusForPeriod(ctx context.Context, apiKey db.ApiKey, period string, limit int64) (APIKeyQuotaStatus, error) {
	start, end, err := s.periodWindow(period)
	if err != nil {
		return APIKeyQuotaStatus{}, err
	}
	used, err := s.queries.GetAPIKeyCostUsedAmount(ctx, db.GetAPIKeyCostUsedAmountParams{
		OrgID:       apiKey.OrgID,
		ApiKeyID:    apiKey.ID,
		CreatedAt:   start,
		CreatedAt_2: end,
	})
	if err != nil {
		return APIKeyQuotaStatus{}, err
	}

	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}
	return APIKeyQuotaStatus{
		APIKey:            apiKey,
		Period:            period,
		LimitMicroUSD:     limit,
		UsedMicroUSD:      used,
		RemainingMicroUSD: remaining,
		Exceeded:          used >= limit,
		WindowStart:       start,
		WindowEnd:         end,
	}, nil
}

func (s *APIKeyQuotaService) periodWindow(period string) (time.Time, time.Time, error) {
	now := s.clock().UTC()
	switch period {
	case QuotaPeriodDaily:
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 0, 1), nil
	case QuotaPeriodMonthly:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0), nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported api key quota period %q", period)
	}
}

func quotaLimitValue(value pgtype.Int8) (int64, bool) {
	if !value.Valid {
		return 0, false
	}
	return value.Int64, true
}

func quotaAction(action string) string {
	if action == QuotaActionWarn {
		return QuotaActionWarn
	}
	return QuotaActionBlock
}

func nullableInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func validateQuotaLimit(name string, value *int64) error {
	if value != nil && *value < 0 {
		return fmt.Errorf("%s must be non-negative", name)
	}
	return nil
}

func validateQuotaAction(action string) (string, error) {
	if action == "" {
		return QuotaActionBlock, nil
	}
	if action != QuotaActionWarn && action != QuotaActionBlock {
		return "", fmt.Errorf("quota_action must be warn or block")
	}
	return action, nil
}
