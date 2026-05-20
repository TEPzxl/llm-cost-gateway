package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/budget"
	"github.com/tep/llm-cost-gateway/internal/costing"
	secretcrypto "github.com/tep/llm-cost-gateway/internal/crypto"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/metering"
	"github.com/tep/llm-cost-gateway/internal/observability"
	"github.com/tep/llm-cost-gateway/internal/provider"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
	"github.com/tep/llm-cost-gateway/internal/routing"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type ChatService struct {
	store               *store.Store
	resolver            *routing.Resolver
	registry            *provider.Registry
	budgets             *budget.Service
	alerts              *budget.AlertService
	metering            *metering.Service
	metrics             *observability.Metrics
	retryPolicy         RetryPolicy
	secretEncryptionKey string
	clock               func() time.Time
}

type ChatInput struct {
	RequestID uuid.UUID
	Principal auth.APIKeyPrincipal
	Method    string
	Path      string
	RawBody   []byte
}

type ChatResult struct {
	Response ChatCompletionResponse
}

type ChatCompletionRequest struct {
	Model       string                 `json:"model"`
	Messages    []contract.ChatMessage `json:"messages"`
	Temperature *float64               `json:"temperature,omitempty"`
	MaxTokens   *int                   `json:"max_tokens,omitempty"`
	Stream      bool                   `json:"stream"`
}

type ChatCompletionResponse struct {
	ID        string       `json:"id"`
	Object    string       `json:"object"`
	Created   int64        `json:"created"`
	Model     string       `json:"model"`
	Provider  ProviderBody `json:"provider"`
	Choices   []ChoiceBody `json:"choices"`
	Usage     UsageBody    `json:"usage"`
	Cost      CostBody     `json:"cost"`
	RequestID uuid.UUID    `json:"request_id"`
}

type ProviderBody struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Model string    `json:"model"`
}

type ChoiceBody struct {
	Index        int                  `json:"index"`
	Message      contract.ChatMessage `json:"message"`
	FinishReason string               `json:"finish_reason"`
}

type UsageBody struct {
	PromptTokens     int32 `json:"prompt_tokens"`
	CompletionTokens int32 `json:"completion_tokens"`
	TotalTokens      int32 `json:"total_tokens"`
}

type CostBody struct {
	Currency       string `json:"currency"`
	TotalCostMicro int64  `json:"total_cost_micro"`
}

type providerAttemptMetadata struct {
	ProviderID uuid.UUID `json:"provider_id"`
	ModelID    uuid.UUID `json:"model_id"`
	Priority   int32     `json:"priority"`
	Result     string    `json:"result"`
	ErrorCode  string    `json:"error_code,omitempty"`
	Retryable  bool      `json:"retryable,omitempty"`
	Attempts   int       `json:"attempts"`
}

func NewChatService(st *store.Store, secretEncryptionKey string, metrics *observability.Metrics, retryPolicy RetryPolicy) *ChatService {
	return &ChatService{
		store:               st,
		resolver:            routing.NewResolver(st.Queries),
		registry:            provider.NewRegistry(),
		budgets:             budget.NewService(st.Queries),
		alerts:              budget.NewAlertService(st),
		metering:            metering.NewService(st, costing.NewCalculator()),
		metrics:             metrics,
		retryPolicy:         retryPolicy.Normalize(),
		secretEncryptionKey: secretEncryptionKey,
		clock:               func() time.Time { return time.Now().UTC() },
	}
}

