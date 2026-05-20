package gateway

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	gatewayservice "github.com/tep/llm-cost-gateway/internal/gateway"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
)

type ChatCompletionsHandler struct {
	service *gatewayservice.ChatService
}

func NewChatCompletionsHandler(service *gatewayservice.ChatService) *ChatCompletionsHandler {
	return &ChatCompletionsHandler{service: service}
}

func (h *ChatCompletionsHandler) Create(c *gin.Context) {
	principal, ok := middleware.APIKeyPrincipalFromContext(c)
	if !ok {
		httpapi.RespondError(c, httpapi.Unauthorized("unauthorized"))
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid request body"))
		return
	}
	requestID, err := uuid.Parse(middleware.RequestIDFromContext(c))
	if err != nil {
		requestID = uuid.New()
	}

	result, err := h.service.Chat(c.Request.Context(), gatewayservice.ChatInput{
		RequestID: requestID,
		Principal: principal,
		Method:    c.Request.Method,
		Path:      c.Request.URL.Path,
		RawBody:   body,
	})
	if err != nil {
		httpapi.RespondError(c, err)
		return
	}

	httpapi.RespondJSON(c, http.StatusOK, result.Response)
}
