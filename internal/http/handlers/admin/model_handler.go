package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/costing"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	"github.com/tep/llm-cost-gateway/internal/provider"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type ModelHandler struct {
	service *provider.Service
	pricing *costing.PricingService
}

func NewModelHandler(service *provider.Service, pricing *costing.PricingService) *ModelHandler {
	return &ModelHandler{service: service, pricing: pricing}
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

type updateModelPricingRequest struct {
	InputPriceMicroUSDPer1KTokens  int64 `json:"input_price_micro_usd_per_1k_tokens"`
	OutputPriceMicroUSDPer1KTokens int64 `json:"output_price_micro_usd_per_1k_tokens"`
}

type modelPricingVersionResponse struct {
	ID                             uuid.UUID  `json:"id"`
	ModelID                        uuid.UUID  `json:"model_id"`
	Version                        int32      `json:"version"`
	InputPriceMicroUSDPer1KTokens  int64      `json:"input_price_micro_usd_per_1k_tokens"`
	OutputPriceMicroUSDPer1KTokens int64      `json:"output_price_micro_usd_per_1k_tokens"`
	Status                         string     `json:"status"`
	EffectiveFrom                  time.Time  `json:"effective_from"`
	EffectiveTo                    *time.Time `json:"effective_to"`
	CreatedAt                      time.Time  `json:"created_at"`
}

type updateModelPricingResponse struct {
	Model           modelResponse               `json:"model"`
	PreviousVersion modelPricingVersionResponse `json:"previous_version"`
	PricingVersion  modelPricingVersionResponse `json:"pricing_version"`
}

type listModelPricingVersionsResponse struct {
	Items []modelPricingVersionResponse `json:"items"`
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

func (h *ModelHandler) UpdatePricing(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}
	modelID, err := uuid.Parse(c.Param("model_id"))
	if err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid model_id"))
		return
	}

	var request updateModelPricingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}
	result, err := h.pricing.UpdateModelPricing(c.Request.Context(), costing.UpdateModelPricingParams{
		OrgID:                          principal.OrgID,
		ModelID:                        modelID,
		InputPriceMicroUSDPer1KTokens:  request.InputPriceMicroUSDPer1KTokens,
		OutputPriceMicroUSDPer1KTokens: request.OutputPriceMicroUSDPer1KTokens,
	})
	if err != nil {
		respondPricingServiceError(c, err)
		return
	}

	httpapi.RespondJSON(c, http.StatusOK, updateModelPricingResponse{
		Model:           newModelResponse(result.Model),
		PreviousVersion: newModelPricingVersionResponse(result.PreviousVersion),
		PricingVersion:  newModelPricingVersionResponse(result.PricingVersion),
	})
}

func (h *ModelHandler) ListPricingVersions(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}
	modelID, err := uuid.Parse(c.Param("model_id"))
	if err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid model_id"))
		return
	}

	versions, err := h.pricing.ListModelPricingVersions(c.Request.Context(), principal.OrgID, modelID)
	if err != nil {
		respondPricingServiceError(c, err)
		return
	}

	items := make([]modelPricingVersionResponse, 0, len(versions))
	for _, version := range versions {
		items = append(items, newModelPricingVersionResponse(version))
	}
	httpapi.RespondJSON(c, http.StatusOK, listModelPricingVersionsResponse{Items: items})
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

func newModelPricingVersionResponse(item db.ModelPricingVersion) modelPricingVersionResponse {
	return modelPricingVersionResponse{
		ID:                             item.ID,
		ModelID:                        item.ModelID,
		Version:                        item.Version,
		InputPriceMicroUSDPer1KTokens:  item.InputPriceMicroUsdPer1kTokens,
		OutputPriceMicroUSDPer1KTokens: item.OutputPriceMicroUsdPer1kTokens,
		Status:                         item.Status,
		EffectiveFrom:                  item.EffectiveFrom,
		EffectiveTo:                    item.EffectiveTo,
		CreatedAt:                      item.CreatedAt,
	}
}

func respondPricingServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, costing.ErrModelPricingNotFound):
		httpapi.RespondError(c, httpapi.NotFound("model not found"))
	case errors.Is(err, costing.ErrInvalidCostInput):
		httpapi.RespondError(c, httpapi.InvalidRequest(err.Error()))
	default:
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
	}
}
