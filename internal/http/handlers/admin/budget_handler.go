package admin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/budget"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type BudgetHandler struct {
	service *budget.Service
}

func NewBudgetHandler(service *budget.Service) *BudgetHandler {
	return &BudgetHandler{service: service}
}

type createBudgetRequest struct {
	Name          string     `json:"name"`
	ScopeType     string     `json:"scope_type"`
	ScopeID       *uuid.UUID `json:"scope_id"`
	Period        string     `json:"period"`
	LimitMicroUSD int64      `json:"limit_micro_usd"`
	Action        string     `json:"action"`
}

type budgetResponse struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	ScopeType     string    `json:"scope_type"`
	Period        string    `json:"period"`
	LimitMicroUSD int64     `json:"limit_micro_usd"`
	Action        string    `json:"action"`
	Status        string    `json:"status"`
}

type listBudgetsResponse struct {
	Items []budgetResponse `json:"items"`
}

type budgetStatusResponse struct {
	BudgetID          uuid.UUID `json:"budget_id"`
	Name              string    `json:"name"`
	Period            string    `json:"period"`
	LimitMicroUSD     int64     `json:"limit_micro_usd"`
	UsedMicroUSD      int64     `json:"used_micro_usd"`
	RemainingMicroUSD int64     `json:"remaining_micro_usd"`
	Exceeded          bool      `json:"exceeded"`
	Action            string    `json:"action"`
}

type listBudgetStatusesResponse struct {
	Items []budgetStatusResponse `json:"items"`
}

func (h *BudgetHandler) Create(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	var request createBudgetRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}
	if err := validateCreateBudgetRequest(request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest(err.Error()))
		return
	}

	created, err := h.service.CreateBudget(c.Request.Context(), budget.CreateBudgetParams{
		OrgID:         principal.OrgID,
		Name:          request.Name,
		ScopeType:     request.ScopeType,
		Period:        request.Period,
		LimitMicroUSD: request.LimitMicroUSD,
		Action:        request.Action,
	})
	if err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest(err.Error()))
		return
	}

	httpapi.RespondJSON(c, http.StatusCreated, newBudgetResponse(created))
}

func (h *BudgetHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	budgets, err := h.service.ListBudgets(c.Request.Context(), principal.OrgID)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	items := make([]budgetResponse, 0, len(budgets))
	for _, item := range budgets {
		items = append(items, newBudgetResponse(item))
	}
	httpapi.RespondJSON(c, http.StatusOK, listBudgetsResponse{Items: items})
}

func (h *BudgetHandler) Status(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	statuses, err := h.service.GetStatus(c.Request.Context(), principal.OrgID)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	items := make([]budgetStatusResponse, 0, len(statuses))
	for _, status := range statuses {
		items = append(items, budgetStatusResponse{
			BudgetID:          status.Budget.ID,
			Name:              status.Budget.Name,
			Period:            status.Budget.Period,
			LimitMicroUSD:     status.Budget.LimitMicroUsd,
			UsedMicroUSD:      status.UsedMicroUSD,
			RemainingMicroUSD: status.RemainingMicroUSD,
			Exceeded:          status.Exceeded,
			Action:            status.Budget.Action,
		})
	}
	httpapi.RespondJSON(c, http.StatusOK, listBudgetStatusesResponse{Items: items})
}

func validateCreateBudgetRequest(request createBudgetRequest) error {
	if strings.TrimSpace(request.Name) == "" {
		return httpapi.InvalidRequest("name is required")
	}
	if request.ScopeType != budget.ScopeTypeOrg {
		return httpapi.InvalidRequest("scope_type must be org")
	}
	if request.ScopeID != nil {
		return httpapi.InvalidRequest("scope_id must be null for org budgets")
	}
	if request.Period != budget.PeriodDaily && request.Period != budget.PeriodMonthly {
		return httpapi.InvalidRequest("period must be daily or monthly")
	}
	if request.LimitMicroUSD < 0 {
		return httpapi.InvalidRequest("limit_micro_usd must be non-negative")
	}
	if request.Action != budget.ActionWarn && request.Action != budget.ActionBlock {
		return httpapi.InvalidRequest("action must be warn or block")
	}
	return nil
}

func newBudgetResponse(item db.Budget) budgetResponse {
	return budgetResponse{
		ID:            item.ID,
		Name:          item.Name,
		ScopeType:     item.ScopeType,
		Period:        item.Period,
		LimitMicroUSD: item.LimitMicroUsd,
		Action:        item.Action,
		Status:        item.Status,
	}
}
