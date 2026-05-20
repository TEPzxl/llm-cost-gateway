package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	gatewayservice "github.com/tep/llm-cost-gateway/internal/gateway"
	httpapi "github.com/tep/llm-cost-gateway/internal/http"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
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
	var probe struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		httpapi.RespondError(c, httpapi.InvalidRequest("invalid JSON body"))
		return
	}

	input := gatewayservice.ChatInput{
		RequestID: requestID,
		Principal: principal,
		Method:    c.Request.Method,
		Path:      c.Request.URL.Path,
		RawBody:   body,
	}
	if probe.Stream {
		h.createStream(c, input)
		return
	}

	result, err := h.service.Chat(c.Request.Context(), input)
	if err != nil {
		httpapi.RespondError(c, err)
		return
	}

	httpapi.RespondJSON(c, http.StatusOK, result.Response)
}

func (h *ChatCompletionsHandler) createStream(c *gin.Context, input gatewayservice.ChatInput) {
	result, err := h.service.StreamChat(c.Request.Context(), input)
	if err != nil {
		httpapi.RespondError(c, err)
		return
	}
	defer result.Stream.Close()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		httpapi.RespondError(c, httpapi.InternalError("streaming is not supported"))
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	c.Writer.WriteHeaderNow()

	var usage *contract.StreamUsage
	for event := range result.Stream.Events() {
		if event.Usage != nil {
			usage = event.Usage
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", event.Data); err != nil {
			break
		}
		flusher.Flush()
	}

	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
	defer cancel()
	_ = h.service.FinalizeStream(finalizeCtx, result, usage, result.Stream.Err())
}
