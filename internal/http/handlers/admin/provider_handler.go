package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	"github.com/tep/llm-cost-gateway/internal/provider"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type ProviderHandler struct {
	service *provider.Service
}

func NewProviderHandler(service *provider.Service) *ProviderHandler {
	return &ProviderHandler{service: service}
}

type createProviderRequest struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	BaseURL   *string `json:"base_url"`
	APIKey    *string `json:"api_key"`
	TimeoutMS int32   `json:"timeout_ms"`
}

type providerResponse struct {
	ID                  uuid.UUID  `json:"id"`
	Name                string     `json:"name"`
	Type                string     `json:"type"`
	BaseURL             *string    `json:"base_url"`
	Status              string     `json:"status"`
	TimeoutMS           int32      `json:"timeout_ms"`
	LastHealthStatus    *string    `json:"last_health_status"`
	LastHealthCheckedAt *time.Time `json:"last_health_checked_at"`
	LastErrorCode       *string    `json:"last_error_code"`
	LastErrorMessage    *string    `json:"last_error_message"`
	CreatedAt           time.Time  `json:"created_at"`
}

type listProvidersResponse struct {
	Items []providerResponse `json:"items"`
}

func (h *ProviderHandler) Create(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	var request createProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}

	created, err := h.service.CreateProvider(c.Request.Context(), provider.CreateProviderParams{
		OrgID:     principal.OrgID,
		Name:      request.Name,
		Type:      request.Type,
		BaseURL:   request.BaseURL,
		APIKey:    request.APIKey,
		TimeoutMS: request.TimeoutMS,
	})
	if err != nil {
		respondProviderServiceError(c, err)
		return
	}

	httpapi.RespondJSON(c, http.StatusCreated, newProviderResponse(created))
}

func (h *ProviderHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	providers, err := h.service.ListProviders(c.Request.Context(), principal.OrgID)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	items := make([]providerResponse, 0, len(providers))
	for _, item := range providers {
		items = append(items, newProviderResponse(item))
	}
	httpapi.RespondJSON(c, http.StatusOK, listProvidersResponse{Items: items})
}

func newProviderResponse(item db.Provider) providerResponse {
	var baseURL *string
	if item.BaseUrl.Valid {
		baseURL = &item.BaseUrl.String
	}

	return providerResponse{
		ID:                  item.ID,
		Name:                item.Name,
		Type:                item.Type,
		BaseURL:             baseURL,
		Status:              item.Status,
		TimeoutMS:           item.TimeoutMs,
		LastHealthStatus:    pgTextPtr(item.LastHealthStatus),
		LastHealthCheckedAt: item.LastHealthCheckedAt,
		LastErrorCode:       pgTextPtr(item.LastErrorCode),
		LastErrorMessage:    pgTextPtr(item.LastErrorMessage),
		CreatedAt:           item.CreatedAt,
	}
}

func pgTextPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func respondProviderServiceError(c *gin.Context, err error) {
	if validation, ok := provider.IsValidationError(err); ok {
		httpapi.RespondError(c, httpapi.InvalidRequest(validation.Message))
		return
	}
	if errors.Is(err, provider.ErrProviderNotFound) {
		httpapi.RespondError(c, httpapi.NotFound("provider not found"))
		return
	}
	if provider.IsUniqueViolation(err) {
		httpapi.RespondError(c, httpapi.Conflict("resource already exists"))
		return
	}
	httpapi.RespondError(c, httpapi.InternalError("internal server error"))
}
