package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	"github.com/tep/llm-cost-gateway/internal/routing"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type RoutePolicyHandler struct {
	service *routing.Service
}

func NewRoutePolicyHandler(service *routing.Service) *RoutePolicyHandler {
	return &RoutePolicyHandler{service: service}
}

type createRoutePolicyRequest struct {
	Name       string                     `json:"name"`
	MatchModel string                     `json:"match_model"`
	Strategy   string                     `json:"strategy"`
	Config     json.RawMessage            `json:"config,omitempty"`
	Targets    []createRouteTargetRequest `json:"targets"`
}

type createRouteTargetRequest struct {
	ProviderID uuid.UUID `json:"provider_id"`
	ModelID    uuid.UUID `json:"model_id"`
	Priority   int32     `json:"priority"`
	Weight     int32     `json:"weight"`
}

type routePolicyResponse struct {
	ID         uuid.UUID       `json:"id"`
	Name       string          `json:"name"`
	MatchModel string          `json:"match_model"`
	Strategy   string          `json:"strategy"`
	Config     json.RawMessage `json:"config"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
}

type listRoutePoliciesResponse struct {
	Items []routePolicyResponse `json:"items"`
}

func (h *RoutePolicyHandler) Create(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	var request createRoutePolicyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}

	targets := make([]routing.TargetParams, 0, len(request.Targets))
	for _, target := range request.Targets {
		targets = append(targets, routing.TargetParams{
			ProviderID: target.ProviderID,
			ModelID:    target.ModelID,
			Priority:   target.Priority,
			Weight:     target.Weight,
		})
	}

	created, err := h.service.CreateRoutePolicy(c.Request.Context(), routing.CreateRoutePolicyParams{
		OrgID:      principal.OrgID,
		Name:       request.Name,
		MatchModel: request.MatchModel,
		Strategy:   request.Strategy,
		Config:     request.Config,
		Targets:    targets,
	})
	if err != nil {
		respondRoutingServiceError(c, err)
		return
	}

	httpapi.RespondJSON(c, http.StatusCreated, newRoutePolicyResponse(created))
}

func (h *RoutePolicyHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	policies, err := h.service.ListRoutePolicies(c.Request.Context(), principal.OrgID)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	items := make([]routePolicyResponse, 0, len(policies))
	for _, policy := range policies {
		items = append(items, newRoutePolicyResponse(policy))
	}
	httpapi.RespondJSON(c, http.StatusOK, listRoutePoliciesResponse{Items: items})
}

func newRoutePolicyResponse(policy db.RoutePolicy) routePolicyResponse {
	return routePolicyResponse{
		ID:         policy.ID,
		Name:       policy.Name,
		MatchModel: policy.MatchModel,
		Strategy:   policy.Strategy,
		Config:     policy.Config,
		Status:     policy.Status,
		CreatedAt:  policy.CreatedAt,
	}
}

func respondRoutingServiceError(c *gin.Context, err error) {
	if validation, ok := routing.IsValidationError(err); ok {
		httpapi.RespondError(c, httpapi.InvalidRequest(validation.Message))
		return
	}
	if errors.Is(err, routing.ErrRouteTargetNotFound) {
		httpapi.RespondError(c, httpapi.NotFound("route target not found"))
		return
	}
	if routing.IsUniqueViolation(err) {
		httpapi.RespondError(c, httpapi.Conflict("resource already exists"))
		return
	}
	httpapi.RespondError(c, httpapi.InternalError("internal server error"))
}
