package openai_compatible

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/tep/llm-cost-gateway/internal/domain"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
)

type Adapter struct {
	client *http.Client
}

type Option func(*Adapter)

func NewAdapter(options ...Option) *Adapter {
	adapter := &Adapter{
		client: &http.Client{},
	}
	for _, option := range options {
		option(adapter)
	}
	return adapter
}

func WithHTTPClient(client *http.Client) Option {
	return func(a *Adapter) {
		if client != nil {
			a.client = client
		}
	}
}

func (a *Adapter) Chat(ctx context.Context, req contract.ChatRequest) (*contract.ChatResponse, error) {
	if req.Stream {
		return nil, contract.NewError(domain.CodeStreamNotSupported, "stream is not supported in v0.1", nil)
	}
	if strings.TrimSpace(req.Provider.BaseURL) == "" {
		return nil, contract.NewError(domain.CodeProviderError, "provider base_url is required", nil)
	}
	if strings.TrimSpace(req.Provider.APIKey) == "" {
		return nil, contract.NewError(domain.CodeProviderError, "provider api key is required", nil)
	}

	timeoutMS := req.Provider.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 30000
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	startedAt := time.Now()
	body, err := json.Marshal(newUpstreamRequest(req))
	if err != nil {
		return nil, contract.NewError(domain.CodeProviderError, "encode upstream request", err)
	}

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, upstreamChatCompletionsURL(req.Provider.BaseURL), bytes.NewReader(body))
	if err != nil {
		return nil, contract.NewError(domain.CodeProviderError, "build upstream request", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Provider.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	httpResp, err := a.client.Do(httpReq)
	if err != nil {
		if isTimeoutError(callCtx, err) {
			return nil, contract.NewError(domain.CodeProviderTimeout, "provider request timed out", err)
		}
		return nil, contract.NewError(domain.CodeProviderUnavailable, "provider request failed", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, upstreamStatusError(httpResp.StatusCode)
	}

	var upstream upstreamResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&upstream); err != nil {
		return nil, contract.NewError(domain.CodeProviderError, "decode upstream response", err)
	}
	if upstream.Usage == nil ||
		upstream.Usage.PromptTokens == nil ||
		upstream.Usage.CompletionTokens == nil ||
		upstream.Usage.TotalTokens == nil {
		return nil, contract.NewError(domain.CodeUsageMissing, "provider response missing usage", nil)
	}
	if len(upstream.Choices) == 0 {
		return nil, contract.NewError(domain.CodeProviderError, "provider response missing choices", nil)
	}

	choice := upstream.Choices[0]
	return &contract.ChatResponse{
		ProviderResponseID: upstream.ID,
		Content:            choice.Message.Content,
		Role:               choice.Message.Role,
		FinishReason:       choice.FinishReason,
		Usage: contract.Usage{
			PromptTokens:     *upstream.Usage.PromptTokens,
			CompletionTokens: *upstream.Usage.CompletionTokens,
			TotalTokens:      *upstream.Usage.TotalTokens,
		},
		LatencyMS: time.Since(startedAt).Milliseconds(),
		RawUsage: map[string]any{
			"prompt_tokens":     *upstream.Usage.PromptTokens,
			"completion_tokens": *upstream.Usage.CompletionTokens,
			"total_tokens":      *upstream.Usage.TotalTokens,
		},
	}, nil
}

type upstreamRequest struct {
	Model       string                 `json:"model"`
	Messages    []contract.ChatMessage `json:"messages"`
	Temperature *float64               `json:"temperature,omitempty"`
	MaxTokens   *int                   `json:"max_tokens,omitempty"`
	Stream      bool                   `json:"stream"`
}

type upstreamResponse struct {
	ID      string           `json:"id"`
	Choices []upstreamChoice `json:"choices"`
	Usage   *upstreamUsage   `json:"usage"`
}

type upstreamChoice struct {
	Index        int             `json:"index"`
	Message      upstreamMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type upstreamMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type upstreamUsage struct {
	PromptTokens     *int `json:"prompt_tokens"`
	CompletionTokens *int `json:"completion_tokens"`
	TotalTokens      *int `json:"total_tokens"`
}

func newUpstreamRequest(req contract.ChatRequest) upstreamRequest {
	return upstreamRequest{
		Model:       req.Model.ProviderModelName,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}
}

func upstreamChatCompletionsURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/chat/completions"
}

func upstreamStatusError(statusCode int) error {
	code := domain.CodeProviderError
	if statusCode >= 500 {
		code = domain.CodeProviderUnavailable
	}
	return contract.NewStatusError(
		code,
		fmt.Sprintf("provider returned status %d", statusCode),
		statusCode,
		statusCode == http.StatusTooManyRequests || statusCode >= 500,
		nil,
	)
}

func isTimeoutError(ctx context.Context, err error) bool {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
