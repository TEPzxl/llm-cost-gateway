package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	defaultListLimit = int32(50)
	maxListLimit     = int32(200)
	defaultWindow    = 24 * time.Hour
)

type RequestLogHandler struct {
	queries *db.Queries
	now     func() time.Time
}

func NewRequestLogHandler(queries *db.Queries) *RequestLogHandler {
	return &RequestLogHandler{
		queries: queries,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

type requestLogResponse struct {
	ID                uuid.UUID  `json:"id"`
	RequestModel      *string    `json:"request_model"`
	Status            string     `json:"status"`
	StatusCode        int32      `json:"status_code"`
	ErrorCode         *string    `json:"error_code"`
	LatencyMS         int32      `json:"latency_ms"`
	ProviderLatencyMS *int32     `json:"provider_latency_ms"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       time.Time  `json:"completed_at"`
	APIKeyID          *uuid.UUID `json:"api_key_id,omitempty"`
	ProviderID        *uuid.UUID `json:"provider_id,omitempty"`
	ModelID           *uuid.UUID `json:"model_id,omitempty"`
}

type listRequestLogsResponse struct {
	Items      []requestLogResponse `json:"items"`
	NextCursor *string              `json:"next_cursor"`
}

func (h *RequestLogHandler) List(c *gin.Context) {
	principal, ok := middleware.AdminTokenPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	window, ok := h.parseWindow(c)
	if !ok {
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

	params := db.ListRequestLogsParams{
		OrgID:  principal.OrgID,
		FromAt: window.from,
		ToAt:   window.to,
		Limit:  limit,
		Offset: offset,
		Status: pgText(c.Query("status")),
	}
	var err error
	if params.ApiKeyID, err = parseOptionalUUID(c, "api_key_id"); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid api_key_id"))
		return
	}
	if params.ProviderID, err = parseOptionalUUID(c, "provider_id"); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid provider_id"))
		return
	}
	if params.ModelID, err = parseOptionalUUID(c, "model_id"); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid model_id"))
		return
	}

	logs, err := h.queries.ListRequestLogs(c.Request.Context(), params)
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	items := make([]requestLogResponse, 0, len(logs))
	for _, item := range logs {
		items = append(items, newRequestLogResponse(item))
	}
	httpapi.RespondJSON(c, http.StatusOK, listRequestLogsResponse{Items: items})
}

type timeWindow struct {
	from time.Time
	to   time.Time
}

func (h *RequestLogHandler) parseWindow(c *gin.Context) (timeWindow, bool) {
	to := h.now()
	from := to.Add(-defaultWindow)
	var err error
	if value := c.Query("from"); value != "" {
		from, err = time.Parse(time.RFC3339, value)
		if err != nil {
			httpapi.RespondError(c, httpapi.InvalidRequest("invalid from"))
			return timeWindow{}, false
		}
	}
	if value := c.Query("to"); value != "" {
		to, err = time.Parse(time.RFC3339, value)
		if err != nil {
			httpapi.RespondError(c, httpapi.InvalidRequest("invalid to"))
			return timeWindow{}, false
		}
	}
	if !from.Before(to) {
		httpapi.RespondError(c, httpapi.InvalidRequest("from must be before to"))
		return timeWindow{}, false
	}
	return timeWindow{from: from.UTC(), to: to.UTC()}, true
}

func parseBoundedInt32(c *gin.Context, name string, fallback int32, max int32) (int32, bool) {
	value := c.Query(name)
	if value == "" {
		return fallback, true
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 || parsed > int64(max) {
		httpapi.RespondError(c, httpapi.InvalidRequest(name+" is invalid"))
		return 0, false
	}
	return int32(parsed), true
}

func parseOptionalUUID(c *gin.Context, name string) (*uuid.UUID, error) {
	value := c.Query(name)
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func pgText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func newRequestLogResponse(item db.RequestLog) requestLogResponse {
	return requestLogResponse{
		ID:                item.ID,
		RequestModel:      textPtr(item.RequestModel),
		Status:            item.Status,
		StatusCode:        item.StatusCode,
		ErrorCode:         textPtr(item.ErrorCode),
		LatencyMS:         item.LatencyMs,
		ProviderLatencyMS: int4Ptr(item.ProviderLatencyMs),
		StartedAt:         item.StartedAt,
		CompletedAt:       item.CompletedAt,
		APIKeyID:          item.ApiKeyID,
		ProviderID:        item.ProviderID,
		ModelID:           item.ModelID,
	}
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func int4Ptr(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}
