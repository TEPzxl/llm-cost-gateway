package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tep/llm-cost-gateway/internal/auth"
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
	Org        meOrgResponse         `json:"org"`
	ActorType  string                `json:"actor_type"`
	Role       string                `json:"role"`
	AdminToken *meAdminTokenResponse `json:"admin_token,omitempty"`
	User       *meUserResponse       `json:"user,omitempty"`
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

type meUserResponse struct {
	ID           uuid.UUID  `json:"id"`
	Email        string     `json:"email"`
	DisplayName  string     `json:"display_name"`
	MembershipID *uuid.UUID `json:"membership_id,omitempty"`
	SessionID    *uuid.UUID `json:"session_id,omitempty"`
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
	actorType := principal.ActorType
	if actorType == "" {
		actorType = auth.ActorServiceToken
	}
	response := meResponse{
		Org: meOrgResponse{
			ID:   org.ID,
			Name: org.Name,
			Slug: org.Slug,
		},
		ActorType: actorType,
		Role:      principal.Role,
	}
	if principal.IsServiceToken() {
		adminToken, err := h.queries.GetAdminToken(c.Request.Context(), db.GetAdminTokenParams{
			OrgID: principal.OrgID,
			ID:    principal.AdminTokenID,
		})
		if err != nil {
			respondAdminReadError(c, err)
			return
		}
		response.AdminToken = &meAdminTokenResponse{
			ID:     adminToken.ID,
			Name:   adminToken.Name,
			Scopes: adminToken.Scopes,
		}
	} else {
		userID := uuid.Nil
		if principal.UserID != nil {
			userID = *principal.UserID
		}
		response.User = &meUserResponse{
			ID:           userID,
			Email:        principal.UserEmail,
			DisplayName:  principal.DisplayName,
			MembershipID: principal.MembershipID,
			SessionID:    principal.SessionID,
		}
	}

	httpapi.RespondJSON(c, http.StatusOK, response)
}

func respondAdminReadError(c *gin.Context, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.RespondError(c, httpapi.NotFound("resource not found"))
		return
	}
	httpapi.RespondError(c, httpapi.InternalError("internal server error"))
}
