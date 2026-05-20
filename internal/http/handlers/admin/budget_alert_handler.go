package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/budget"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type BudgetAlertHandler struct {
	service *budget.AlertService
}

func NewBudgetAlertHandler(service *budget.AlertService) *BudgetAlertHandler {
	return &BudgetAlertHandler{service: service}
}

type createBudgetAlertRequest struct {
	BudgetID      uuid.UUID `json:"budget_id"`
	WebhookURL    string    `json:"webhook_url"`
	WebhookSecret string    `json:"webhook_secret"`
	Status        string    `json:"status"`
}

type budgetAlertResponse struct {
	ID         uuid.UUID `json:"id"`
	BudgetID   uuid.UUID `json:"budget_id"`
	WebhookURL string    `json:"webhook_url"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type budgetAlertDeliveryResponse struct {
	ID                uuid.UUID `json:"id"`
	BudgetAlertID     uuid.UUID `json:"budget_alert_id"`
	BudgetID          uuid.UUID `json:"budget_id"`
	Threshold         int32     `json:"threshold"`
	Period            string    `json:"period"`
	PeriodWindowStart time.Time `json:"period_window_start"`
	PeriodWindowEnd   time.Time `json:"period_window_end"`
	UsedMicroUSD      int64     `json:"used_micro_usd"`
	LimitMicroUSD     int64     `json:"limit_micro_usd"`
	WebhookURL        string    `json:"webhook_url"`
	Status            string    `json:"status"`
	HTTPStatus        *int32    `json:"http_status"`
	ErrorMessage      *string   `json:"error_message"`
	CreatedAt         time.Time `json:"created_at"`
}

type listBudgetAlertsResponse struct {
	Items []budgetAlertResponse `json:"items"`
}

type listBudgetAlertDeliveriesResponse struct {
	Items []budgetAlertDeliveryResponse `json:"items"`
}

func (h *BudgetAlertHandler) Create(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	var request createBudgetAlertRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}
	if request.BudgetID == uuid.Nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("budget_id is required"))
		return
	}
	if strings.TrimSpace(request.WebhookURL) == "" {
		httpapi.RespondError(c, httpapi.InvalidRequest("webhook_url is required"))
		return
	}

	created, err := h.service.CreateAlert(c.Request.Context(), budget.CreateAlertParams{
		OrgID:         principal.OrgID,
		BudgetID:      request.BudgetID,
		WebhookURL:    request.WebhookURL,
		WebhookSecret: request.WebhookSecret,
		Status:        request.Status,
	})
	if err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest(err.Error()))
		return
	}

	httpapi.RespondJSON(c, http.StatusCreated, newBudgetAlertResponse(created))
}

func (h *BudgetAlertHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	alerts, err := h.service.ListAlerts(c.Request.Context(), principal.OrgID)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	items := make([]budgetAlertResponse, 0, len(alerts))
	for _, item := range alerts {
		items = append(items, newBudgetAlertResponse(item))
	}
	httpapi.RespondJSON(c, http.StatusOK, listBudgetAlertsResponse{Items: items})
}

func (h *BudgetAlertHandler) ListDeliveries(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}
	limit := int32(100)
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 || parsed > 200 {
			httpapi.RespondError(c, httpapi.InvalidRequest("limit must be between 1 and 200"))
			return
		}
		limit = int32(parsed)
	}

	deliveries, err := h.service.ListDeliveries(c.Request.Context(), principal.OrgID, limit)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	items := make([]budgetAlertDeliveryResponse, 0, len(deliveries))
	for _, item := range deliveries {
		items = append(items, newBudgetAlertDeliveryResponse(item))
	}
	httpapi.RespondJSON(c, http.StatusOK, listBudgetAlertDeliveriesResponse{Items: items})
}

func newBudgetAlertResponse(item db.BudgetAlert) budgetAlertResponse {
	return budgetAlertResponse{
		ID:         item.ID,
		BudgetID:   item.BudgetID,
		WebhookURL: item.WebhookUrl,
		Status:     item.Status,
		CreatedAt:  item.CreatedAt,
	}
}

func newBudgetAlertDeliveryResponse(item db.BudgetAlertDelivery) budgetAlertDeliveryResponse {
	return budgetAlertDeliveryResponse{
		ID:                item.ID,
		BudgetAlertID:     item.BudgetAlertID,
		BudgetID:          item.BudgetID,
		Threshold:         item.Threshold,
		Period:            item.Period,
		PeriodWindowStart: item.PeriodWindowStart,
		PeriodWindowEnd:   item.PeriodWindowEnd,
		UsedMicroUSD:      item.UsedMicroUsd,
		LimitMicroUSD:     item.LimitMicroUsd,
		WebhookURL:        item.WebhookUrl,
		Status:            item.Status,
		HTTPStatus:        pgInt4Ptr(item.HttpStatus),
		ErrorMessage:      pgTextPtr(item.ErrorMessage),
		CreatedAt:         item.CreatedAt,
	}
}

func pgInt4Ptr(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}
