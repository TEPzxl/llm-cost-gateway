package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	"github.com/tep/llm-cost-gateway/internal/policy"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type ContentPolicyHandler struct {
	service *policy.Service
}

func NewContentPolicyHandler(service *policy.Service) *ContentPolicyHandler {
	return &ContentPolicyHandler{service: service}
}

type createContentPolicyRequest struct {
	Name      string `json:"name"`
	PIIAction string `json:"pii_action"`
}

type contentPolicyResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	PIIAction string    `json:"pii_action"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type listContentPoliciesResponse struct {
	Items []contentPolicyResponse `json:"items"`
}

func (h *ContentPolicyHandler) Create(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}
	var request createContentPolicyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		httpapi.RespondError(c, httpapi.InvalidRequest("name is required"))
		return
	}
	created, err := h.service.CreateContentPolicy(c.Request.Context(), policy.CreateContentPolicyParams{
		OrgID:     principal.OrgID,
		Name:      request.Name,
		PIIAction: request.PIIAction,
	})
	if err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest(err.Error()))
		return
	}
	middleware.SetAdminAuditResource(c, "content_policy", created.ID)
	middleware.SetAdminAuditAction(c, "create_content_policy")
	httpapi.RespondJSON(c, http.StatusCreated, newContentPolicyResponse(created))
}

func (h *ContentPolicyHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}
	policies, err := h.service.ListContentPolicies(c.Request.Context(), principal.OrgID)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	items := make([]contentPolicyResponse, 0, len(policies))
	for _, item := range policies {
		items = append(items, newContentPolicyResponse(item))
	}
	httpapi.RespondJSON(c, http.StatusOK, listContentPoliciesResponse{Items: items})
}

func newContentPolicyResponse(item db.ContentPolicy) contentPolicyResponse {
	return contentPolicyResponse{
		ID:        item.ID,
		Name:      item.Name,
		PIIAction: item.PiiAction,
		Status:    item.Status,
		CreatedAt: item.CreatedAt,
	}
}
