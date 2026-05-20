package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type MeHandler struct {
	queries *db.Queries
}

func NewMeHandler(queries *db.Queries) *MeHandler {
	return &MeHandler{queries: queries}
}

type meResponse struct {
	Org        meOrgResponse        `json:"org"`
	AdminToken meAdminTokenResponse `json:"admin_token"`
}

type meOrgResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

type meAdminTokenResponse struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Scopes []string  `json:"scopes"`
}

func (h *MeHandler) Get(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	org, err := h.queries.GetOrganization(c.Request.Context(), principal.OrgID)
	if err != nil {
		respondAdminReadError(c, err)
		return
	}
	adminToken, err := h.queries.GetAdminToken(c.Request.Context(), db.GetAdminTokenParams{
		OrgID: principal.OrgID,
		ID:    principal.AdminTokenID,
	})
	if err != nil {
		respondAdminReadError(c, err)
		return
	}

	httpapi.RespondJSON(c, http.StatusOK, meResponse{
		Org: meOrgResponse{
			ID:   org.ID,
			Name: org.Name,
			Slug: org.Slug,
		},
		AdminToken: meAdminTokenResponse{
			ID:     adminToken.ID,
			Name:   adminToken.Name,
			Scopes: adminToken.Scopes,
		},
	})
}

func respondAdminReadError(c *gin.Context, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.RespondError(c, httpapi.NotFound("resource not found"))
		return
	}
	httpapi.RespondError(c, httpapi.InternalError("internal server error"))
}