func (s *ChatService) Chat(ctx context.Context, input ChatInput) (ChatResult, error) {
	startedAt := s.clock()
	var request ChatCompletionRequest
	if err := json.Unmarshal(input.RawBody, &request); err != nil {
		return ChatResult{}, domain.NewError(http.StatusBadRequest, domain.CodeInvalidRequest, "invalid JSON body")
	}
	if request.Stream {
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, http.StatusBadRequest, domain.CodeStreamNotSupported, "", nil, nil, nil, startedAt)
		s.metrics.ObserveGatewayRequest("error", "", request.Model, s.clock().Sub(startedAt))
		return ChatResult{}, domain.NewError(http.StatusBadRequest, domain.CodeStreamNotSupported, "stream is not supported")
	}
	if request.Model == "" || len(request.Messages) == 0 {
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, http.StatusBadRequest, domain.CodeInvalidRequest, "", nil, nil, nil, startedAt)
		s.metrics.ObserveGatewayRequest("error", "", request.Model, s.clock().Sub(startedAt))
		return ChatResult{}, domain.NewError(http.StatusBadRequest, domain.CodeInvalidRequest, "model and messages are required")
	}

	budgetCheck, err := s.budgets.Check(ctx, input.Principal.OrgID)
	if err != nil {
		return ChatResult{}, err
	}
	if !budgetCheck.Allowed {
		metadata := budgetMetadata(budgetCheck)
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusBudgetBlocked, http.StatusPaymentRequired, domain.CodeBudgetExceeded, "", nil, nil, metadata, startedAt)
		s.metrics.IncBudgetBlocked()
		s.metrics.ObserveGatewayRequest("budget_blocked", "", request.Model, s.clock().Sub(startedAt))
		return ChatResult{}, domain.NewError(http.StatusPaymentRequired, domain.CodeBudgetExceeded, "budget exceeded")
	}

	resolved, err := s.resolver.ResolveTargets(ctx, routing.ResolveParams{
		OrgID:          input.Principal.OrgID,
		RequestedModel: request.Model,
	})
	if err != nil {
		if errors.Is(err, routing.ErrRouteNotFound) {
			_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, http.StatusNotFound, domain.CodeRouteNotFound, "", nil, nil, nil, startedAt)
			s.metrics.ObserveGatewayRequest("error", "", request.Model, s.clock().Sub(startedAt))
			return ChatResult{}, domain.NewError(http.StatusNotFound, domain.CodeRouteNotFound, "route not found")
		}
		return ChatResult{}, err
	}

	includeFallbackMetadata := resolved.RoutePolicy.Strategy == routing.StrategyFallback
	var attempts []providerAttemptMetadata
	var selected routing.ResolvedTarget
	var providerStarted time.Time
	var providerResponse *contract.ChatResponse
	var lastErr error
	var lastCode string
	var lastStatus int
	for index, target := range resolved.Targets {
		selected = target
		adapter, err := s.registry.AdapterFor(target.Provider.Type)
		if err != nil {
			lastErr = err
			lastStatus = http.StatusServiceUnavailable
			lastCode = domain.CodeProviderUnavailable
			attempts = appendProviderAttempt(attempts, target, "error", lastCode, true, 1)
			if shouldTryNextFallbackTarget(includeFallbackMetadata, true, index, len(resolved.Targets)) {
				s.metrics.ObserveProviderRequest(target.Provider.Name, request.Model, "error", lastCode)
				continue
			}
			break
		}
		providerConfig, err := s.providerConfig(ctx, target.Provider)
		if err != nil {
			lastErr = err
			lastStatus = http.StatusServiceUnavailable
			lastCode = domain.CodeProviderUnavailable
			attempts = appendProviderAttempt(attempts, target, "error", lastCode, true, 1)
			if shouldTryNextFallbackTarget(includeFallbackMetadata, true, index, len(resolved.Targets)) {
				s.metrics.ObserveProviderRequest(target.Provider.Name, request.Model, "error", lastCode)
				continue
			}
			break
		}

		providerStarted = s.clock()
		var attemptCount int
		providerResponse, attemptCount, err = s.chatProviderWithRetry(ctx, adapter, contract.ChatRequest{
			RequestID:   input.RequestID.String(),
			OrgID:       input.Principal.OrgID,
			Provider:    providerConfig,
			Model:       contract.ModelConfig{ID: target.Model.ID, ProviderModelName: target.Model.ProviderModelName, DisplayName: target.Model.DisplayName},
			Messages:    request.Messages,
			Temperature: request.Temperature,
			MaxTokens:   request.MaxTokens,
			Stream:      false,
		})
		if err == nil {
			attempts = appendProviderAttempt(attempts, target, "success", "", false, attemptCount)
			break
		}
		lastErr = err
		lastStatus, lastCode = providerErrorStatus(err)
		retryable := fallbackRetryable(err)
		attempts = appendProviderAttempt(attempts, target, "error", lastCode, retryable, attemptCount)
		s.metrics.ObserveProviderRequest(target.Provider.Name, request.Model, "error", lastCode)
		if shouldTryNextFallbackTarget(includeFallbackMetadata, retryable, index, len(resolved.Targets)) {
			continue
		}
		break
	}

	if providerResponse == nil {
		if lastErr == nil {
			lastStatus = http.StatusServiceUnavailable
			lastCode = domain.CodeProviderUnavailable
		}
		if includeFallbackMetadata && fallbackRetryable(lastErr) && len(attempts) == len(resolved.Targets) {
			lastStatus = http.StatusServiceUnavailable
			lastCode = domain.CodeProviderUnavailable
		}
		metadata := chatMetadata(budgetCheck, attempts, includeFallbackMetadata, includeFallbackMetadata || hasRetriedTarget(attempts))
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, lastStatus, lastCode, "", &selected.Provider.ID, &selected.Model.ID, metadata, startedAt)
		s.metrics.ObserveGatewayRequest("error", selected.Provider.Name, request.Model, s.clock().Sub(startedAt))
		return ChatResult{}, domain.NewError(lastStatus, lastCode, "provider request failed")
	}
	s.metrics.ObserveProviderRequest(selected.Provider.Name, request.Model, "success", "")

	providerLatencyMS := int32(providerResponse.LatencyMS)
	if providerLatencyMS == 0 {
		providerLatencyMS = int32(s.clock().Sub(providerStarted).Milliseconds())
	}
	metadata := chatMetadata(budgetCheck, attempts, includeFallbackMetadata, includeFallbackMetadata || hasRetriedTarget(attempts))
	successStatus := metering.StatusSuccess
	if budgetCheck.Warning {
		successStatus = metering.StatusBudgetWarned
	}
	rawUsage, _ := json.Marshal(providerResponse.RawUsage)
	metered, err := s.metering.RecordSuccess(ctx, metering.RecordSuccessInput{
		RequestID:                      input.RequestID,
		OrgID:                          input.Principal.OrgID,
		APIKeyID:                       input.Principal.APIKeyID,
		ProviderID:                     selected.Provider.ID,
		ModelID:                        selected.Model.ID,
		RoutePolicyID:                  &resolved.RoutePolicy.ID,
		Method:                         input.Method,
		Path:                           input.Path,
		RequestModel:                   request.Model,
		Status:                         successStatus,
		StatusCode:                     http.StatusOK,
		RequestHash:                    hashBytes(input.RawBody),
		ResponseHash:                   hashBytes([]byte(providerResponse.Content)),
		LatencyMS:                      int32(s.clock().Sub(startedAt).Milliseconds()),
		ProviderLatencyMS:              &providerLatencyMS,
		Metadata:                       metadata,
		ProviderUsageJSON:              rawUsage,
		PromptTokens:                   int64(providerResponse.Usage.PromptTokens),
		CompletionTokens:               int64(providerResponse.Usage.CompletionTokens),
		InputPriceMicroUSDPer1KTokens:  selected.Model.InputPriceMicroUsdPer1kTokens,
		OutputPriceMicroUSDPer1KTokens: selected.Model.OutputPriceMicroUsdPer1kTokens,
		StartedAt:                      startedAt,
		CompletedAt:                    s.clock(),
	})
	if err != nil {
		return ChatResult{}, err
	}
	_, _ = s.alerts.CheckAndDeliver(ctx, input.Principal.OrgID)
	statusLabel := "success"
	if budgetCheck.Warning {
		statusLabel = metering.StatusBudgetWarned
	}
	s.metrics.ObserveGatewayRequest(statusLabel, selected.Provider.Name, request.Model, s.clock().Sub(startedAt))
	s.metrics.AddTokens(request.Model, int64(providerResponse.Usage.PromptTokens), int64(providerResponse.Usage.CompletionTokens))
	s.metrics.AddCostMicroUSD(request.Model, metered.CostRecord.TotalCostMicro)

	response := ChatCompletionResponse{
		ID:      "chatcmpl_" + input.RequestID.String(),
		Object:  "chat.completion",
		Created: s.clock().Unix(),
		Model:   request.Model,
		Provider: ProviderBody{
			ID:    selected.Provider.ID,
			Name:  selected.Provider.Name,
			Model: selected.Model.ProviderModelName,
		},
		Choices: []ChoiceBody{
			{
				Index: 0,
				Message: contract.ChatMessage{
					Role:    providerResponse.Role,
					Content: providerResponse.Content,
				},
				FinishReason: providerResponse.FinishReason,
			},
		},
		Usage: UsageBody{
			PromptTokens:     metered.UsageRecord.PromptTokens,
			CompletionTokens: metered.UsageRecord.CompletionTokens,
			TotalTokens:      metered.UsageRecord.TotalTokens,
		},
		Cost: CostBody{
			Currency:       metered.CostRecord.Currency,
			TotalCostMicro: metered.CostRecord.TotalCostMicro,
		},
		RequestID: input.RequestID,
	}
	return ChatResult{Response: response}, nil
}

