package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/analytics"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type AnalyticsHandler struct {
	service *analytics.UsageAnalyticsService
	logs    *RequestLogHandler
}

func NewAnalyticsHandler(service *analytics.UsageAnalyticsService, queries *db.Queries) *AnalyticsHandler {
	return &AnalyticsHandler{
		service: service,
		logs:    NewRequestLogHandler(queries),
	}
}

type analyticsItemsResponse[T any] struct {
	Items []T `json:"items"`
}

func (h *AnalyticsHandler) DailyCostTrend(c *gin.Context) {
	orgID, window, ok := h.analyticsRequest(c)
	if !ok {
		return
	}
	items, err := h.service.DailyCostTrend(c.Request.Context(), orgID, window)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	httpapi.RespondJSON(c, http.StatusOK, analyticsItemsResponse[analytics.DailyCostPoint]{Items: items})
}

func (h *AnalyticsHandler) ModelCostBreakdown(c *gin.Context) {
	orgID, window, ok := h.analyticsRequest(c)
	if !ok {
		return
	}
	items, err := h.service.ModelCostBreakdown(c.Request.Context(), orgID, window)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	httpapi.RespondJSON(c, http.StatusOK, analyticsItemsResponse[analytics.ModelCostBreakdownItem]{Items: items})
}

func (h *AnalyticsHandler) ProviderLatency(c *gin.Context) {
	orgID, window, ok := h.analyticsRequest(c)
	if !ok {
		return
	}
	items, err := h.service.ProviderLatency(c.Request.Context(), orgID, window)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	httpapi.RespondJSON(c, http.StatusOK, analyticsItemsResponse[analytics.ProviderLatencyItem]{Items: items})
}

func (h *AnalyticsHandler) ErrorRateTrend(c *gin.Context) {
	orgID, window, ok := h.analyticsRequest(c)
	if !ok {
		return
	}
	items, err := h.service.ErrorRateTrend(c.Request.Context(), orgID, window)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	httpapi.RespondJSON(c, http.StatusOK, analyticsItemsResponse[analytics.ErrorRatePoint]{Items: items})
}

func (h *AnalyticsHandler) analyticsRequest(c *gin.Context) (uuid.UUID, analytics.TimeWindow, bool) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return uuid.Nil, analytics.TimeWindow{}, false
	}
	window, ok := h.logs.parseWindow(c)
	if !ok {
		return uuid.Nil, analytics.TimeWindow{}, false
	}
	return principal.OrgID, analytics.TimeWindow{From: window.from, To: window.to}, true
}
