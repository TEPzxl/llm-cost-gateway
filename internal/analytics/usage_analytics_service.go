package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/store"
)

const (
	statusSuccess      = "success"
	statusBudgetWarned = "budget_warned"
)

type Sink interface {
	WriteUsageRecord(ctx context.Context, record UsageRecord) error
}

type DisabledSink struct{}

func (DisabledSink) WriteUsageRecord(context.Context, UsageRecord) error {
	return nil
}

type clickHouseBackend interface {
	InsertUsageRecord(ctx context.Context, record UsageRecord) error
	QueryJSONEachRow(ctx context.Context, query string, scan func(json.RawMessage) error) error
}

type UsageAnalyticsService struct {
	store             *store.Store
	clickhouse        clickHouseBackend
	clickHouseEnabled bool
}

type TimeWindow struct {
	From time.Time
	To   time.Time
}

type DailyCostPoint struct {
	Day               string `json:"day"`
	RequestCount      int64  `json:"request_count"`
	TotalCostMicroUSD int64  `json:"total_cost_micro_usd"`
}

type ModelCostBreakdownItem struct {
	ModelID           uuid.UUID `json:"model_id"`
	RequestCount      int64     `json:"request_count"`
	PromptTokens      int64     `json:"prompt_tokens"`
	CompletionTokens  int64     `json:"completion_tokens"`
	TotalTokens       int64     `json:"total_tokens"`
	TotalCostMicroUSD int64     `json:"total_cost_micro_usd"`
}

type ProviderLatencyItem struct {
	ProviderID       uuid.UUID `json:"provider_id"`
	RequestCount     int64     `json:"request_count"`
	AverageLatencyMS float64   `json:"avg_latency_ms"`
	P50LatencyMS     float64   `json:"p50_latency_ms"`
	P95LatencyMS     float64   `json:"p95_latency_ms"`
	P99LatencyMS     float64   `json:"p99_latency_ms"`
}

type ErrorRatePoint struct {
	Day          string  `json:"day"`
	RequestCount int64   `json:"request_count"`
	ErrorCount   int64   `json:"error_count"`
	ErrorRate    float64 `json:"error_rate"`
}

func NewUsageAnalyticsService(st *store.Store, clickhouse clickHouseBackend, clickHouseEnabled bool) *UsageAnalyticsService {
	return &UsageAnalyticsService{
		store:             st,
		clickhouse:        clickhouse,
		clickHouseEnabled: clickHouseEnabled && clickhouse != nil,
	}
}

func (s *UsageAnalyticsService) WriteUsageRecord(ctx context.Context, record UsageRecord) error {
	if s == nil || !s.clickHouseEnabled || s.clickhouse == nil {
		return nil
	}
	return s.clickhouse.InsertUsageRecord(ctx, record)
}

func (s *UsageAnalyticsService) DailyCostTrend(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]DailyCostPoint, error) {
	if s.clickHouseEnabled {
		return s.dailyCostTrendClickHouse(ctx, orgID, window)
	}
	return s.dailyCostTrendPostgres(ctx, orgID, window)
}

func (s *UsageAnalyticsService) ModelCostBreakdown(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]ModelCostBreakdownItem, error) {
	if s.clickHouseEnabled {
		return s.modelCostBreakdownClickHouse(ctx, orgID, window)
	}
	return s.modelCostBreakdownPostgres(ctx, orgID, window)
}

func (s *UsageAnalyticsService) ProviderLatency(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]ProviderLatencyItem, error) {
	if s.clickHouseEnabled {
		return s.providerLatencyClickHouse(ctx, orgID, window)
	}
	return s.providerLatencyPostgres(ctx, orgID, window)
}

func (s *UsageAnalyticsService) ErrorRateTrend(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]ErrorRatePoint, error) {
	if s.clickHouseEnabled {
		return s.errorRateTrendClickHouse(ctx, orgID, window)
	}
	return s.errorRateTrendPostgres(ctx, orgID, window)
}

