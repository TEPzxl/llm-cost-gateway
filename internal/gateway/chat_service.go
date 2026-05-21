package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/analytics"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/budget"
	promptcache "github.com/tep/llm-cost-gateway/internal/cache"
	"github.com/tep/llm-cost-gateway/internal/costing"
	secretcrypto "github.com/tep/llm-cost-gateway/internal/crypto"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/embedding"
	"github.com/tep/llm-cost-gateway/internal/events"
	"github.com/tep/llm-cost-gateway/internal/metering"
	"github.com/tep/llm-cost-gateway/internal/observability"
	"github.com/tep/llm-cost-gateway/internal/provider"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
	"github.com/tep/llm-cost-gateway/internal/routing"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"go.uber.org/zap"
)

type ChatService struct {
	store               *store.Store
	resolver            *routing.Resolver
	registry            *provider.Registry
	budgets             *budget.Service
	alerts              *budget.AlertService
	apiKeyQuotas        *auth.APIKeyQuotaService
	metering            *metering.Service
	metrics             *observability.Metrics
	retryPolicy         RetryPolicy
	secretEncryptionKey string
	clock               func() time.Time
	usageEvents         events.Publisher
	usageAnalytics      analytics.Sink
	promptCache         *promptcache.PromptCache
	semanticCache       *promptcache.SemanticCache
	embedding           embedding.Adapter
	semanticThreshold   float64
	semanticMaxTemp     float64
	logger              *zap.Logger
}

type ChatServiceOption func(*ChatService)

func WithUsageEventPublisher(publisher events.Publisher) ChatServiceOption {
	return func(s *ChatService) {
		if publisher != nil {
			s.usageEvents = publisher
		}
	}
}

func WithLogger(logger *zap.Logger) ChatServiceOption {
	return func(s *ChatService) {
		if logger != nil {
			s.logger = logger
		}
	}
}

func WithUsageAnalyticsSink(sink analytics.Sink) ChatServiceOption {
	return func(s *ChatService) {
		if sink != nil {
			s.usageAnalytics = sink
		}
	}
}

func WithPromptCache(cache *promptcache.PromptCache) ChatServiceOption {
	return func(s *ChatService) {
		s.promptCache = cache
	}
}

