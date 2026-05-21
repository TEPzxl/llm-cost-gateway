package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/audit"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type AuditHandler struct {
	service *audit.AdminAuditService
}

func NewAuditHandler(service *audit.AdminAuditService) *AuditHandler {
	return &AuditHandler{service: service}
}

type auditLogResponse struct {
	ID                uuid.UUID  `json:"id"`
	ActorAdminTokenID *uuid.UUID `json:"actor_admin_token_id"`
	ActorUserID       *uuid.UUID `json:"actor_user_id"`
	Action            string     `json:"action"`
	ResourceType      string     `json:"resource_type"`
	ResourceID        *uuid.UUID `json:"resource_id"`
	RequestID         string     `json:"request_id"`
	CreatedAt         time.Time  `json:"created_at"`
}

type listAuditLogsResponse struct {
	Items []auditLogResponse `json:"items"`
}

func (h *AuditHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}
	limit, ok := parseBoundedInt32(c, "limit", defaultListLimit, maxListLimit)
	if !ok {
		return
	}
	offset, ok := parseBoundedInt32(c, "offset", 0, 1_000_000)
	if !ok {
		return
	}

	logs, err := h.service.List(c.Request.Context(), principal.OrgID, limit, offset)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	items := make([]auditLogResponse, 0, len(logs))
	for _, item := range logs {
		items = append(items, newAuditLogResponse(item))
	}
	httpapi.RespondJSON(c, http.StatusOK, listAuditLogsResponse{Items: items})
}

func newAuditLogResponse(item db.AdminAuditLog) auditLogResponse {
	return auditLogResponse{
		ID:                item.ID,
		ActorAdminTokenID: item.ActorAdminTokenID,
		ActorUserID:       item.ActorUserID,
		Action:            item.Action,
		ResourceType:      item.ResourceType,
		ResourceID:        item.ResourceID,
		RequestID:         item.RequestID,
		CreatedAt:         item.CreatedAt,
	}
}