func (s *UsageAnalyticsService) dailyCostTrendPostgres(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]DailyCostPoint, error) {
	rows, err := s.store.Pool.Query(ctx, `
		SELECT
		  to_char(date_trunc('day', rl.completed_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
		  count(*)::bigint AS request_count,
		  COALESCE(sum(cr.total_cost_micro), 0)::bigint AS total_cost_micro_usd
		FROM request_logs rl
		LEFT JOIN usage_records ur
		  ON ur.org_id = rl.org_id
		 AND ur.request_log_id = rl.id
		LEFT JOIN cost_records cr
		  ON cr.org_id = rl.org_id
		 AND cr.usage_record_id = ur.id
		WHERE rl.org_id = $1
		  AND rl.completed_at >= $2
		  AND rl.completed_at < $3
		GROUP BY day
		ORDER BY day ASC
	`, orgID, window.From, window.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []DailyCostPoint{}
	for rows.Next() {
		var item DailyCostPoint
		if err := rows.Scan(&item.Day, &item.RequestCount, &item.TotalCostMicroUSD); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *UsageAnalyticsService) modelCostBreakdownPostgres(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]ModelCostBreakdownItem, error) {
	rows, err := s.store.Pool.Query(ctx, `
		SELECT
		  ur.model_id,
		  count(*)::bigint AS request_count,
		  COALESCE(sum(ur.prompt_tokens), 0)::bigint AS prompt_tokens,
		  COALESCE(sum(ur.completion_tokens), 0)::bigint AS completion_tokens,
		  COALESCE(sum(ur.total_tokens), 0)::bigint AS total_tokens,
		  COALESCE(sum(cr.total_cost_micro), 0)::bigint AS total_cost_micro_usd
		FROM usage_records ur
		LEFT JOIN cost_records cr
		  ON cr.org_id = ur.org_id
		 AND cr.usage_record_id = ur.id
		WHERE ur.org_id = $1
		  AND ur.created_at >= $2
		  AND ur.created_at < $3
		GROUP BY ur.model_id
		ORDER BY total_cost_micro_usd DESC, request_count DESC
	`, orgID, window.From, window.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []ModelCostBreakdownItem{}
	for rows.Next() {
		var item ModelCostBreakdownItem
		if err := rows.Scan(&item.ModelID, &item.RequestCount, &item.PromptTokens, &item.CompletionTokens, &item.TotalTokens, &item.TotalCostMicroUSD); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *UsageAnalyticsService) providerLatencyPostgres(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]ProviderLatencyItem, error) {
	rows, err := s.store.Pool.Query(ctx, `
		SELECT
		  provider_id,
		  count(*)::bigint AS request_count,
		  COALESCE(avg(latency_ms), 0)::double precision AS avg_latency_ms,
		  COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY latency_ms), 0)::double precision AS p50_latency_ms,
		  COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms), 0)::double precision AS p95_latency_ms,
		  COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms), 0)::double precision AS p99_latency_ms
		FROM request_logs
		WHERE org_id = $1
		  AND completed_at >= $2
		  AND completed_at < $3
		  AND provider_id IS NOT NULL
		GROUP BY provider_id
		ORDER BY p95_latency_ms DESC, request_count DESC
	`, orgID, window.From, window.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []ProviderLatencyItem{}
	for rows.Next() {
		var item ProviderLatencyItem
		if err := rows.Scan(&item.ProviderID, &item.RequestCount, &item.AverageLatencyMS, &item.P50LatencyMS, &item.P95LatencyMS, &item.P99LatencyMS); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *UsageAnalyticsService) errorRateTrendPostgres(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]ErrorRatePoint, error) {
	rows, err := s.store.Pool.Query(ctx, `
		SELECT
		  to_char(date_trunc('day', completed_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
		  count(*)::bigint AS request_count,
		  count(*) FILTER (WHERE status NOT IN ($4, $5))::bigint AS error_count,
		  CASE
		    WHEN count(*) = 0 THEN 0
		    ELSE (count(*) FILTER (WHERE status NOT IN ($4, $5))::double precision / count(*)::double precision)
		  END AS error_rate
		FROM request_logs
		WHERE org_id = $1
		  AND completed_at >= $2
		  AND completed_at < $3
		GROUP BY day
		ORDER BY day ASC
	`, orgID, window.From, window.To, statusSuccess, statusBudgetWarned)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []ErrorRatePoint{}
	for rows.Next() {
		var item ErrorRatePoint
		if err := rows.Scan(&item.Day, &item.RequestCount, &item.ErrorCount, &item.ErrorRate); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *UsageAnalyticsService) dailyCostTrendClickHouse(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]DailyCostPoint, error) {
	query := fmt.Sprintf(`
		SELECT
		  toString(toDate(created_at, 'UTC')) AS day,
		  count() AS request_count,
		  sum(total_cost_micro_usd) AS total_cost_micro_usd
		FROM usage_events
		WHERE org_id = '%s'
		  AND created_at >= parseDateTime64BestEffort('%s', 3, 'UTC')
		  AND created_at < parseDateTime64BestEffort('%s', 3, 'UTC')
		GROUP BY day
		ORDER BY day ASC
	`, orgID, clickHouseTime(window.From), clickHouseTime(window.To))
	var items []DailyCostPoint
	err := s.clickhouse.QueryJSONEachRow(ctx, query, func(raw json.RawMessage) error {
		var item DailyCostPoint
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		items = append(items, item)
		return nil
	})
	return items, err
}

func (s *UsageAnalyticsService) modelCostBreakdownClickHouse(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]ModelCostBreakdownItem, error) {
	query := fmt.Sprintf(`
		SELECT
		  model_id,
		  count() AS request_count,
		  sum(prompt_tokens) AS prompt_tokens,
		  sum(completion_tokens) AS completion_tokens,
		  sum(total_tokens) AS total_tokens,
		  sum(total_cost_micro_usd) AS total_cost_micro_usd
		FROM usage_events
		WHERE org_id = '%s'
		  AND created_at >= parseDateTime64BestEffort('%s', 3, 'UTC')
		  AND created_at < parseDateTime64BestEffort('%s', 3, 'UTC')
		  AND model_id IS NOT NULL
		GROUP BY model_id
		ORDER BY total_cost_micro_usd DESC, request_count DESC
	`, orgID, clickHouseTime(window.From), clickHouseTime(window.To))
	var items []ModelCostBreakdownItem
	err := s.clickhouse.QueryJSONEachRow(ctx, query, func(raw json.RawMessage) error {
		var row struct {
			ModelID           uuid.UUID `json:"model_id"`
			RequestCount      int64     `json:"request_count"`
			PromptTokens      int64     `json:"prompt_tokens"`
			CompletionTokens  int64     `json:"completion_tokens"`
			TotalTokens       int64     `json:"total_tokens"`
			TotalCostMicroUSD int64     `json:"total_cost_micro_usd"`
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			return err
		}
		items = append(items, ModelCostBreakdownItem(row))
		return nil
	})
	return items, err
}

func (s *UsageAnalyticsService) providerLatencyClickHouse(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]ProviderLatencyItem, error) {
	query := fmt.Sprintf(`
		SELECT
		  provider_id,
		  count() AS request_count,
		  avg(latency_ms) AS avg_latency_ms,
		  quantileExact(0.50)(latency_ms) AS p50_latency_ms,
		  quantileExact(0.95)(latency_ms) AS p95_latency_ms,
		  quantileExact(0.99)(latency_ms) AS p99_latency_ms
		FROM usage_events
		WHERE org_id = '%s'
		  AND created_at >= parseDateTime64BestEffort('%s', 3, 'UTC')
		  AND created_at < parseDateTime64BestEffort('%s', 3, 'UTC')
		  AND provider_id IS NOT NULL
		GROUP BY provider_id
		ORDER BY p95_latency_ms DESC, request_count DESC
	`, orgID, clickHouseTime(window.From), clickHouseTime(window.To))
	var items []ProviderLatencyItem
	err := s.clickhouse.QueryJSONEachRow(ctx, query, func(raw json.RawMessage) error {
		var item ProviderLatencyItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		items = append(items, item)
		return nil
	})
	return items, err
}

func (s *UsageAnalyticsService) errorRateTrendClickHouse(ctx context.Context, orgID uuid.UUID, window TimeWindow) ([]ErrorRatePoint, error) {
	query := fmt.Sprintf(`
		SELECT
		  toString(toDate(created_at, 'UTC')) AS day,
		  count() AS request_count,
		  countIf(status NOT IN ('%s', '%s')) AS error_count,
		  if(count() = 0, 0, error_count / count()) AS error_rate
		FROM usage_events
		WHERE org_id = '%s'
		  AND created_at >= parseDateTime64BestEffort('%s', 3, 'UTC')
		  AND created_at < parseDateTime64BestEffort('%s', 3, 'UTC')
		GROUP BY day
		ORDER BY day ASC
	`, statusSuccess, statusBudgetWarned, orgID, clickHouseTime(window.From), clickHouseTime(window.To))
	var items []ErrorRatePoint
	err := s.clickhouse.QueryJSONEachRow(ctx, query, func(raw json.RawMessage) error {
		var item ErrorRatePoint
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		items = append(items, item)
		return nil
	})
	return items, err
}

func clickHouseTime(value time.Time) string {
	return value.UTC().Format(clickHouseTimeLayout)
}