func (s *ChatService) recordFailure(ctx context.Context, input ChatInput, requestModel string, status string, statusCode int, errorCode string, responseHash string, providerID *uuid.UUID, modelID *uuid.UUID, metadata *json.RawMessage, startedAt time.Time) error {
	_, err := s.metering.RecordFailure(ctx, metering.RecordFailureInput{
		RequestID:    input.RequestID,
		OrgID:        input.Principal.OrgID,
		APIKeyID:     &input.Principal.APIKeyID,
		ProviderID:   providerID,
		ModelID:      modelID,
		Method:       input.Method,
		Path:         input.Path,
		RequestModel: requestModel,
		Status:       status,
		StatusCode:   int32(statusCode),
		ErrorCode:    errorCode,
		RequestHash:  hashBytes(input.RawBody),
		ResponseHash: responseHash,
		LatencyMS:    int32(s.clock().Sub(startedAt).Milliseconds()),
		Metadata:     metadata,
		StartedAt:    startedAt,
		CompletedAt:  s.clock(),
	})
	return err
}

func (s *ChatService) providerConfig(ctx context.Context, item db.Provider) (contract.ProviderConfig, error) {
	cfg := contract.ProviderConfig{
		ID:        item.ID,
		Name:      item.Name,
		Type:      item.Type,
		TimeoutMS: item.TimeoutMs,
	}
	if item.BaseUrl.Valid {
		cfg.BaseURL = item.BaseUrl.String
	}
	if item.Type != provider.TypeOpenAICompatible {
		return cfg, nil
	}

	secret, err := s.store.Queries.GetProviderSecret(ctx, db.GetProviderSecretParams{
		OrgID:      item.OrgID,
		ProviderID: item.ID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return contract.ProviderConfig{}, err
		}
		return contract.ProviderConfig{}, err
	}
	box, err := secretcrypto.NewSecretBox(s.secretEncryptionKey)
	if err != nil {
		return contract.ProviderConfig{}, err
	}
	apiKey, err := box.Open(secret.EncryptedApiKey, secret.Nonce)
	if err != nil {
		return contract.ProviderConfig{}, err
	}
	cfg.APIKey = apiKey
	return cfg, nil
}

