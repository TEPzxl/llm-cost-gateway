package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/anomaly"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type AnomalyPolicyHandler struct {
	service *anomaly.Service
}

func NewAnomalyPolicyHandler(service *anomaly.Service) *AnomalyPolicyHandler {
	return &AnomalyPolicyHandler{service: service}
}

type createAnomalyPolicyRequest struct {
	Name                  string     `json:"name"`
	RuleType              string     `json:"rule_type"`
	ScopeType             string     `json:"scope_type"`
	ScopeID               *uuid.UUID `json:"scope_id"`
	ModelAlias            string     `json:"model_alias"`
	ThresholdMicroUSD     *int64     `json:"threshold_micro_usd"`
	ThresholdBPS          *int32     `json:"threshold_bps"`
	SpikeMultiplierBPS    int32      `json:"spike_multiplier_bps"`
	CurrentWindowMinutes  int32      `json:"current_window_minutes"`
	BaselineWindowMinutes int32      `json:"baseline_window_minutes"`
	MinRequests           int32      `json:"min_requests"`
	Action                string     `json:"action"`
	FallbackModel         string     `json:"fallback_model"`
}

type anomalyPolicyResponse struct {
	ID                    uuid.UUID  `json:"id"`
	Name                  string     `json:"name"`
	RuleType              string     `json:"rule_type"`
	ScopeType             string     `json:"scope_type"`
	ScopeID               *uuid.UUID `json:"scope_id"`
	ModelAlias            *string    `json:"model_alias"`
	ThresholdMicroUSD     *int64     `json:"threshold_micro_usd"`
	ThresholdBPS          *int32     `json:"threshold_bps"`
	SpikeMultiplierBPS    int32      `json:"spike_multiplier_bps"`
	CurrentWindowMinutes  int32      `json:"current_window_minutes"`
	BaselineWindowMinutes int32      `json:"baseline_window_minutes"`
	MinRequests           int32      `json:"min_requests"`
	Action                string     `json:"action"`
	FallbackModel         *string    `json:"fallback_model"`
	Status                string     `json:"status"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type listAnomalyPoliciesResponse struct {
	Items []anomalyPolicyResponse `json:"items"`
}

func (h *AnomalyPolicyHandler) Create(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	var request createAnomalyPolicyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}
	created, err := h.service.CreatePolicy(c.Request.Context(), anomaly.CreatePolicyParams{
		OrgID:                 principal.OrgID,
		Name:                  request.Name,
		RuleType:              request.RuleType,
		ScopeType:             request.ScopeType,
		ScopeID:               request.ScopeID,
		ModelAlias:            request.ModelAlias,
		ThresholdMicroUSD:     request.ThresholdMicroUSD,
		ThresholdBPS:          request.ThresholdBPS,
		SpikeMultiplierBPS:    request.SpikeMultiplierBPS,
		CurrentWindowMinutes:  request.CurrentWindowMinutes,
		BaselineWindowMinutes: request.BaselineWindowMinutes,
		MinRequests:           request.MinRequests,
		Action:                request.Action,
		FallbackModel:         request.FallbackModel,
	})
	if err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest(err.Error()))
		return
	}

	middleware.SetAdminAuditResource(c, "anomaly_policy", created.ID)
	middleware.SetAdminAuditAction(c, "create_anomaly_policy")
	httpapi.RespondJSON(c, http.StatusCreated, newAnomalyPolicyResponse(created))
}

func (h *AnomalyPolicyHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	policies, err := h.service.ListPolicies(c.Request.Context(), principal.OrgID)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	items := make([]anomalyPolicyResponse, 0, len(policies))
	for _, policy := range policies {
		items = append(items, newAnomalyPolicyResponse(policy))
	}
	httpapi.RespondJSON(c, http.StatusOK, listAnomalyPoliciesResponse{Items: items})
}

func newAnomalyPolicyResponse(policy db.CostAnomalyPolicy) anomalyPolicyResponse {
	return anomalyPolicyResponse{
		ID:                    policy.ID,
		Name:                  policy.Name,
		RuleType:              policy.RuleType,
		ScopeType:             policy.ScopeType,
		ScopeID:               policy.ScopeID,
		ModelAlias:            anomalyTextPtr(policy.ModelAlias.String, policy.ModelAlias.Valid),
		ThresholdMicroUSD:     anomalyInt64Ptr(policy.ThresholdMicroUsd.Int64, policy.ThresholdMicroUsd.Valid),
		ThresholdBPS:          anomalyInt32Ptr(policy.ThresholdBps.Int32, policy.ThresholdBps.Valid),
		SpikeMultiplierBPS:    policy.SpikeMultiplierBps,
		CurrentWindowMinutes:  policy.CurrentWindowMinutes,
		BaselineWindowMinutes: policy.BaselineWindowMinutes,
		MinRequests:           policy.MinRequests,
		Action:                policy.Action,
		FallbackModel:         anomalyTextPtr(policy.FallbackModel.String, policy.FallbackModel.Valid),
		Status:                policy.Status,
		CreatedAt:             policy.CreatedAt,
		UpdatedAt:             policy.UpdatedAt,
	}
}

func anomalyTextPtr(value string, valid bool) *string {
	if !valid || strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func anomalyInt64Ptr(value int64, valid bool) *int64 {
	if !valid {
		return nil
	}
	return &value
}

func anomalyInt32Ptr(value int32, valid bool) *int32 {
	if !valid {
		return nil
	}
	return &value
}
