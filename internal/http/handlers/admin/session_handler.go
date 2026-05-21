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
)

type SessionHandler struct {
	service *auth.SessionService
}

func NewSessionHandler(service *auth.SessionService) *SessionHandler {
	return &SessionHandler{service: service}
}

type passwordlessMockLoginRequest struct {
	OrgSlug string `json:"org_slug"`
	Email   string `json:"email"`
}

type passwordlessMockLoginResponse struct {
	Token        string    `json:"token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	OrgID        uuid.UUID `json:"org_id"`
	OrgSlug      string    `json:"org_slug"`
	UserID       uuid.UUID `json:"user_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	MembershipID uuid.UUID `json:"membership_id"`
	Role         string    `json:"role"`
}

func (h *SessionHandler) PasswordlessMockLogin(c *gin.Context) {
	var request passwordlessMockLoginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}
	if strings.TrimSpace(request.OrgSlug) == "" {
		httpapi.RespondError(c, httpapi.InvalidRequest("org_slug is required"))
		return
	}
	if strings.TrimSpace(request.Email) == "" {
		httpapi.RespondError(c, httpapi.InvalidRequest("email is required"))
		return
	}

	result, err := h.service.PasswordlessMockLogin(c.Request.Context(), auth.PasswordlessMockLoginParams{
		OrgSlug: request.OrgSlug,
		Email:   request.Email,
	})
	if err != nil {
		if errors.Is(err, auth.ErrMembershipNotFound) {
			httpapi.RespondError(c, httpapi.Unauthorized("membership not found"))
			return
		}
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	httpapi.RespondJSON(c, http.StatusOK, passwordlessMockLoginResponse{
		Token:        result.Token,
		TokenType:    "session",
		ExpiresAt:    result.Session.ExpiresAt,
		OrgID:        result.OrgID,
		OrgSlug:      result.OrgSlug,
		UserID:       result.UserID,
		Email:        result.Email,
		DisplayName:  result.DisplayName,
		MembershipID: result.MembershipID,
		Role:         result.Role,
	})
}
