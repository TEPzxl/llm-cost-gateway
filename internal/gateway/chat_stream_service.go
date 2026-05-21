package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/budget"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/metering"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
	"github.com/tep/llm-cost-gateway/internal/routing"
)

type ChatStreamResult struct {
	Stream contract.ChatStream

	input             ChatInput
	request           ChatCompletionRequest
	resolved          routing.ResolveResult
	budgetCheck       budget.CheckResult
	quotaCheck        auth.APIKeyQuotaCheckResult
	startedAt         time.Time
	providerStartedAt time.Time
}

func (s *ChatService) StreamChat(ctx context.Context, input ChatInput) (*ChatStreamResult, error) {
	startedAt := s.clock()
	var request ChatCompletionRequest
	if err := json.Unmarshal(input.RawBody, &request); err != nil {
		return nil, domain.NewError(http.StatusBadRequest, domain.CodeInvalidRequest, "invalid JSON body")
	}
	if request.Model == "" || len(request.Messages) == 0 {
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, http.StatusBadRequest, domain.CodeInvalidRequest, "", nil, nil, nil, startedAt)
		s.metrics.ObserveGatewayRequest("error", "", request.Model, s.clock().Sub(startedAt))
		return nil, domain.NewError(http.StatusBadRequest, domain.CodeInvalidRequest, "model and messages are required")
	}
	s.recordPromptCacheSkip(ctx, input, request, "stream")

	budgetCheck, err := s.budgets.Check(ctx, input.Principal.OrgID)
	if err != nil {
		return nil, err
	}
	if !budgetCheck.Allowed {
		metadata := budgetMetadata(budgetCheck)
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusBudgetBlocked, http.StatusPaymentRequired, domain.CodeBudgetExceeded, "", nil, nil, metadata, startedAt)
		s.metrics.IncBudgetBlocked()
		s.metrics.ObserveGatewayRequest("budget_blocked", "", request.Model, s.clock().Sub(startedAt))
		return nil, domain.NewError(http.StatusPaymentRequired, domain.CodeBudgetExceeded, "budget exceeded")
	}
	quotaCheck, err := s.apiKeyQuotas.Check(ctx, input.Principal)
	if err != nil {
		return nil, err
	}
	if !quotaCheck.Allowed {
		metadata := apiKeyQuotaMetadata(quotaCheck)
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusBudgetBlocked, http.StatusPaymentRequired, domain.CodeAPIKeyQuotaExceeded, "", nil, nil, metadata, startedAt)
		s.metrics.ObserveGatewayRequest("api_key_quota_blocked", "", request.Model, s.clock().Sub(startedAt))
		return nil, domain.NewError(http.StatusPaymentRequired, domain.CodeAPIKeyQuotaExceeded, "api key quota exceeded")
	}

	resolved, err := s.resolver.Resolve(ctx, routing.ResolveParams{
		OrgID:                 input.Principal.OrgID,
		RequestedModel:        request.Model,
		EstimatedPromptTokens: estimatePromptTokens(request.Messages),
		EstimatedMaxTokens:    estimateMaxTokens(request.MaxTokens),
	})
	if err != nil {
		if errors.Is(err, routing.ErrRouteNotFound) {
			_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, http.StatusNotFound, domain.CodeRouteNotFound, "", nil, nil, nil, startedAt)
			s.metrics.ObserveGatewayRequest("error", "", request.Model, s.clock().Sub(startedAt))
			return nil, domain.NewError(http.StatusNotFound, domain.CodeRouteNotFound, "route not found")
		}
		return nil, err
	}

	adapter, err := s.registry.AdapterFor(resolved.Provider.Type)
	if err != nil {
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, http.StatusServiceUnavailable, domain.CodeProviderUnavailable, "", &resolved.Provider.ID, &resolved.Model.ID, streamMetadata(budgetCheck, quotaCheck, false), startedAt)
		s.metrics.ObserveGatewayRequest("error", resolved.Provider.Name, request.Model, s.clock().Sub(startedAt))
		return nil, domain.NewError(http.StatusServiceUnavailable, domain.CodeProviderUnavailable, "provider unavailable")
	}
	providerConfig, err := s.providerConfig(ctx, resolved.Provider)
	if err != nil {
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, http.StatusServiceUnavailable, domain.CodeProviderUnavailable, "", &resolved.Provider.ID, &resolved.Model.ID, streamMetadata(budgetCheck, quotaCheck, false), startedAt)
		s.metrics.ObserveGatewayRequest("error", resolved.Provider.Name, request.Model, s.clock().Sub(startedAt))
		return nil, domain.NewError(http.StatusServiceUnavailable, domain.CodeProviderUnavailable, "provider unavailable")
	}

	providerStartedAt := s.clock()
	stream, err := adapter.StreamChat(ctx, contract.ChatRequest{
		RequestID:   input.RequestID.String(),
		OrgID:       input.Principal.OrgID,
		Provider:    providerConfig,
		Model:       contract.ModelConfig{ID: resolved.Model.ID, ProviderModelName: resolved.Model.ProviderModelName, DisplayName: resolved.Model.DisplayName},
		Messages:    request.Messages,
		Temperature: request.Temperature,
		MaxTokens:   request.MaxTokens,
		Stream:      true,
	})
	if err != nil {
		status, code := providerErrorStatus(err)
		_ = s.recordFailure(ctx, input, request.Model, metering.StatusError, status, code, "", &resolved.Provider.ID, &resolved.Model.ID, streamMetadata(budgetCheck, quotaCheck, false), startedAt)
		s.metrics.ObserveProviderRequest(resolved.Provider.Name, request.Model, "error", code)
		s.metrics.ObserveGatewayRequest("error", resolved.Provider.Name, request.Model, s.clock().Sub(startedAt))
		return nil, domain.NewError(status, code, "provider request failed")
	}

	return &ChatStreamResult{
		Stream:            stream,
		input:             input,
		request:           request,
		resolved:          resolved,
		budgetCheck:       budgetCheck,
		quotaCheck:        quotaCheck,
		startedAt:         startedAt,
		providerStartedAt: providerStartedAt,
	}, nil
}

