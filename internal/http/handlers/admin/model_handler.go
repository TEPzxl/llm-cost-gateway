package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	"github.com/tep/llm-cost-gateway/internal/provider"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type ModelHandler struct {
	service *provider.Service
}

func NewModelHandler(service *provider.Service) *ModelHandler {
	return &ModelHandler{service: service}
}

type createModelRequest struct {
	ProviderID                     uuid.UUID `json:"provider_id"`
	ProviderModelName              string    `json:"provider_model_name"`
	DisplayName                    string    `json:"display_name"`
	InputPriceMicroUSDPer1KTokens  int64     `json:"input_price_micro_usd_per_1k_tokens"`
	OutputPriceMicroUSDPer1KTokens int64     `json:"output_price_micro_usd_per_1k_tokens"`
	ContextWindow                  *int32    `json:"context_window"`
}

type modelResponse struct {
	ID                             uuid.UUID `json:"id"`
	ProviderID                     uuid.UUID `json:"provider_id"`
	ProviderModelName              string    `json:"provider_model_name"`
	DisplayName                    string    `json:"display_name"`
	InputPriceMicroUSDPer1KTokens  int64     `json:"input_price_micro_usd_per_1k_tokens"`
	OutputPriceMicroUSDPer1KTokens int64     `json:"output_price_micro_usd_per_1k_tokens"`
	ContextWindow                  *int32    `json:"context_window"`
	Status                         string    `json:"status"`
	CreatedAt                      time.Time `json:"created_at"`
}

type listModelsResponse struct {
	Items []modelResponse `json:"items"`
}

func (h *ModelHandler) Create(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	var request createModelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}

	created, err := h.service.CreateModel(c.Request.Context(), provider.CreateModelParams{
		OrgID:                          principal.OrgID,
		ProviderID:                     request.ProviderID,
		ProviderModelName:              request.ProviderModelName,
		DisplayName:                    request.DisplayName,
		InputPriceMicroUSDPer1KTokens:  request.InputPriceMicroUSDPer1KTokens,
		OutputPriceMicroUSDPer1KTokens: request.OutputPriceMicroUSDPer1KTokens,
		ContextWindow:                  request.ContextWindow,
	})
	if err != nil {
		respondProviderServiceError(c, err)
		return
	}

	httpapi.RespondJSON(c, http.StatusCreated, newModelResponse(created))
}

func (h *ModelHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	models, err := h.service.ListModels(c.Request.Context(), principal.OrgID)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	items := make([]modelResponse, 0, len(models))
	for _, item := range models {
		items = append(items, newModelResponse(item))
	}
	httpapi.RespondJSON(c, http.StatusOK, listModelsResponse{Items: items})
}

func newModelResponse(item db.Model) modelResponse {
	var contextWindow *int32
	if item.ContextWindow.Valid {
		contextWindow = &item.ContextWindow.Int32
	}

	return modelResponse{
		ID:                             item.ID,
		ProviderID:                     item.ProviderID,
		ProviderModelName:              item.ProviderModelName,
		DisplayName:                    item.DisplayName,
		InputPriceMicroUSDPer1KTokens:  item.InputPriceMicroUsdPer1kTokens,
		OutputPriceMicroUSDPer1KTokens: item.OutputPriceMicroUsdPer1kTokens,
		ContextWindow:                  contextWindow,
		Status:                         item.Status,
		CreatedAt:                      item.CreatedAt,
	}
}
