package admin

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/auth"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type MembersHandler struct {
	service *auth.SessionService
}

func NewMembersHandler(service *auth.SessionService) *MembersHandler {
	return &MembersHandler{service: service}
}

type upsertMemberRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type memberResponse struct {
	MembershipID uuid.UUID `json:"membership_id"`
	OrgID        uuid.UUID `json:"org_id"`
	UserID       uuid.UUID `json:"user_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type listMembersResponse struct {
	Items []memberResponse `json:"items"`
}

func (h *MembersHandler) Create(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	var request upsertMemberRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}
	if strings.TrimSpace(request.Email) == "" {
		httpapi.RespondError(c, httpapi.InvalidRequest("email is required"))
		return
	}
	if !auth.ValidRole(request.Role) {
		httpapi.RespondError(c, httpapi.InvalidRequest("role must be owner, admin, or viewer"))
		return
	}

	result, err := h.service.UpsertMember(c.Request.Context(), auth.UpsertMemberParams{
		OrgID:       principal.OrgID,
		Email:       request.Email,
		DisplayName: request.DisplayName,
		Role:        request.Role,
	})
	if err != nil {
		if errors.Is(err, auth.ErrOrganizationNotFound) {
			httpapi.RespondError(c, httpapi.NotFound("organization not found"))
			return
		}
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	httpapi.RespondJSON(c, http.StatusCreated, memberResponse{
		MembershipID: result.Membership.ID,
		OrgID:        result.Membership.OrgID,
		UserID:       result.User.ID,
		Email:        result.User.Email,
		DisplayName:  result.User.DisplayName,
		Role:         result.Membership.Role,
		Status:       result.Membership.Status,
		CreatedAt:    result.Membership.CreatedAt,
		UpdatedAt:    result.Membership.UpdatedAt,
	})
}

func (h *MembersHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	members, err := h.service.ListMembers(c.Request.Context(), principal.OrgID)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	items := make([]memberResponse, 0, len(members))
	for _, member := range members {
		items = append(items, newMemberResponse(member))
	}
	httpapi.RespondJSON(c, http.StatusOK, listMembersResponse{Items: items})
}

func newMemberResponse(member db.ListOrgMembersRow) memberResponse {
	return memberResponse{
		MembershipID: member.MembershipID,
		OrgID:        member.OrgID,
		UserID:       member.UserID,
		Email:        member.Email,
		DisplayName:  member.DisplayName,
		Role:         member.Role,
		Status:       member.Status,
		CreatedAt:    member.CreatedAt,
		UpdatedAt:    member.UpdatedAt,
	}
}
