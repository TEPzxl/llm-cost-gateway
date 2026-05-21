package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type CacheEventHandler struct {
	queries *db.Queries
	now     func() time.Time
}

func NewCacheEventHandler(queries *db.Queries) *CacheEventHandler {
	return &CacheEventHandler{
		queries: queries,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

type cacheEventResponse struct {
	ID             uuid.UUID  `json:"id"`
	RequestLogID   *uuid.UUID `json:"request_log_id"`
	EventType      string     `json:"event_type"`
	RequestedModel string     `json:"requested_model"`
	CacheKeyHash   string     `json:"cache_key_hash"`
	MessagesHash   string     `json:"messages_hash"`
	Reason         *string    `json:"reason"`
	CreatedAt      time.Time  `json:"created_at"`
}

type listCacheEventsResponse struct {
	Items []cacheEventResponse `json:"items"`
}

func (h *CacheEventHandler) List(c *gin.Context) {
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

	events, err := h.queries.ListCacheEvents(c.Request.Context(), db.ListCacheEventsParams{
		OrgID:          principal.OrgID,
		FromAt:         window.from,
		ToAt:           window.to,
		EventType:      pgText(c.Query("event_type")),
		RequestedModel: pgText(c.Query("requested_model")),
		Limit:          limit,
	})
	if err != nil {
		httpapi.RespondError(c, httpapi.InternalError("internal server error"))
		return
	}
	items := make([]cacheEventResponse, 0, len(events))
	for _, item := range events {
		items = append(items, newCacheEventResponse(item))
	}
	httpapi.RespondJSON(c, http.StatusOK, listCacheEventsResponse{Items: items})
}

func (h *CacheEventHandler) parseWindow(c *gin.Context) (timeWindow, bool) {
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

func newCacheEventResponse(item db.CacheEvent) cacheEventResponse {
	return cacheEventResponse{
		ID:             item.ID,
		RequestLogID:   item.RequestLogID,
		EventType:      item.EventType,
		RequestedModel: item.RequestedModel,
		CacheKeyHash:   item.CacheKeyHash,
		MessagesHash:   item.MessagesHash,
		Reason:         textPtr(item.Reason),
		CreatedAt:      item.CreatedAt,
	}
}
