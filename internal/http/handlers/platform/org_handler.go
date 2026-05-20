package platform

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type OrgHandler struct {
	queries *db.Queries
}

func NewOrgHandler(queries *db.Queries) *OrgHandler {
	return &OrgHandler{queries: queries}
}

type createOrgRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type orgResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *OrgHandler) Create(c *gin.Context) {
	var request createOrgRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}

	name := strings.TrimSpace(request.Name)
	slug := strings.TrimSpace(request.Slug)
	if name == "" || slug == "" {
		httpapi.RespondError(c, httpapi.InvalidRequest("name and slug are required"))
		return
	}

	now := time.Now().UTC()
	org, err := h.queries.CreateOrganization(c.Request.Context(), db.CreateOrganizationParams{
		ID:        uuid.New(),
		Name:      name,
		Slug:      slug,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		respondPlatformDBError(c, err)
		return
	}

	httpapi.RespondJSON(c, http.StatusCreated, orgResponse{
		ID:        org.ID,
		Name:      org.Name,
		Slug:      org.Slug,
		Status:    org.Status,
		CreatedAt: org.CreatedAt,
	})
}

func respondPlatformDBError(c *gin.Context, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		httpapi.RespondError(c, httpapi.Conflict("resource already exists"))
		return
	}

	httpapi.RespondError(c, httpapi.InternalError("internal server error"))
}
