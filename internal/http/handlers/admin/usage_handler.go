package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	GroupByProvider = "provider"
	GroupByModel    = "model"
	GroupByAPIKey   = "api_key"
)

type UsageHandler struct {
	queries *db.Queries
	logs    *RequestLogHandler
}

func NewUsageHandler(queries *db.Queries) *UsageHandler {
	return &UsageHandler{
		queries: queries,
		logs:    NewRequestLogHandler(queries),
	}
}

type usageSummaryResponse struct {
	GroupBy string             `json:"group_by"`
	Items   []usageSummaryItem `json:"items"`
}

type usageSummaryItem struct {
	ProviderID           *uuid.UUID `json:"provider_id,omitempty"`
	ProviderName         *string    `json:"provider_name,omitempty"`
	ModelID              *uuid.UUID `json:"model_id,omitempty"`
	ModelName            *string    `json:"model_name,omitempty"`
	APIKeyID             *uuid.UUID `json:"api_key_id,omitempty"`
	APIKeyName           *string    `json:"api_key_name,omitempty"`
	RequestCount         int64      `json:"request_count"`
	SuccessCount         int64      `json:"success_count"`
	ErrorCount           int64      `json:"error_count"`
	PromptTokens         int64      `json:"prompt_tokens"`
	CompletionTokens     int64      `json:"completion_tokens"`
	TotalTokens          int64      `json:"total_tokens"`
	TotalCostMicroUSD    int64      `json:"total_cost_micro_usd"`
	AverageLatencyMillis float64    `json:"avg_latency_ms"`
}

func (h *UsageHandler) Summary(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}
	groupBy := c.DefaultQuery("group_by", GroupByModel)
	if groupBy != GroupByProvider && groupBy != GroupByModel && groupBy != GroupByAPIKey {
		httpapi.RespondError(c, httpapi.InvalidRequest("group_by must be provider, model or api_key"))
		return
	}
	window, ok := h.logs.parseWindow(c)
	if !ok {
		return
	}

	var (
		items []usageSummaryItem
		err   error
	)
	switch groupBy {
	case GroupByProvider:
		items, err = h.summaryByProvider(c, principal.OrgID, window)
	case GroupByModel:
		items, err = h.summaryByModel(c, principal.OrgID, window)
	case GroupByAPIKey:
		items, err = h.summaryByAPIKey(c, principal.OrgID, window)
	}
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	httpapi.RespondJSON(c, http.StatusOK, usageSummaryResponse{GroupBy: groupBy, Items: items})
}

func (h *UsageHandler) summaryByProvider(c *gin.Context, orgID uuid.UUID, window timeWindow) ([]usageSummaryItem, error) {
	rows, err := h.queries.UsageSummaryByProvider(c.Request.Context(), db.UsageSummaryByProviderParams{
		OrgID:       orgID,
		CreatedAt:   window.from,
		CreatedAt_2: window.to,
	})
	if err != nil {
		return nil, err
	}
	items := make([]usageSummaryItem, 0, len(rows))
	for _, row := range rows {
		providerID := row.ProviderID
		item := usageSummaryItem{
			ProviderID:           &providerID,
			RequestCount:         row.RequestCount,
			SuccessCount:         row.SuccessCount,
			ErrorCount:           row.ErrorCount,
			PromptTokens:         row.PromptTokens,
			CompletionTokens:     row.CompletionTokens,
			TotalTokens:          row.TotalTokens,
			TotalCostMicroUSD:    row.TotalCostMicroUsd,
			AverageLatencyMillis: row.AvgLatencyMs,
		}
		if provider, err := h.queries.GetProvider(c.Request.Context(), db.GetProviderParams{OrgID: orgID, ID: row.ProviderID}); err == nil {
			item.ProviderName = &provider.Name
		}
		items = append(items, item)
	}
	return items, nil
}

func (h *UsageHandler) summaryByModel(c *gin.Context, orgID uuid.UUID, window timeWindow) ([]usageSummaryItem, error) {
	rows, err := h.queries.UsageSummaryByModel(c.Request.Context(), db.UsageSummaryByModelParams{
		OrgID:       orgID,
		CreatedAt:   window.from,
		CreatedAt_2: window.to,
	})
	if err != nil {
		return nil, err
	}
	items := make([]usageSummaryItem, 0, len(rows))
	for _, row := range rows {
		modelID := row.ModelID
		item := usageSummaryItem{
			ModelID:              &modelID,
			RequestCount:         row.RequestCount,
			SuccessCount:         row.SuccessCount,
			ErrorCount:           row.ErrorCount,
			PromptTokens:         row.PromptTokens,
			CompletionTokens:     row.CompletionTokens,
			TotalTokens:          row.TotalTokens,
			TotalCostMicroUSD:    row.TotalCostMicroUsd,
			AverageLatencyMillis: row.AvgLatencyMs,
		}
		if model, err := h.queries.GetModel(c.Request.Context(), db.GetModelParams{OrgID: orgID, ID: row.ModelID}); err == nil {
			item.ModelName = &model.DisplayName
		}
		items = append(items, item)
	}
	return items, nil
}

func (h *UsageHandler) summaryByAPIKey(c *gin.Context, orgID uuid.UUID, window timeWindow) ([]usageSummaryItem, error) {
	rows, err := h.queries.UsageSummaryByAPIKey(c.Request.Context(), db.UsageSummaryByAPIKeyParams{
		OrgID:       orgID,
		CreatedAt:   window.from,
		CreatedAt_2: window.to,
	})
	if err != nil {
		return nil, err
	}
	items := make([]usageSummaryItem, 0, len(rows))
	for _, row := range rows {
		apiKeyID := row.ApiKeyID
		item := usageSummaryItem{
			APIKeyID:             &apiKeyID,
			RequestCount:         row.RequestCount,
			SuccessCount:         row.SuccessCount,
			ErrorCount:           row.ErrorCount,
			PromptTokens:         row.PromptTokens,
			CompletionTokens:     row.CompletionTokens,
			TotalTokens:          row.TotalTokens,
			TotalCostMicroUSD:    row.TotalCostMicroUsd,
			AverageLatencyMillis: row.AvgLatencyMs,
		}
		if apiKey, err := h.queries.GetAPIKey(c.Request.Context(), db.GetAPIKeyParams{OrgID: orgID, ID: row.ApiKeyID}); err == nil {
			item.APIKeyName = &apiKey.Name
		}
		items = append(items, item)
	}
	return items, nil
}
