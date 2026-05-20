package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	"github.com/tep/llm-cost-gateway/internal/provider"
)

type ProviderHealthHandler struct {
	service *provider.HealthService
}

func NewProviderHealthHandler(service *provider.HealthService) *ProviderHealthHandler {
	return &ProviderHealthHandler{service: service}
}

func (h *ProviderHealthHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	providers, err := h.service.List(c.Request.Context(), principal.OrgID)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	items := make([]providerResponse, 0, len(providers))
	for _, item := range providers {
		items = append(items, newProviderResponse(item))
	}
	httpapi.RespondJSON(c, http.StatusOK, listProvidersResponse{Items: items})
}

func (h *ProviderHealthHandler) Check(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}
	providerID, err := uuid.Parse(c.Param("provider_id"))
	if err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid provider_id"))
		return
	}

	checked, err := h.service.Check(c.Request.Context(), principal.OrgID, providerID)
	if err != nil {
		respondProviderServiceError(c, err)
		return
	}
	httpapi.RespondJSON(c, http.StatusOK, newProviderResponse(checked))
}
