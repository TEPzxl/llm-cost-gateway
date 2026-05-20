package admin

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/auth"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type APIKeyHandler struct {
	service *auth.APIKeyService
}

func NewAPIKeyHandler(service *auth.APIKeyService) *APIKeyHandler {
	return &APIKeyHandler{service: service}
}

type createAPIKeyRequest struct {
	Name                     string     `json:"name"`
	Scopes                   []string   `json:"scopes"`
	RPMLimit                 int32      `json:"rpm_limit"`
	DailyCostLimitMicroUSD   *int64     `json:"daily_cost_limit_micro_usd"`
	MonthlyCostLimitMicroUSD *int64     `json:"monthly_cost_limit_micro_usd"`
	QuotaAction              string     `json:"quota_action"`
	ExpiresAt                *time.Time `json:"expires_at"`
}

type createAPIKeyResponse struct {
	ID                       uuid.UUID `json:"id"`
	Name                     string    `json:"name"`
	Key                      string    `json:"key"`
	KeyPrefix                string    `json:"key_prefix"`
	Status                   string    `json:"status"`
	RPMLimit                 int32     `json:"rpm_limit"`
	DailyCostLimitMicroUSD   *int64    `json:"daily_cost_limit_micro_usd"`
	MonthlyCostLimitMicroUSD *int64    `json:"monthly_cost_limit_micro_usd"`
	QuotaAction              string    `json:"quota_action"`
	CreatedAt                time.Time `json:"created_at"`
}

type apiKeyResponse struct {
	ID                       uuid.UUID  `json:"id"`
	Name                     string     `json:"name"`
	KeyPrefix                string     `json:"key_prefix"`
	Status                   string     `json:"status"`
	RPMLimit                 int32      `json:"rpm_limit"`
	DailyCostLimitMicroUSD   *int64     `json:"daily_cost_limit_micro_usd"`
	MonthlyCostLimitMicroUSD *int64     `json:"monthly_cost_limit_micro_usd"`
	QuotaAction              string     `json:"quota_action"`
	ExpiresAt                *time.Time `json:"expires_at"`
	LastUsedAt               *time.Time `json:"last_used_at"`
	CreatedAt                time.Time  `json:"created_at"`
}

type listAPIKeysResponse struct {
	Items []apiKeyResponse `json:"items"`
}

func (h *APIKeyHandler) Create(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	var request createAPIKeyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}
	if strings.TrimSpace(request.Name) == "" {
		httpapi.RespondError(c, httpapi.InvalidRequest("name is required"))
		return
	}
	if request.RPMLimit <= 0 {
		httpapi.RespondError(c, httpapi.InvalidRequest("rpm_limit must be greater than zero"))
		return
	}
	if request.DailyCostLimitMicroUSD != nil && *request.DailyCostLimitMicroUSD < 0 {
		httpapi.RespondError(c, httpapi.InvalidRequest("daily_cost_limit_micro_usd must be non-negative"))
		return
	}
	if request.MonthlyCostLimitMicroUSD != nil && *request.MonthlyCostLimitMicroUSD < 0 {
		httpapi.RespondError(c, httpapi.InvalidRequest("monthly_cost_limit_micro_usd must be non-negative"))
		return
	}
	if request.QuotaAction != "" && request.QuotaAction != auth.QuotaActionWarn && request.QuotaAction != auth.QuotaActionBlock {
		httpapi.RespondError(c, httpapi.InvalidRequest("quota_action must be warn or block"))
		return
	}

	created, err := h.service.CreateAPIKey(c.Request.Context(), auth.CreateAPIKeyParams{
		OrgID:                    principal.OrgID,
		Name:                     request.Name,
		Scopes:                   request.Scopes,
		RPMLimit:                 request.RPMLimit,
		DailyCostLimitMicroUSD:   request.DailyCostLimitMicroUSD,
		MonthlyCostLimitMicroUSD: request.MonthlyCostLimitMicroUSD,
		QuotaAction:              request.QuotaAction,
		ExpiresAt:                request.ExpiresAt,
	})
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	httpapi.RespondJSON(c, http.StatusCreated, createAPIKeyResponse{
		ID:                       created.APIKey.ID,
		Name:                     created.APIKey.Name,
		Key:                      created.Key,
		KeyPrefix:                created.APIKey.KeyPrefix,
		Status:                   created.APIKey.Status,
		RPMLimit:                 created.APIKey.RpmLimit,
		DailyCostLimitMicroUSD:   pgInt8Ptr(created.APIKey.DailyCostLimitMicroUsd),
		MonthlyCostLimitMicroUSD: pgInt8Ptr(created.APIKey.MonthlyCostLimitMicroUsd),
		QuotaAction:              created.APIKey.QuotaAction,
		CreatedAt:                created.APIKey.CreatedAt,
	})
}

func (h *APIKeyHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	apiKeys, err := h.service.ListAPIKeys(c.Request.Context(), principal.OrgID)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	items := make([]apiKeyResponse, 0, len(apiKeys))
	for _, apiKey := range apiKeys {
		items = append(items, newAPIKeyResponse(apiKey))
	}
	httpapi.RespondJSON(c, http.StatusOK, listAPIKeysResponse{Items: items})
}

func (h *APIKeyHandler) Revoke(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	apiKeyID, err := uuid.Parse(c.Param("api_key_id"))
	if err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid api_key_id"))
		return
	}

	apiKey, err := h.service.RevokeAPIKey(c.Request.Context(), principal.OrgID, apiKeyID)
	if err != nil {
		if errors.Is(err, auth.ErrAPIKeyNotFound) {
			httpapi.RespondError(c, httpapi.NotFound("api key not found"))
			return
		}
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}

	httpapi.RespondJSON(c, http.StatusOK, newAPIKeyResponse(apiKey))
}

func newAPIKeyResponse(apiKey db.ApiKey) apiKeyResponse {
	return apiKeyResponse{
		ID:                       apiKey.ID,
		Name:                     apiKey.Name,
		KeyPrefix:                apiKey.KeyPrefix,
		Status:                   apiKey.Status,
		RPMLimit:                 apiKey.RpmLimit,
		DailyCostLimitMicroUSD:   pgInt8Ptr(apiKey.DailyCostLimitMicroUsd),
		MonthlyCostLimitMicroUSD: pgInt8Ptr(apiKey.MonthlyCostLimitMicroUsd),
		QuotaAction:              apiKey.QuotaAction,
		ExpiresAt:                apiKey.ExpiresAt,
		LastUsedAt:               apiKey.LastUsedAt,
		CreatedAt:                apiKey.CreatedAt,
	}
}

func pgInt8Ptr(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