func (s *ChatService) chatProviderWithRetry(ctx context.Context, adapter contract.Adapter, request contract.ChatRequest) (*contract.ChatResponse, int, error) {
	policy := s.retryPolicy.Normalize()
	maxAttempts := policy.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		response, err := adapter.Chat(ctx, request)
		if err == nil {
			return response, attempt, nil
		}
		lastErr = err
		if attempt == maxAttempts || !s.retryPolicy.ShouldRetry(err) {
			return nil, attempt, err
		}
		timer := time.NewTimer(policy.Backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, attempt, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, maxAttempts, lastErr
}

func providerErrorStatus(err error) (int, string) {
	switch contract.ErrorCode(err) {
	case domain.CodeProviderTimeout:
		return http.StatusGatewayTimeout, domain.CodeProviderTimeout
	case domain.CodeProviderUnavailable:
		return http.StatusServiceUnavailable, domain.CodeProviderUnavailable
	case domain.CodeUsageMissing:
		return http.StatusBadGateway, domain.CodeUsageMissing
	default:
		return http.StatusBadGateway, domain.CodeProviderError
	}
}

func appendProviderAttempt(attempts []providerAttemptMetadata, target routing.ResolvedTarget, result string, errorCode string, retryable bool, attemptCount int) []providerAttemptMetadata {
	if attemptCount < 1 {
		attemptCount = 1
	}
	return append(attempts, providerAttemptMetadata{
		ProviderID: target.Provider.ID,
		ModelID:    target.Model.ID,
		Priority:   target.RouteTarget.Priority,
		Result:     result,
		ErrorCode:  errorCode,
		Retryable:  retryable,
		Attempts:   attemptCount,
	})
}

func shouldTryNextFallbackTarget(fallbackPolicy bool, retryable bool, index int, targetCount int) bool {
	return fallbackPolicy && retryable && index < targetCount-1
}

func fallbackRetryable(err error) bool {
	if err == nil {
		return false
	}
	if contract.Retryable(err) {
		return true
	}
	switch contract.ErrorCode(err) {
	case domain.CodeProviderTimeout, domain.CodeProviderUnavailable:
		return true
	default:
		return false
	}
}

func hasRetriedTarget(attempts []providerAttemptMetadata) bool {
	for _, attempt := range attempts {
		if attempt.Attempts > 1 {
			return true
		}
	}
	return false
}

func chatMetadata(result budget.CheckResult, attempts []providerAttemptMetadata, includeFallback bool, includeAttempts bool) *json.RawMessage {
	body := map[string]any{}
	if budgetBody := budgetMetadataBody(result); budgetBody != nil {
		body["budget"] = budgetBody
	}
	if includeFallback {
		fallbackCount := len(attempts) - 1
		if fallbackCount < 0 {
			fallbackCount = 0
		}
		body["fallback_count"] = fallbackCount
	}
	if includeAttempts {
		body["attempted_targets"] = attempts
	}
	if len(body) == 0 {
		return nil
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	raw := json.RawMessage(rawBody)
	return &raw
}

func budgetMetadata(result budget.CheckResult) *json.RawMessage {
	body := budgetMetadataBody(result)
	if body == nil {
		return nil
	}
	rawBody, err := json.Marshal(map[string]any{"budget": body})
	if err != nil {
		return nil
	}
	raw := json.RawMessage(rawBody)
	return &raw
}

func budgetMetadataBody(result budget.CheckResult) map[string]any {
	if result.Status == nil {
		return nil
	}
	return map[string]any{
		"id":                   result.Status.Budget.ID,
		"action":               result.Action,
		"used_micro_usd":       result.Status.UsedMicroUSD,
		"limit_micro_usd":      result.Status.Budget.LimitMicroUsd,
		"remaining_micro_usd":  result.Status.RemainingMicroUSD,
		"exceeded":             result.Status.Exceeded,
		"period":               result.Status.Budget.Period,
		"window_start":         result.Status.WindowStart,
		"window_end":           result.Status.WindowEnd,
		"pre_check_only":       true,
		"projected_cost_known": false,
	}
}

func hashBytes(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