func WithSemanticCache(cache *promptcache.SemanticCache, adapter embedding.Adapter, threshold float64, maxTemperature float64) ChatServiceOption {
	return func(s *ChatService) {
		s.semanticCache = cache
		s.embedding = adapter
		s.semanticThreshold = threshold
		s.semanticMaxTemp = maxTemperature
	}
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

type cacheEventInput struct {
	OrgID          uuid.UUID
	RequestLogID   *uuid.UUID
	EventType      string
	RequestedModel string
	CacheKey       promptcache.PromptKey
	Reason         string
}

type semanticCacheCandidate struct {
	enabled      bool
	skipReason   string
	cacheKeyHash string
	messagesHash string
	embedding    []float64
}

func NewChatService(st *store.Store, secretEncryptionKey string, metrics *observability.Metrics, retryPolicy RetryPolicy, opts ...ChatServiceOption) *ChatService {
	service := &ChatService{
		store:               st,
		resolver:            routing.NewResolver(st.Queries),
		registry:            provider.NewRegistry(),
		budgets:             budget.NewService(st.Queries),
		alerts:              budget.NewAlertService(st),
		apiKeyQuotas:        auth.NewAPIKeyQuotaService(st.Queries),
		metering:            metering.NewService(st, costing.NewCalculator()),
		metrics:             metrics,
		retryPolicy:         retryPolicy.Normalize(),
		secretEncryptionKey: secretEncryptionKey,
		clock:               func() time.Time { return time.Now().UTC() },
		usageEvents:         events.DisabledPublisher{},
		usageAnalytics:      analytics.DisabledSink{},
		semanticThreshold:   0.92,
		semanticMaxTemp:     0.3,
		logger:              zap.NewNop(),
	}
	for _, opt := range opts {
		opt(service)
	}
	service.metering = metering.NewService(
		st,
		costing.NewCalculator(),
		metering.WithUsageEventPublisher(service.usageEvents),
		metering.WithUsageAnalyticsSink(service.usageAnalytics),
		metering.WithLogger(service.logger),
		metering.WithMetrics(metrics),
	)
	return service
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
	quotaCheck, err := s.apiKeyQuotas.Check(ctx, input.Principal)
	if err != nil {
		return ChatResult{}, err
	}
	if !quotaCheck.Allowed {
		metadata := apiKeyQuotaMetadata(quotaCheck)
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusBudgetBlocked, http.StatusPaymentRequired, domain.CodeAPIKeyQuotaExceeded, "", nil, nil, metadata, startedAt)
		s.metrics.ObserveGatewayRequest("api_key_quota_blocked", "", request.Model, s.clock().Sub(startedAt))
		return ChatResult{}, domain.NewError(http.StatusPaymentRequired, domain.CodeAPIKeyQuotaExceeded, "api key quota exceeded")
	}

	resolved, err := s.resolver.ResolveTargets(ctx, routing.ResolveParams{
		OrgID:                 input.Principal.OrgID,
		RequestedModel:        request.Model,
		EstimatedPromptTokens: estimatePromptTokens(request.Messages),
		EstimatedMaxTokens:    estimateMaxTokens(request.MaxTokens),
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
	cacheKey, cacheable := s.promptCacheKey(request, input.Principal.OrgID)
	if cacheable {
		if cached, err := s.promptCache.Get(ctx, cacheKey); err == nil {
			return s.respondFromPromptCache(ctx, input, request, resolved, cached, cacheKey, budgetCheck, quotaCheck, startedAt)
		} else if errors.Is(err, promptcache.ErrCacheMiss) {
			s.recordCacheEvent(ctx, cacheEventInput{
				OrgID:          input.Principal.OrgID,
				EventType:      "miss",
				RequestedModel: request.Model,
				CacheKey:       cacheKey,
			})
		} else {
			s.recordCacheEvent(ctx, cacheEventInput{
				OrgID:          input.Principal.OrgID,
				EventType:      "read_error",
				RequestedModel: request.Model,
				CacheKey:       cacheKey,
				Reason:         "redis_error",
			})
			s.logger.Warn("prompt cache read failed", zap.Error(err), zap.String("request_id", input.RequestID.String()))
		}
	}
	semanticCandidate := s.semanticCacheCandidate(ctx, input, request, cacheKey)
	if semanticCandidate.enabled {
		if match, err := s.semanticCache.Lookup(ctx, promptcache.SemanticLookupInput{
			OrgID:          input.Principal.OrgID,
			RequestedModel: request.Model,
			Embedding:      semanticCandidate.embedding,
			Threshold:      s.semanticThreshold,
		}); err == nil {
			return s.respondFromSemanticCache(ctx, input, request, resolved, match, budgetCheck, quotaCheck, startedAt)
		} else if errors.Is(err, promptcache.ErrSemanticCacheMiss) {
			s.recordSemanticCacheEvent(ctx, input.Principal.OrgID, nil, "semantic_miss", request.Model, semanticCandidate.cacheKeyHash, semanticCandidate.messagesHash, "")
		} else {
			s.recordSemanticCacheEvent(ctx, input.Principal.OrgID, nil, "semantic_read_error", request.Model, semanticCandidate.cacheKeyHash, semanticCandidate.messagesHash, "redis_error")
			s.logger.Warn("semantic cache read failed", zap.Error(err), zap.String("request_id", input.RequestID.String()))
		}
	} else if semanticCandidate.skipReason != "" {
		s.recordSemanticCacheEvent(ctx, input.Principal.OrgID, nil, "semantic_skip", request.Model, semanticCandidate.cacheKeyHash, semanticCandidate.messagesHash, semanticCandidate.skipReason)
	}
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
		metadata := chatMetadata(budgetCheck, quotaCheck, attempts, includeFallbackMetadata, includeFallbackMetadata || hasRetriedTarget(attempts))
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, lastStatus, lastCode, "", &selected.Provider.ID, &selected.Model.ID, metadata, startedAt)
		s.metrics.ObserveGatewayRequest("error", selected.Provider.Name, request.Model, s.clock().Sub(startedAt))
		return ChatResult{}, domain.NewError(lastStatus, lastCode, "provider request failed")
	}
	s.metrics.ObserveProviderRequest(selected.Provider.Name, request.Model, "success", "")

	providerLatencyMS := int32(providerResponse.LatencyMS)
	if providerLatencyMS == 0 {
		providerLatencyMS = int32(s.clock().Sub(providerStarted).Milliseconds())
	}
	metadata := chatMetadata(budgetCheck, quotaCheck, attempts, includeFallbackMetadata, includeFallbackMetadata || hasRetriedTarget(attempts))
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
	if cacheable {
		cached := promptcache.CachedChatResponse{
			ProviderID:    selected.Provider.ID,
			ModelID:       selected.Model.ID,
			ProviderName:  selected.Provider.Name,
			ProviderModel: selected.Model.ProviderModelName,
			Role:          providerResponse.Role,
			Content:       providerResponse.Content,
			FinishReason:  providerResponse.FinishReason,
		}
		if err := s.promptCache.Set(ctx, cacheKey, cached); err != nil {
			s.recordCacheEvent(ctx, cacheEventInput{
				OrgID:          input.Principal.OrgID,
				RequestLogID:   &metered.RequestLog.ID,
				EventType:      "write_error",
				RequestedModel: request.Model,
				CacheKey:       cacheKey,
				Reason:         "redis_error",
			})
			s.logger.Warn("prompt cache write failed", zap.Error(err), zap.String("request_id", input.RequestID.String()))
		} else {
			s.recordCacheEvent(ctx, cacheEventInput{
				OrgID:          input.Principal.OrgID,
				RequestLogID:   &metered.RequestLog.ID,
				EventType:      "store",
				RequestedModel: request.Model,
				CacheKey:       cacheKey,
			})
		}
	}
	if semanticCandidate.enabled {
		match, err := s.semanticCache.Store(ctx, promptcache.SemanticStoreInput{
			OrgID:          input.Principal.OrgID,
			RequestedModel: request.Model,
			MessagesHash:   semanticCandidate.messagesHash,
			Embedding:      semanticCandidate.embedding,
			Response: promptcache.CachedChatResponse{
				ProviderID:    selected.Provider.ID,
				ModelID:       selected.Model.ID,
				ProviderName:  selected.Provider.Name,
				ProviderModel: selected.Model.ProviderModelName,
				Role:          providerResponse.Role,
				Content:       providerResponse.Content,
				FinishReason:  providerResponse.FinishReason,
			},
		})
		if err != nil {
			s.recordSemanticCacheEvent(ctx, input.Principal.OrgID, &metered.RequestLog.ID, "semantic_write_error", request.Model, semanticCandidate.cacheKeyHash, semanticCandidate.messagesHash, "redis_error")
			s.logger.Warn("semantic cache write failed", zap.Error(err), zap.String("request_id", input.RequestID.String()))
		} else {
			s.recordSemanticCacheEvent(ctx, input.Principal.OrgID, &metered.RequestLog.ID, "semantic_store", request.Model, match.CacheKeyHash, match.MessagesHash, "")
		}
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

func (s *ChatService) respondFromSemanticCache(ctx context.Context, input ChatInput, request ChatCompletionRequest, resolved routing.ResolveTargetsResult, match promptcache.SemanticMatch, budgetCheck budget.CheckResult, quotaCheck auth.APIKeyQuotaCheckResult, startedAt time.Time) (ChatResult, error) {
	target := resolved.Targets[0]
	status := metering.StatusSuccess
	if budgetCheck.Warning {
		status = metering.StatusBudgetWarned
	}
	metadata := withSemanticCacheMetadata(chatMetadata(budgetCheck, quotaCheck, nil, false, false), "semantic_hit", match.CacheKeyHash, match.Similarity)
	requestLog, err := s.metering.RecordCacheHit(ctx, metering.RecordCacheHitInput{
		RequestID:     input.RequestID,
		OrgID:         input.Principal.OrgID,
		APIKeyID:      input.Principal.APIKeyID,
		ProviderID:    target.Provider.ID,
		ModelID:       target.Model.ID,
		RoutePolicyID: &resolved.RoutePolicy.ID,
		Method:        input.Method,
		Path:          input.Path,
		RequestModel:  request.Model,
		Status:        status,
		StatusCode:    http.StatusOK,
		RequestHash:   hashBytes(input.RawBody),
		ResponseHash:  hashBytes([]byte(match.Response.Content)),
		LatencyMS:     int32(s.clock().Sub(startedAt).Milliseconds()),
		Metadata:      metadata,
		StartedAt:     startedAt,
		CompletedAt:   s.clock(),
	})
	if err != nil {
		return ChatResult{}, err
	}
	s.recordSemanticCacheEvent(ctx, input.Principal.OrgID, &requestLog.ID, "semantic_hit", request.Model, match.CacheKeyHash, match.MessagesHash, "")
	statusLabel := "success"
	if budgetCheck.Warning {
		statusLabel = metering.StatusBudgetWarned
	}
	s.metrics.ObserveGatewayRequest(statusLabel, target.Provider.Name, request.Model, s.clock().Sub(startedAt))
	return ChatResult{Response: cachedChatResponse(input.RequestID, request.Model, target, match.Response, s.clock())}, nil
}

func (s *ChatService) respondFromPromptCache(ctx context.Context, input ChatInput, request ChatCompletionRequest, resolved routing.ResolveTargetsResult, cached promptcache.CachedChatResponse, cacheKey promptcache.PromptKey, budgetCheck budget.CheckResult, quotaCheck auth.APIKeyQuotaCheckResult, startedAt time.Time) (ChatResult, error) {
	target := resolved.Targets[0]
	status := metering.StatusSuccess
	if budgetCheck.Warning {
		status = metering.StatusBudgetWarned
	}
	metadata := withCacheMetadata(chatMetadata(budgetCheck, quotaCheck, nil, false, false), "hit", cacheKey.CacheKeyHash)
	requestLog, err := s.metering.RecordCacheHit(ctx, metering.RecordCacheHitInput{
		RequestID:     input.RequestID,
		OrgID:         input.Principal.OrgID,
		APIKeyID:      input.Principal.APIKeyID,
		ProviderID:    target.Provider.ID,
		ModelID:       target.Model.ID,
		RoutePolicyID: &resolved.RoutePolicy.ID,
		Method:        input.Method,
		Path:          input.Path,
		RequestModel:  request.Model,
		Status:        status,
		StatusCode:    http.StatusOK,
		RequestHash:   hashBytes(input.RawBody),
		ResponseHash:  hashBytes([]byte(cached.Content)),
		LatencyMS:     int32(s.clock().Sub(startedAt).Milliseconds()),
		Metadata:      metadata,
		StartedAt:     startedAt,
		CompletedAt:   s.clock(),
	})
	if err != nil {
		return ChatResult{}, err
	}
	s.recordCacheEvent(ctx, cacheEventInput{
		OrgID:          input.Principal.OrgID,
		RequestLogID:   &requestLog.ID,
		EventType:      "hit",
		RequestedModel: request.Model,
		CacheKey:       cacheKey,
	})
	statusLabel := "success"
	if budgetCheck.Warning {
		statusLabel = metering.StatusBudgetWarned
	}
	s.metrics.ObserveGatewayRequest(statusLabel, target.Provider.Name, request.Model, s.clock().Sub(startedAt))
	return ChatResult{Response: cachedChatResponse(input.RequestID, request.Model, target, cached, s.clock())}, nil
}

func cachedChatResponse(requestID uuid.UUID, requestedModel string, target routing.ResolvedTarget, cached promptcache.CachedChatResponse, now time.Time) ChatCompletionResponse {
	return ChatCompletionResponse{
		ID:      "chatcmpl_" + requestID.String(),
		Object:  "chat.completion",
		Created: now.Unix(),
		Model:   requestedModel,
		Provider: ProviderBody{
			ID:    target.Provider.ID,
			Name:  target.Provider.Name,
			Model: target.Model.ProviderModelName,
		},
		Choices: []ChoiceBody{
			{
				Index: 0,
				Message: contract.ChatMessage{
					Role:    cached.Role,
					Content: cached.Content,
				},
				FinishReason: cached.FinishReason,
			},
		},
		Usage: UsageBody{
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
		Cost: CostBody{
			Currency:       "USD",
			TotalCostMicro: 0,
		},
		RequestID: requestID,
	}
}

func (s *ChatService) promptCacheKey(request ChatCompletionRequest, orgID uuid.UUID) (promptcache.PromptKey, bool) {
	if s.promptCache == nil || request.Stream {
		return promptcache.PromptKey{}, false
	}
	key, err := promptcache.BuildPromptKey(promptcache.PromptKeyInput{
		OrgID:          orgID,
		RequestedModel: request.Model,
		Messages:       request.Messages,
		Temperature:    request.Temperature,
		MaxTokens:      request.MaxTokens,
	})
	if err != nil {
		return promptcache.PromptKey{}, false
	}
	return key, true
}

func (s *ChatService) semanticCacheCandidate(ctx context.Context, input ChatInput, request ChatCompletionRequest, promptKey promptcache.PromptKey) semanticCacheCandidate {
	if s.semanticCache == nil || s.embedding == nil || request.Stream {
		return semanticCacheCandidate{}
	}
	key := promptKey
	if key.CacheKeyHash == "" {
		built, err := promptcache.BuildPromptKey(promptcache.PromptKeyInput{
			OrgID:          input.Principal.OrgID,
			RequestedModel: request.Model,
			Messages:       request.Messages,
			Temperature:    request.Temperature,
			MaxTokens:      request.MaxTokens,
		})
		if err != nil {
			return semanticCacheCandidate{}
		}
		key = built
	}
	candidate := semanticCacheCandidate{
		cacheKeyHash: key.CacheKeyHash,
		messagesHash: key.MessagesHash,
	}
	if request.Temperature != nil && *request.Temperature > s.semanticMaxTemp {
		candidate.skipReason = "high_temperature"
		return candidate
	}
	if highRiskMessages(request.Messages) {
		candidate.skipReason = "high_risk"
		return candidate
	}
	vector, err := s.embedding.Embed(ctx, messagesText(request.Messages))
	if err != nil {
		candidate.skipReason = "embedding_error"
		return candidate
	}
	candidate.enabled = true
	candidate.embedding = vector
	return candidate
}

func (s *ChatService) recordPromptCacheSkip(ctx context.Context, input ChatInput, request ChatCompletionRequest, reason string) {
	if s.promptCache == nil {
		return
	}
	key, err := promptcache.BuildPromptKey(promptcache.PromptKeyInput{
		OrgID:          input.Principal.OrgID,
		RequestedModel: request.Model,
		Messages:       request.Messages,
		Temperature:    request.Temperature,
		MaxTokens:      request.MaxTokens,
	})
	if err != nil {
		return
	}
	s.recordCacheEvent(ctx, cacheEventInput{
		OrgID:          input.Principal.OrgID,
		EventType:      "skip",
		RequestedModel: request.Model,
		CacheKey:       key,
		Reason:         reason,
	})
}

func (s *ChatService) recordSemanticCacheEvent(ctx context.Context, orgID uuid.UUID, requestLogID *uuid.UUID, eventType string, requestedModel string, cacheKeyHash string, messagesHash string, reason string) {
	if orgID == uuid.Nil || cacheKeyHash == "" || messagesHash == "" || requestedModel == "" {
		return
	}
	_, err := s.store.Queries.InsertCacheEvent(ctx, db.InsertCacheEventParams{
		ID:             uuid.New(),
		OrgID:          orgID,
		RequestLogID:   requestLogID,
		EventType:      eventType,
		RequestedModel: requestedModel,
		CacheKeyHash:   cacheKeyHash,
		MessagesHash:   messagesHash,
		Reason:         pgText(reason),
		CreatedAt:      s.clock(),
	})
	if err != nil {
		s.logger.Warn("record semantic cache event failed", zap.Error(err), zap.String("event_type", eventType))
	}
}

func messagesText(messages []contract.ChatMessage) string {
	var builder strings.Builder
	for _, message := range messages {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(message.Role)
		builder.WriteByte(':')
		builder.WriteString(message.Content)
	}
	return builder.String()
}

func highRiskMessages(messages []contract.ChatMessage) bool {
	text := strings.ToLower(messagesText(messages))
	for _, marker := range []string{"password", "api key", "secret", "credit card", "ssn"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
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

func (s *ChatService) recordCacheEvent(ctx context.Context, input cacheEventInput) {
	if input.OrgID == uuid.Nil || input.CacheKey.CacheKeyHash == "" || input.RequestedModel == "" {
		return
	}
	_, err := s.store.Queries.InsertCacheEvent(ctx, db.InsertCacheEventParams{
		ID:             uuid.New(),
		OrgID:          input.OrgID,
		RequestLogID:   input.RequestLogID,
		EventType:      input.EventType,
		RequestedModel: input.RequestedModel,
		CacheKeyHash:   input.CacheKey.CacheKeyHash,
		MessagesHash:   input.CacheKey.MessagesHash,
		Reason:         pgText(input.Reason),
		CreatedAt:      s.clock(),
	})
	if err != nil {
		s.logger.Warn("record cache event failed", zap.Error(err), zap.String("event_type", input.EventType))
	}
}

func pgText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func estimatePromptTokens(messages []contract.ChatMessage) int64 {
	var totalRunes int
	for _, message := range messages {
		totalRunes += utf8.RuneCountInString(message.Content)
	}
	if totalRunes == 0 && len(messages) > 0 {
		return 1
	}
	tokens := int64((totalRunes + 3) / 4)
	if tokens < 1 && len(messages) > 0 {
		return 1
	}
	return tokens
}

func estimateMaxTokens(maxTokens *int) int64 {
	if maxTokens == nil || *maxTokens <= 0 {
		return 0
	}
	return int64(*maxTokens)
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

func chatMetadata(result budget.CheckResult, quotaResult auth.APIKeyQuotaCheckResult, attempts []providerAttemptMetadata, includeFallback bool, includeAttempts bool) *json.RawMessage {
	body := map[string]any{}
	if budgetBody := budgetMetadataBody(result); budgetBody != nil {
		body["budget"] = budgetBody
	}
	if quotaBody := apiKeyQuotaMetadataBody(quotaResult); quotaBody != nil {
		body["api_key_quota"] = quotaBody
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

func withCacheMetadata(existing *json.RawMessage, status string, cacheKeyHash string) *json.RawMessage {
	body := map[string]any{}
	if existing != nil && len(*existing) > 0 {
		_ = json.Unmarshal(*existing, &body)
	}
	body["cache"] = map[string]any{
		"status":         status,
		"cache_key_hash": cacheKeyHash,
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return existing
	}
	raw := json.RawMessage(rawBody)
	return &raw
}

func withSemanticCacheMetadata(existing *json.RawMessage, status string, cacheKeyHash string, similarity float64) *json.RawMessage {
	body := map[string]any{}
	if existing != nil && len(*existing) > 0 {
		_ = json.Unmarshal(*existing, &body)
	}
	body["cache"] = map[string]any{
		"status":         status,
		"cache_key_hash": cacheKeyHash,
		"similarity":     similarity,
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return existing
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

func apiKeyQuotaMetadata(result auth.APIKeyQuotaCheckResult) *json.RawMessage {
	body := apiKeyQuotaMetadataBody(result)
	if body == nil {
		return nil
	}
	rawBody, err := json.Marshal(map[string]any{"api_key_quota": body})
	if err != nil {
		return nil
	}
	raw := json.RawMessage(rawBody)
	return &raw
}

func apiKeyQuotaMetadataBody(result auth.APIKeyQuotaCheckResult) map[string]any {
	if result.Status == nil {
		return nil
	}
	return map[string]any{
		"id":                   result.Status.APIKey.ID,
		"action":               result.Action,
		"used_micro_usd":       result.Status.UsedMicroUSD,
		"limit_micro_usd":      result.Status.LimitMicroUSD,
		"remaining_micro_usd":  result.Status.RemainingMicroUSD,
		"exceeded":             result.Status.Exceeded,
		"period":               result.Status.Period,
		"window_start":         result.Status.WindowStart,
		"window_end":           result.Status.WindowEnd,
		"pre_check_only":       true,
		"projected_cost_known": false,
	}
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
