package limits

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tep/llm-cost-gateway/internal/store"
)

const (
	ScopeTypeOrg    = "org"
	ScopeTypeAPIKey = "api_key"

	PeriodDaily   = "daily"
	PeriodMonthly = "monthly"

	ActionBlock = "block"
	ActionWarn  = "warn"

	StatusActive = "active"
)

type ReservationService struct {
	store *store.Store
	clock func() time.Time
}

type Option func(*ReservationService)

func WithClock(clock func() time.Time) Option {
	return func(s *ReservationService) {
		if clock != nil {
			s.clock = clock
		}
	}
}

func NewReservationService(st *store.Store, opts ...Option) *ReservationService {
	service := &ReservationService{
		store: st,
		clock: func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

type ReserveInput struct {
	RequestID             uuid.UUID
	OrgID                 uuid.UUID
	APIKeyID              uuid.UUID
	EstimatedCostMicroUSD int64
}

type ReserveResult struct {
	Allowed          bool
	BlockedScope     string
	Period           string
	LimitMicroUSD    int64
	UsedMicroUSD     int64
	ReservedMicroUSD int64
}

type constraint struct {
	ScopeType     string
	ScopeID       uuid.UUID
	Period        string
	LimitMicroUSD int64
}

type counterState struct {
	ID               uuid.UUID
	SettledMicroUSD  int64
	ReservedMicroUSD int64
}

func (s *ReservationService) Reserve(ctx context.Context, input ReserveInput) (ReserveResult, error) {
	if input.RequestID == uuid.Nil {
		return ReserveResult{}, fmt.Errorf("request_id is required")
	}
	if input.OrgID == uuid.Nil {
		return ReserveResult{}, fmt.Errorf("org_id is required")
	}
	if input.APIKeyID == uuid.Nil {
		return ReserveResult{}, fmt.Errorf("api_key_id is required")
	}
	if input.EstimatedCostMicroUSD < 0 {
		return ReserveResult{}, fmt.Errorf("estimated_cost_micro_usd must be non-negative")
	}

	tx, err := s.store.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReserveResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	constraints, err := s.loadBlockingConstraints(ctx, tx, input.OrgID, input.APIKeyID)
	if err != nil {
		return ReserveResult{}, err
	}
	if len(constraints) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return ReserveResult{}, err
		}
		return ReserveResult{Allowed: true}, nil
	}

	now := s.clock().UTC()
	for _, item := range constraints {
		windowStart, windowEnd, err := periodWindow(now, item.Period)
		if err != nil {
			return ReserveResult{}, err
		}
		state, err := s.ensureAndLockCounter(ctx, tx, input.OrgID, input.APIKeyID, item, windowStart, windowEnd, now)
		if err != nil {
			return ReserveResult{}, err
		}
		used := state.SettledMicroUSD + state.ReservedMicroUSD
		remaining := item.LimitMicroUSD - used
		if remaining <= 0 {
			return ReserveResult{
				Allowed:          false,
				BlockedScope:     item.ScopeType,
				Period:           item.Period,
				LimitMicroUSD:    item.LimitMicroUSD,
				UsedMicroUSD:     used,
				ReservedMicroUSD: state.ReservedMicroUSD,
			}, nil
		}
		reservationAmount := input.EstimatedCostMicroUSD
		if reservationAmount > remaining {
			if used > 0 {
				return ReserveResult{
					Allowed:          false,
					BlockedScope:     item.ScopeType,
					Period:           item.Period,
					LimitMicroUSD:    item.LimitMicroUSD,
					UsedMicroUSD:     used,
					ReservedMicroUSD: state.ReservedMicroUSD,
				}, nil
			}
			reservationAmount = remaining
		}
		if reservationAmount == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cost_limit_counters
			SET reserved_micro_usd = reserved_micro_usd + $2,
			    updated_at = $3
			WHERE id = $1
		`, state.ID, reservationAmount, now); err != nil {
			return ReserveResult{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cost_reservations (
				id, request_id, org_id, api_key_id, scope_type, scope_id, period,
				window_start, window_end, reserved_micro_usd, settled_micro_usd,
				status, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7,
				$8, $9, $10, 0,
				'reserved', $11, $11
			)
		`, uuid.New(), input.RequestID, input.OrgID, input.APIKeyID, item.ScopeType, item.ScopeID, item.Period, windowStart, windowEnd, reservationAmount, now); err != nil {
			return ReserveResult{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return ReserveResult{}, err
	}
	return ReserveResult{Allowed: true}, nil
}

func (s *ReservationService) Settle(ctx context.Context, requestID uuid.UUID, actualCostMicroUSD int64) error {
	if actualCostMicroUSD < 0 {
		return fmt.Errorf("actual_cost_micro_usd must be non-negative")
	}
	tx, err := s.store.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := SettleTx(ctx, tx, requestID, actualCostMicroUSD, s.clock().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *ReservationService) Release(ctx context.Context, requestID uuid.UUID) error {
	tx, err := s.store.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := ReleaseTx(ctx, tx, requestID, s.clock().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func SettleTx(ctx context.Context, tx pgx.Tx, requestID uuid.UUID, actualCostMicroUSD int64, now time.Time) error {
	if requestID == uuid.Nil {
		return fmt.Errorf("request_id is required")
	}
	if actualCostMicroUSD < 0 {
		return fmt.Errorf("actual_cost_micro_usd must be non-negative")
	}
	rows, err := tx.Query(ctx, `
		SELECT org_id, scope_type, scope_id, period, window_start, reserved_micro_usd
		FROM cost_reservations
		WHERE request_id = $1 AND status = 'reserved'
		FOR UPDATE
	`, requestID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type reservation struct {
		OrgID            uuid.UUID
		ScopeType        string
		ScopeID          uuid.UUID
		Period           string
		WindowStart      time.Time
		ReservedMicroUSD int64
	}
	var reservations []reservation
	for rows.Next() {
		var item reservation
		if err := rows.Scan(&item.OrgID, &item.ScopeType, &item.ScopeID, &item.Period, &item.WindowStart, &item.ReservedMicroUSD); err != nil {
			return err
		}
		reservations = append(reservations, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range reservations {
		if _, err := tx.Exec(ctx, `
				UPDATE cost_limit_counters
				SET reserved_micro_usd = GREATEST(reserved_micro_usd - $6, 0),
				    settled_micro_usd = settled_micro_usd + $7,
				    updated_at = $8
				WHERE org_id = $1
				  AND scope_type = $2
				  AND scope_id = $3
				  AND period = $4
				  AND window_start = $5
			`, item.OrgID, item.ScopeType, item.ScopeID, item.Period, item.WindowStart, item.ReservedMicroUSD, actualCostMicroUSD, now); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE cost_reservations
		SET status = 'settled',
		    settled_micro_usd = $2,
		    updated_at = $3
		WHERE request_id = $1 AND status = 'reserved'
	`, requestID, actualCostMicroUSD, now)
	return err
}

func ReleaseTx(ctx context.Context, tx pgx.Tx, requestID uuid.UUID, now time.Time) error {
	if requestID == uuid.Nil {
		return fmt.Errorf("request_id is required")
	}
	rows, err := tx.Query(ctx, `
		SELECT org_id, scope_type, scope_id, period, window_start, reserved_micro_usd
		FROM cost_reservations
		WHERE request_id = $1 AND status = 'reserved'
		FOR UPDATE
	`, requestID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type reservation struct {
		OrgID            uuid.UUID
		ScopeType        string
		ScopeID          uuid.UUID
		Period           string
		WindowStart      time.Time
		ReservedMicroUSD int64
	}
	var reservations []reservation
	for rows.Next() {
		var item reservation
		if err := rows.Scan(&item.OrgID, &item.ScopeType, &item.ScopeID, &item.Period, &item.WindowStart, &item.ReservedMicroUSD); err != nil {
			return err
		}
		reservations = append(reservations, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range reservations {
		if _, err := tx.Exec(ctx, `
			UPDATE cost_limit_counters
			SET reserved_micro_usd = GREATEST(reserved_micro_usd - $6, 0),
			    updated_at = $7
			WHERE org_id = $1
			  AND scope_type = $2
			  AND scope_id = $3
			  AND period = $4
			  AND window_start = $5
		`, item.OrgID, item.ScopeType, item.ScopeID, item.Period, item.WindowStart, item.ReservedMicroUSD, now); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE cost_reservations
		SET status = 'released', updated_at = $2
		WHERE request_id = $1 AND status = 'reserved'
	`, requestID, now)
	return err
}

func (s *ReservationService) loadBlockingConstraints(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, apiKeyID uuid.UUID) ([]constraint, error) {
	items := make([]constraint, 0, 4)
	rows, err := tx.Query(ctx, `
		SELECT period, MIN(limit_micro_usd)::bigint AS limit_micro_usd
		FROM budgets
		WHERE org_id = $1
		  AND scope_type = 'org'
		  AND status = 'active'
		  AND action = 'block'
		GROUP BY period
		ORDER BY period
	`, orgID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var period string
		var limit int64
		if err := rows.Scan(&period, &limit); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, constraint{ScopeType: ScopeTypeOrg, ScopeID: orgID, Period: period, LimitMicroUSD: limit})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	var dailyLimit *int64
	var monthlyLimit *int64
	var quotaAction string
	if err := tx.QueryRow(ctx, `
		SELECT daily_cost_limit_micro_usd, monthly_cost_limit_micro_usd, quota_action
		FROM api_keys
		WHERE org_id = $1 AND id = $2
	`, orgID, apiKeyID).Scan(&dailyLimit, &monthlyLimit, &quotaAction); err != nil {
		return nil, err
	}
	if quotaAction == ActionBlock {
		if dailyLimit != nil {
			items = append(items, constraint{ScopeType: ScopeTypeAPIKey, ScopeID: apiKeyID, Period: PeriodDaily, LimitMicroUSD: *dailyLimit})
		}
		if monthlyLimit != nil {
			items = append(items, constraint{ScopeType: ScopeTypeAPIKey, ScopeID: apiKeyID, Period: PeriodMonthly, LimitMicroUSD: *monthlyLimit})
		}
	}
	return items, nil
}

func (s *ReservationService) ensureAndLockCounter(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, apiKeyID uuid.UUID, item constraint, windowStart time.Time, windowEnd time.Time, now time.Time) (counterState, error) {
	id := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO cost_limit_counters (
			id, org_id, scope_type, scope_id, period, window_start, window_end,
			settled_micro_usd, reserved_micro_usd, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			COALESCE((
				SELECT SUM(cr.total_cost_micro)::bigint
				FROM cost_records cr
				LEFT JOIN usage_records ur ON ur.org_id = cr.org_id AND ur.id = cr.usage_record_id
				WHERE cr.org_id = $2
				  AND cr.created_at >= $6
				  AND cr.created_at < $7
				  AND ($3 = 'org' OR ur.api_key_id = $8)
			), 0),
			0, $9, $9
		)
		ON CONFLICT (org_id, scope_type, scope_id, period, window_start) DO NOTHING
	`, id, orgID, item.ScopeType, item.ScopeID, item.Period, windowStart, windowEnd, apiKeyID, now); err != nil {
		return counterState{}, err
	}
	var state counterState
	if err := tx.QueryRow(ctx, `
		SELECT id, settled_micro_usd, reserved_micro_usd
		FROM cost_limit_counters
		WHERE org_id = $1
		  AND scope_type = $2
		  AND scope_id = $3
		  AND period = $4
		  AND window_start = $5
		FOR UPDATE
	`, orgID, item.ScopeType, item.ScopeID, item.Period, windowStart).Scan(&state.ID, &state.SettledMicroUSD, &state.ReservedMicroUSD); err != nil {
		return counterState{}, err
	}
	return state, nil
}

func periodWindow(now time.Time, period string) (time.Time, time.Time, error) {
	now = now.UTC()
	switch period {
	case PeriodDaily:
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 0, 1), nil
	case PeriodMonthly:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0), nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported cost reservation period %q", period)
	}
}
