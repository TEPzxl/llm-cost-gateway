package platform

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

type AdminTokenHandler struct {
	service *auth.AdminTokenService
}

func NewAdminTokenHandler(service *auth.AdminTokenService) *AdminTokenHandler {
	return &AdminTokenHandler{service: service}
}

type createAdminTokenRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type createAdminTokenResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Token       string    `json:"token"`
	TokenPrefix string    `json:"token_prefix"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h *AdminTokenHandler) Create(c *gin.Context) {
	orgID, err := uuid.Parse(c.Param("org_id"))
	if err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid org_id"))
		return
	}

	var request createAdminTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		httpapi.RespondError(c, httpapi.InvalidRequest("name is required"))
		return
	}

	created, err := h.service.CreateAdminToken(c.Request.Context(), auth.CreateAdminTokenParams{
		OrgID:     orgID,
		Name:      request.Name,
		Scopes:    request.Scopes,
		ExpiresAt: request.ExpiresAt,
	})
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrOrganizationNotFound):
			httpapi.RespondError(c, httpapi.NotFound("organization not found"))
		default:
			httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		}
		return
	}

	httpapi.RespondJSON(c, http.StatusCreated, createAdminTokenResponse{
		ID:          created.AdminToken.ID,
		Name:        created.AdminToken.Name,
		Token:       created.Token,
		TokenPrefix: created.AdminToken.TokenPrefix,
		Status:      created.AdminToken.Status,
		CreatedAt:   created.AdminToken.CreatedAt,
	})
}
