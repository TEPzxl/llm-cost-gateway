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
	metering            *metering.Service
	metrics             *observability.Metrics
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

func NewChatService(st *store.Store, secretEncryptionKey string, metrics *observability.Metrics) *ChatService {
	return &ChatService{
		store:               st,
		resolver:            routing.NewResolver(st.Queries),
		registry:            provider.NewRegistry(),
		budgets:             budget.NewService(st.Queries),
		metering:            metering.NewService(st, costing.NewCalculator()),
		metrics:             metrics,
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

	resolved, err := s.resolver.Resolve(ctx, routing.ResolveParams{
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

	adapter, err := s.registry.AdapterFor(resolved.Provider.Type)
	if err != nil {
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, http.StatusServiceUnavailable, domain.CodeProviderUnavailable, "", &resolved.Provider.ID, &resolved.Model.ID, nil, startedAt)
		s.metrics.ObserveGatewayRequest("error", resolved.Provider.Name, request.Model, s.clock().Sub(startedAt))
		return ChatResult{}, domain.NewError(http.StatusServiceUnavailable, domain.CodeProviderUnavailable, "provider unavailable")
	}
	providerConfig, err := s.providerConfig(ctx, resolved.Provider)
	if err != nil {
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, http.StatusServiceUnavailable, domain.CodeProviderUnavailable, "", &resolved.Provider.ID, &resolved.Model.ID, nil, startedAt)
		s.metrics.ObserveGatewayRequest("error", resolved.Provider.Name, request.Model, s.clock().Sub(startedAt))
		return ChatResult{}, domain.NewError(http.StatusServiceUnavailable, domain.CodeProviderUnavailable, "provider unavailable")
	}

	providerStarted := s.clock()
	providerResponse, err := adapter.Chat(ctx, contract.ChatRequest{
		RequestID:   input.RequestID.String(),
		OrgID:       input.Principal.OrgID,
		Provider:    providerConfig,
		Model:       contract.ModelConfig{ID: resolved.Model.ID, ProviderModelName: resolved.Model.ProviderModelName, DisplayName: resolved.Model.DisplayName},
		Messages:    request.Messages,
		Temperature: request.Temperature,
		MaxTokens:   request.MaxTokens,
		Stream:      false,
	})
	if err != nil {
		status, code := providerErrorStatus(err)
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, status, code, "", &resolved.Provider.ID, &resolved.Model.ID, nil, startedAt)
		s.metrics.ObserveProviderRequest(resolved.Provider.Name, request.Model, "error", code)
		s.metrics.ObserveGatewayRequest("error", resolved.Provider.Name, request.Model, s.clock().Sub(startedAt))
		return ChatResult{}, domain.NewError(status, code, "provider request failed")
	}
	s.metrics.ObserveProviderRequest(resolved.Provider.Name, request.Model, "success", "")

	providerLatencyMS := int32(providerResponse.LatencyMS)
	if providerLatencyMS == 0 {
		providerLatencyMS = int32(s.clock().Sub(providerStarted).Milliseconds())
	}
	metadata := budgetMetadata(budgetCheck)
	successStatus := metering.StatusSuccess
	if budgetCheck.Warning {
		successStatus = metering.StatusBudgetWarned
	}
	rawUsage, _ := json.Marshal(providerResponse.RawUsage)
	metered, err := s.metering.RecordSuccess(ctx, metering.RecordSuccessInput{
		RequestID:                      input.RequestID,
		OrgID:                          input.Principal.OrgID,
		APIKeyID:                       input.Principal.APIKeyID,
		ProviderID:                     resolved.Provider.ID,
		ModelID:                        resolved.Model.ID,
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
		InputPriceMicroUSDPer1KTokens:  resolved.Model.InputPriceMicroUsdPer1kTokens,
		OutputPriceMicroUSDPer1KTokens: resolved.Model.OutputPriceMicroUsdPer1kTokens,
		StartedAt:                      startedAt,
		CompletedAt:                    s.clock(),
	})
	if err != nil {
		return ChatResult{}, err
	}
	statusLabel := "success"
	if budgetCheck.Warning {
		statusLabel = metering.StatusBudgetWarned
	}
	s.metrics.ObserveGatewayRequest(statusLabel, resolved.Provider.Name, request.Model, s.clock().Sub(startedAt))
	s.metrics.AddTokens(request.Model, int64(providerResponse.Usage.PromptTokens), int64(providerResponse.Usage.CompletionTokens))
	s.metrics.AddCostMicroUSD(request.Model, metered.CostRecord.TotalCostMicro)

	response := ChatCompletionResponse{
		ID:      "chatcmpl_" + input.RequestID.String(),
		Object:  "chat.completion",
		Created: s.clock().Unix(),
		Model:   request.Model,
		Provider: ProviderBody{
			ID:    resolved.Provider.ID,
			Name:  resolved.Provider.Name,
			Model: resolved.Model.ProviderModelName,
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

func budgetMetadata(result budget.CheckResult) *json.RawMessage {
	if result.Status == nil {
		return nil
	}
	body, err := json.Marshal(map[string]any{
		"budget": map[string]any{
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
		},
	})
	if err != nil {
		return nil
	}
	raw := json.RawMessage(body)
	return &raw
}

func hashBytes(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
