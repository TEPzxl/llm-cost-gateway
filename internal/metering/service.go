package metering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/analytics"
	"github.com/tep/llm-cost-gateway/internal/costing"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/events"
	"github.com/tep/llm-cost-gateway/internal/limits"
	"github.com/tep/llm-cost-gateway/internal/observability"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"go.uber.org/zap"
)

const (
	StatusSuccess       = "success"
	StatusError         = "error"
	StatusRateLimited   = "rate_limited"
	StatusBudgetWarned  = "budget_warned"
	StatusBudgetBlocked = "budget_blocked"
)

var ErrInvalidMeteringInput = errors.New("invalid metering input")

type Service struct {
	store      *store.Store
	calculator *costing.Calculator
	now        func() time.Time
	publisher  events.Publisher
	logger     *zap.Logger
	metrics    *observability.Metrics
	analytics  analytics.Sink
}

type Option func(*Service)

func WithUsageEventPublisher(publisher events.Publisher) Option {
	return func(s *Service) {
		if publisher != nil {
			s.publisher = publisher
		}
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(s *Service) {
		if logger != nil {
			s.logger = logger
		}
	}
}

func WithMetrics(metrics *observability.Metrics) Option {
	return func(s *Service) {
		s.metrics = metrics
	}
}

func WithUsageAnalyticsSink(sink analytics.Sink) Option {
	return func(s *Service) {
		if sink != nil {
			s.analytics = sink
		}
	}
}

type RecordSuccessInput struct {
	RequestID                      uuid.UUID
	OrgID                          uuid.UUID
	APIKeyID                       uuid.UUID
	ProviderID                     uuid.UUID
	ModelID                        uuid.UUID
	RoutePolicyID                  *uuid.UUID
	Method                         string
	Path                           string
	RequestModel                   string
	Status                         string
	StatusCode                     int32
	RequestHash                    string
	ResponseHash                   string
	LatencyMS                      int32
	ProviderLatencyMS              *int32
	Metadata                       *json.RawMessage
	ProviderUsageJSON              json.RawMessage
	PromptTokens                   int64
	CompletionTokens               int64
	InputPriceMicroUSDPer1KTokens  int64
	OutputPriceMicroUSDPer1KTokens int64
	StartedAt                      time.Time
	CompletedAt                    time.Time
}

type RecordFailureInput struct {
	RequestID         uuid.UUID
	OrgID             uuid.UUID
	APIKeyID          *uuid.UUID
	ProviderID        *uuid.UUID
	ModelID           *uuid.UUID
	RoutePolicyID     *uuid.UUID
	Method            string
	Path              string
	RequestModel      string
	Status            string
	StatusCode        int32
	ErrorCode         string
	RequestHash       string
	ResponseHash      string
	LatencyMS         int32
	ProviderLatencyMS *int32
	Metadata          *json.RawMessage
	StartedAt         time.Time
	CompletedAt       time.Time
}

type RecordCacheHitInput struct {
	RequestID         uuid.UUID
	OrgID             uuid.UUID
	APIKeyID          uuid.UUID
	ProviderID        uuid.UUID
	ModelID           uuid.UUID
	RoutePolicyID     *uuid.UUID
	Method            string
	Path              string
	RequestModel      string
	Status            string
	StatusCode        int32
	RequestHash       string
	ResponseHash      string
	LatencyMS         int32
	ProviderLatencyMS *int32
	Metadata          *json.RawMessage
	StartedAt         time.Time
	CompletedAt       time.Time
}

type RecordSuccessResult struct {
	RequestLog  db.RequestLog
	UsageRecord db.UsageRecord
	CostRecord  db.CostRecord
}

func NewService(st *store.Store, calculator *costing.Calculator, opts ...Option) *Service {
	if calculator == nil {
		calculator = costing.NewCalculator()
	}
	service := &Service{
		store:      st,
		calculator: calculator,
		now:        func() time.Time { return time.Now().UTC() },
		publisher:  events.DisabledPublisher{},
		logger:     zap.NewNop(),
		analytics:  analytics.DisabledSink{},
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

func (s *Service) RecordSuccess(ctx context.Context, input RecordSuccessInput) (RecordSuccessResult, error) {
	if err := validateSuccessInput(input); err != nil {
		return RecordSuccessResult{}, err
	}

	var result RecordSuccessResult
	err := s.store.ExecTxRaw(ctx, func(tx pgx.Tx, q *db.Queries) error {
		startedAt := nonZeroTime(input.StartedAt, s.now())
		completedAt := nonZeroTime(input.CompletedAt, s.now())
		requestLog, err := q.InsertRequestLog(ctx, db.InsertRequestLogParams{
			ID:                input.RequestID,
			OrgID:             input.OrgID,
			ApiKeyID:          &input.APIKeyID,
			ProviderID:        &input.ProviderID,
			ModelID:           &input.ModelID,
			RoutePolicyID:     input.RoutePolicyID,
			Method:            input.Method,
			Path:              input.Path,
			RequestModel:      textValue(input.RequestModel),
			Status:            successStatus(input.Status),
			StatusCode:        input.StatusCode,
			RequestHash:       textValue(input.RequestHash),
			ResponseHash:      textValue(input.ResponseHash),
			LatencyMs:         input.LatencyMS,
			ProviderLatencyMs: int4Value(input.ProviderLatencyMS),
			Metadata:          input.Metadata,
			StartedAt:         startedAt,
			CompletedAt:       completedAt,
		})
		if err != nil {
			return err
		}

		providerUsage := rawMessagePtr(input.ProviderUsageJSON)
		usageRecord, err := q.InsertUsageRecord(ctx, db.InsertUsageRecordParams{
			ID:                uuid.New(),
			RequestLogID:      requestLog.ID,
			OrgID:             input.OrgID,
			ApiKeyID:          input.APIKeyID,
			ProviderID:        input.ProviderID,
			ModelID:           input.ModelID,
			PromptTokens:      int32(input.PromptTokens),
			CompletionTokens:  int32(input.CompletionTokens),
			TotalTokens:       int32(input.PromptTokens + input.CompletionTokens),
			ProviderUsageJson: providerUsage,
			CreatedAt:         completedAt,
		})
		if err != nil {
			return err
		}

		pricingVersion, err := q.GetActiveModelPricingVersion(ctx, db.GetActiveModelPricingVersionParams{
			OrgID:   input.OrgID,
			ModelID: input.ModelID,
		})
		if err != nil {
			return err
		}
		calculation, err := s.calculator.Calculate(costing.CalculateInput{
			PromptTokens:                   input.PromptTokens,
			CompletionTokens:               input.CompletionTokens,
			InputPriceMicroUSDPer1KTokens:  pricingVersion.InputPriceMicroUsdPer1kTokens,
			OutputPriceMicroUSDPer1KTokens: pricingVersion.OutputPriceMicroUsdPer1kTokens,
		})
		if err != nil {
			return err
		}
		pricingSnapshot, err := json.Marshal(calculation.PricingSnapshot)
		if err != nil {
			return err
		}
		costRecord, err := q.InsertCostRecord(ctx, db.InsertCostRecordParams{
			ID:               uuid.New(),
			UsageRecordID:    usageRecord.ID,
			OrgID:            input.OrgID,
			ProviderID:       input.ProviderID,
			ModelID:          input.ModelID,
			Currency:         calculation.Currency,
			InputCostMicro:   calculation.InputCostMicro,
			OutputCostMicro:  calculation.OutputCostMicro,
			TotalCostMicro:   calculation.TotalCostMicro,
			PricingVersionID: &pricingVersion.ID,
			PricingSnapshot:  pricingSnapshot,
			CreatedAt:        completedAt,
		})
		if err != nil {
			return err
		}

		if err := limits.SettleTx(ctx, tx, input.RequestID, costRecord.TotalCostMicro, completedAt); err != nil {
			return err
		}

		result = RecordSuccessResult{
			RequestLog:  requestLog,
			UsageRecord: usageRecord,
			CostRecord:  costRecord,
		}
		return nil
	})
	if err != nil {
		return RecordSuccessResult{}, err
	}
	s.publishUsageEvent(ctx, events.UsageEvent{
		Type:              events.EventUsageRecorded,
		OrgID:             input.OrgID,
		APIKeyID:          uuidPtr(input.APIKeyID),
		ProviderID:        uuidPtr(input.ProviderID),
		ModelID:           uuidPtr(input.ModelID),
		RequestID:         result.RequestLog.ID,
		PromptTokens:      int64(result.UsageRecord.PromptTokens),
		CompletionTokens:  int64(result.UsageRecord.CompletionTokens),
		TotalTokens:       int64(result.UsageRecord.TotalTokens),
		TotalCostMicroUSD: result.CostRecord.TotalCostMicro,
		Status:            result.RequestLog.Status,
		CreatedAt:         result.UsageRecord.CreatedAt,
	})
	s.writeAnalyticsRecord(ctx, analytics.UsageRecord{
		RequestID:         result.RequestLog.ID,
		OrgID:             input.OrgID,
		APIKeyID:          uuidPtr(input.APIKeyID),
		ProviderID:        uuidPtr(input.ProviderID),
		ModelID:           uuidPtr(input.ModelID),
		Status:            result.RequestLog.Status,
		PromptTokens:      int64(result.UsageRecord.PromptTokens),
		CompletionTokens:  int64(result.UsageRecord.CompletionTokens),
		TotalTokens:       int64(result.UsageRecord.TotalTokens),
		TotalCostMicroUSD: result.CostRecord.TotalCostMicro,
		LatencyMS:         result.RequestLog.LatencyMs,
		CreatedAt:         result.RequestLog.CompletedAt,
	})
	return result, nil
}

func (s *Service) RecordFailure(ctx context.Context, input RecordFailureInput) (db.RequestLog, error) {
	if err := validateFailureInput(input); err != nil {
		return db.RequestLog{}, err
	}

	requestLog, err := s.store.Queries.InsertRequestLog(ctx, db.InsertRequestLogParams{
		ID:                input.RequestID,
		OrgID:             input.OrgID,
		ApiKeyID:          input.APIKeyID,
		ProviderID:        input.ProviderID,
		ModelID:           input.ModelID,
		RoutePolicyID:     input.RoutePolicyID,
		Method:            input.Method,
		Path:              input.Path,
		RequestModel:      textValue(input.RequestModel),
		Status:            input.Status,
		StatusCode:        input.StatusCode,
		ErrorCode:         textValue(input.ErrorCode),
		RequestHash:       textValue(input.RequestHash),
		ResponseHash:      textValue(input.ResponseHash),
		LatencyMs:         input.LatencyMS,
		ProviderLatencyMs: int4Value(input.ProviderLatencyMS),
		Metadata:          input.Metadata,
		StartedAt:         nonZeroTime(input.StartedAt, s.now()),
		CompletedAt:       nonZeroTime(input.CompletedAt, s.now()),
	})
	if err != nil {
		return db.RequestLog{}, err
	}
	if input.Status == StatusBudgetBlocked && input.ErrorCode == domain.CodeBudgetExceeded {
		s.publishUsageEvent(ctx, events.UsageEvent{
			Type:              events.EventRequestBlocked,
			OrgID:             input.OrgID,
			APIKeyID:          input.APIKeyID,
			ProviderID:        input.ProviderID,
			ModelID:           input.ModelID,
			RequestID:         requestLog.ID,
			PromptTokens:      0,
			CompletionTokens:  0,
			TotalTokens:       0,
			TotalCostMicroUSD: 0,
			Status:            requestLog.Status,
			CreatedAt:         requestLog.CompletedAt,
		})
	}
	s.writeAnalyticsRecord(ctx, analytics.UsageRecord{
		RequestID:         requestLog.ID,
		OrgID:             input.OrgID,
		APIKeyID:          input.APIKeyID,
		ProviderID:        input.ProviderID,
		ModelID:           input.ModelID,
		Status:            requestLog.Status,
		PromptTokens:      0,
		CompletionTokens:  0,
		TotalTokens:       0,
		TotalCostMicroUSD: 0,
		LatencyMS:         requestLog.LatencyMs,
		CreatedAt:         requestLog.CompletedAt,
	})
	return requestLog, nil
}

func (s *Service) RecordCacheHit(ctx context.Context, input RecordCacheHitInput) (db.RequestLog, error) {
	if err := validateCacheHitInput(input); err != nil {
		return db.RequestLog{}, err
	}
	requestLog, err := s.store.Queries.InsertRequestLog(ctx, db.InsertRequestLogParams{
		ID:                input.RequestID,
		OrgID:             input.OrgID,
		ApiKeyID:          &input.APIKeyID,
		ProviderID:        &input.ProviderID,
		ModelID:           &input.ModelID,
		RoutePolicyID:     input.RoutePolicyID,
		Method:            input.Method,
		Path:              input.Path,
		RequestModel:      textValue(input.RequestModel),
		Status:            successStatus(input.Status),
		StatusCode:        input.StatusCode,
		RequestHash:       textValue(input.RequestHash),
		ResponseHash:      textValue(input.ResponseHash),
		LatencyMs:         input.LatencyMS,
		ProviderLatencyMs: int4Value(input.ProviderLatencyMS),
		Metadata:          input.Metadata,
		StartedAt:         nonZeroTime(input.StartedAt, s.now()),
		CompletedAt:       nonZeroTime(input.CompletedAt, s.now()),
	})
	if err != nil {
		return db.RequestLog{}, err
	}
	s.writeAnalyticsRecord(ctx, analytics.UsageRecord{
		RequestID:         requestLog.ID,
		OrgID:             input.OrgID,
		APIKeyID:          uuidPtr(input.APIKeyID),
		ProviderID:        uuidPtr(input.ProviderID),
		ModelID:           uuidPtr(input.ModelID),
		Status:            requestLog.Status,
		PromptTokens:      0,
		CompletionTokens:  0,
		TotalTokens:       0,
		TotalCostMicroUSD: 0,
		LatencyMS:         requestLog.LatencyMs,
		CreatedAt:         requestLog.CompletedAt,
	})
	return requestLog, nil
}

func (s *Service) publishUsageEvent(ctx context.Context, event events.UsageEvent) {
	if s.publisher == nil {
		return
	}
	if err := s.publisher.Publish(ctx, event); err != nil {
		if s.logger != nil {
			s.logger.Warn(
				"publish usage event failed",
				zap.Error(err),
				zap.String("event_type", event.Type),
				zap.String("request_id", event.RequestID.String()),
				zap.String("org_id", event.OrgID.String()),
			)
		}
		if s.metrics != nil {
			s.metrics.IncUsageEventPublishFailed()
		}
	}
}

func (s *Service) writeAnalyticsRecord(ctx context.Context, record analytics.UsageRecord) {
	if s.analytics == nil {
		return
	}
	if err := s.analytics.WriteUsageRecord(ctx, record); err != nil {
		if s.logger != nil {
			s.logger.Warn(
				"write analytics record failed",
				zap.Error(err),
				zap.String("request_id", record.RequestID.String()),
				zap.String("org_id", record.OrgID.String()),
			)
		}
		if s.metrics != nil {
			s.metrics.IncAnalyticsWriteFailed()
		}
	}
}

func validateSuccessInput(input RecordSuccessInput) error {
	if input.RequestID == uuid.Nil {
		return fmt.Errorf("%w: request_id is required", ErrInvalidMeteringInput)
	}
	if input.OrgID == uuid.Nil || input.APIKeyID == uuid.Nil || input.ProviderID == uuid.Nil || input.ModelID == uuid.Nil {
		return fmt.Errorf("%w: org_id, api_key_id, provider_id and model_id are required", ErrInvalidMeteringInput)
	}
	if input.Method == "" || input.Path == "" {
		return fmt.Errorf("%w: method and path are required", ErrInvalidMeteringInput)
	}
	if input.StatusCode < 100 || input.StatusCode > 599 {
		return fmt.Errorf("%w: status_code must be valid HTTP status", ErrInvalidMeteringInput)
	}
	if input.Status != "" && input.Status != StatusSuccess && input.Status != StatusBudgetWarned {
		return fmt.Errorf("%w: invalid success status", ErrInvalidMeteringInput)
	}
	if input.LatencyMS < 0 {
		return fmt.Errorf("%w: latency_ms must be non-negative", ErrInvalidMeteringInput)
	}
	if input.PromptTokens < 0 || input.CompletionTokens < 0 {
		return fmt.Errorf("%w: tokens must be non-negative", ErrInvalidMeteringInput)
	}
	if input.PromptTokens > math.MaxInt32 || input.CompletionTokens > math.MaxInt32 || input.PromptTokens+input.CompletionTokens > math.MaxInt32 {
		return fmt.Errorf("%w: tokens exceed database integer range", ErrInvalidMeteringInput)
	}
	return nil
}

func successStatus(status string) string {
	if status == "" {
		return StatusSuccess
	}
	return status
}

func validateFailureInput(input RecordFailureInput) error {
	if input.RequestID == uuid.Nil {
		return fmt.Errorf("%w: request_id is required", ErrInvalidMeteringInput)
	}
	if input.OrgID == uuid.Nil {
		return fmt.Errorf("%w: org_id is required", ErrInvalidMeteringInput)
	}
	if input.Method == "" || input.Path == "" {
		return fmt.Errorf("%w: method and path are required", ErrInvalidMeteringInput)
	}
	if !validFailureStatus(input.Status) {
		return fmt.Errorf("%w: invalid failure status", ErrInvalidMeteringInput)
	}
	if input.StatusCode < 100 || input.StatusCode > 599 {
		return fmt.Errorf("%w: status_code must be valid HTTP status", ErrInvalidMeteringInput)
	}
	if input.LatencyMS < 0 {
		return fmt.Errorf("%w: latency_ms must be non-negative", ErrInvalidMeteringInput)
	}
	return nil
}

func validateCacheHitInput(input RecordCacheHitInput) error {
	if input.RequestID == uuid.Nil {
		return fmt.Errorf("%w: request_id is required", ErrInvalidMeteringInput)
	}
	if input.OrgID == uuid.Nil || input.APIKeyID == uuid.Nil || input.ProviderID == uuid.Nil || input.ModelID == uuid.Nil {
		return fmt.Errorf("%w: org_id, api_key_id, provider_id and model_id are required", ErrInvalidMeteringInput)
	}
	if input.Method == "" || input.Path == "" {
		return fmt.Errorf("%w: method and path are required", ErrInvalidMeteringInput)
	}
	if input.Status != "" && input.Status != StatusSuccess && input.Status != StatusBudgetWarned {
		return fmt.Errorf("%w: invalid cache hit status", ErrInvalidMeteringInput)
	}
	if input.StatusCode < 100 || input.StatusCode > 599 {
		return fmt.Errorf("%w: status_code must be valid HTTP status", ErrInvalidMeteringInput)
	}
	if input.LatencyMS < 0 {
		return fmt.Errorf("%w: latency_ms must be non-negative", ErrInvalidMeteringInput)
	}
	return nil
}

func validFailureStatus(status string) bool {
	switch status {
	case StatusError, StatusRateLimited, StatusBudgetWarned, StatusBudgetBlocked:
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

func int4Value(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func rawMessagePtr(value json.RawMessage) *json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	copied := append(json.RawMessage(nil), value...)
	return &copied
}

func nonZeroTime(value time.Time, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}

func uuidPtr(value uuid.UUID) *uuid.UUID {
	return &value
}