func (s *ChatService) FinalizeStream(ctx context.Context, result *ChatStreamResult, usage *contract.StreamUsage, streamErr error) error {
	providerLatencyMS := int32(s.clock().Sub(result.providerStartedAt).Milliseconds())
	completedAt := s.clock()
	metadata := streamMetadata(result.budgetCheck, result.quotaCheck, usage == nil)
	if streamErr != nil {
		status, code := providerErrorStatus(streamErr)
		err := s.recordFailure(ctx, result.input, result.request.Model, metering.StatusError, status, code, "", &result.resolved.Provider.ID, &result.resolved.Model.ID, metadata, result.startedAt)
		s.metrics.ObserveProviderRequest(result.resolved.Provider.Name, result.request.Model, "error", code)
		s.metrics.ObserveGatewayRequest("error", result.resolved.Provider.Name, result.request.Model, s.clock().Sub(result.startedAt))
		return err
	}
	if usage == nil {
		_, err := s.metering.RecordFailure(ctx, metering.RecordFailureInput{
			RequestID:         result.input.RequestID,
			OrgID:             result.input.Principal.OrgID,
			APIKeyID:          &result.input.Principal.APIKeyID,
			ProviderID:        &result.resolved.Provider.ID,
			ModelID:           &result.resolved.Model.ID,
			RoutePolicyID:     &result.resolved.RoutePolicy.ID,
			Method:            result.input.Method,
			Path:              result.input.Path,
			RequestModel:      result.request.Model,
			Status:            metering.StatusError,
			StatusCode:        http.StatusOK,
			ErrorCode:         domain.CodeUsageMissing,
			RequestHash:       hashBytes(result.input.RawBody),
			LatencyMS:         int32(completedAt.Sub(result.startedAt).Milliseconds()),
			ProviderLatencyMS: &providerLatencyMS,
			Metadata:          metadata,
			StartedAt:         result.startedAt,
			CompletedAt:       completedAt,
		})
		s.metrics.ObserveGatewayRequest("error", result.resolved.Provider.Name, result.request.Model, completedAt.Sub(result.startedAt))
		return err
	}

	successStatus := metering.StatusSuccess
	if result.budgetCheck.Warning {
		successStatus = metering.StatusBudgetWarned
	}
	rawUsage, _ := json.Marshal(usage.Raw)
	metered, err := s.metering.RecordSuccess(ctx, metering.RecordSuccessInput{
		RequestID:                      result.input.RequestID,
		OrgID:                          result.input.Principal.OrgID,
		APIKeyID:                       result.input.Principal.APIKeyID,
		ProviderID:                     result.resolved.Provider.ID,
		ModelID:                        result.resolved.Model.ID,
		RoutePolicyID:                  &result.resolved.RoutePolicy.ID,
		Method:                         result.input.Method,
		Path:                           result.input.Path,
		RequestModel:                   result.request.Model,
		Status:                         successStatus,
		StatusCode:                     http.StatusOK,
		RequestHash:                    hashBytes(result.input.RawBody),
		LatencyMS:                      int32(completedAt.Sub(result.startedAt).Milliseconds()),
		ProviderLatencyMS:              &providerLatencyMS,
		Metadata:                       metadata,
		ProviderUsageJSON:              rawUsage,
		PromptTokens:                   int64(usage.PromptTokens),
		CompletionTokens:               int64(usage.CompletionTokens),
		InputPriceMicroUSDPer1KTokens:  result.resolved.Model.InputPriceMicroUsdPer1kTokens,
		OutputPriceMicroUSDPer1KTokens: result.resolved.Model.OutputPriceMicroUsdPer1kTokens,
		StartedAt:                      result.startedAt,
		CompletedAt:                    completedAt,
	})
	if err != nil {
		return err
	}
	statusLabel := "success"
	if result.budgetCheck.Warning {
		statusLabel = metering.StatusBudgetWarned
	}
	s.metrics.ObserveProviderRequest(result.resolved.Provider.Name, result.request.Model, "success", "")
	s.metrics.ObserveGatewayRequest(statusLabel, result.resolved.Provider.Name, result.request.Model, completedAt.Sub(result.startedAt))
	s.metrics.AddTokens(result.request.Model, int64(usage.PromptTokens), int64(usage.CompletionTokens))
	s.metrics.AddCostMicroUSD(result.request.Model, metered.CostRecord.TotalCostMicro)
	return nil
}

func streamMetadata(result budget.CheckResult, quotaResult auth.APIKeyQuotaCheckResult, usageMissing bool) *json.RawMessage {
	body := map[string]any{
		"stream": true,
	}
	if usageMissing {
		body["usage_missing"] = true
	}
	if budgetBody := budgetMetadataBody(result); budgetBody != nil {
		body["budget"] = budgetBody
	}
	if quotaBody := apiKeyQuotaMetadataBody(quotaResult); quotaBody != nil {
		body["api_key_quota"] = quotaBody
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	raw := json.RawMessage(rawBody)
	return &raw
}
