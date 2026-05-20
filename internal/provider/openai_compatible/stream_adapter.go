package openai_compatible

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tep/llm-cost-gateway/internal/domain"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
)

func (a *Adapter) StreamChat(ctx context.Context, req contract.ChatRequest) (contract.ChatStream, error) {
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

	body, err := json.Marshal(newStreamingUpstreamRequest(req))
	if err != nil {
		cancel()
		return nil, contract.NewError(domain.CodeProviderError, "encode upstream request", err)
	}

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, upstreamChatCompletionsURL(req.Provider.BaseURL), bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, contract.NewError(domain.CodeProviderError, "build upstream request", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.Provider.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	httpResp, err := a.client.Do(httpReq)
	if err != nil {
		cancel()
		if isTimeoutError(callCtx, err) {
			return nil, contract.NewError(domain.CodeProviderTimeout, "provider request timed out", err)
		}
		return nil, contract.NewError(domain.CodeProviderUnavailable, "provider request failed", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		_ = httpResp.Body.Close()
		cancel()
		return nil, upstreamStatusError(httpResp.StatusCode)
	}

	stream := &openAIStream{
		body:   httpResp.Body,
		cancel: cancel,
		events: make(chan contract.StreamEvent),
	}
	go stream.read(callCtx)
	return stream, nil
}

type openAIStream struct {
	body   io.ReadCloser
	cancel context.CancelFunc
	events chan contract.StreamEvent
	err    error
}

func (s *openAIStream) Events() <-chan contract.StreamEvent {
	return s.events
}

func (s *openAIStream) Err() error {
	return s.err
}

func (s *openAIStream) Close() error {
	s.cancel()
	return s.body.Close()
}

func (s *openAIStream) read(ctx context.Context) {
	defer close(s.events)
	defer s.cancel()
	defer s.body.Close()

	reader := bufio.NewReader(s.body)
	var dataLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if len(line) > 0 {
				dataLines = appendDataLine(dataLines, line)
			}
			if len(dataLines) > 0 {
				s.emitData(strings.Join(dataLines, "\n"))
			}
			if err != io.EOF && ctx.Err() == nil {
				s.err = contract.NewError(domain.CodeProviderError, "read provider stream", err)
			}
			return
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if len(dataLines) > 0 && s.emitData(strings.Join(dataLines, "\n")) {
				return
			}
			dataLines = nil
			continue
		}
		dataLines = appendDataLine(dataLines, trimmed)
	}
}

func appendDataLine(lines []string, line string) []string {
	if strings.HasPrefix(line, "data:") {
		return append(lines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
	}
	return lines
}

func (s *openAIStream) emitData(data string) bool {
	event := contract.StreamEvent{
		Data: data,
		Done: data == "[DONE]",
	}
	if usage := parseStreamUsage(data); usage != nil {
		event.Usage = usage
	}
	s.events <- event
	return event.Done
}

func parseStreamUsage(data string) *contract.StreamUsage {
	if data == "" || data == "[DONE]" {
		return nil
	}
	var payload struct {
		Usage *struct {
			PromptTokens     *int `json:"prompt_tokens"`
			CompletionTokens *int `json:"completion_tokens"`
			TotalTokens      *int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil || payload.Usage == nil {
		return nil
	}
	if payload.Usage.PromptTokens == nil || payload.Usage.CompletionTokens == nil || payload.Usage.TotalTokens == nil {
		return nil
	}
	return &contract.StreamUsage{
		PromptTokens:     *payload.Usage.PromptTokens,
		CompletionTokens: *payload.Usage.CompletionTokens,
		TotalTokens:      *payload.Usage.TotalTokens,
		Raw: map[string]any{
			"prompt_tokens":     *payload.Usage.PromptTokens,
			"completion_tokens": *payload.Usage.CompletionTokens,
			"total_tokens":      *payload.Usage.TotalTokens,
		},
	}
}

func newStreamingUpstreamRequest(req contract.ChatRequest) upstreamRequest {
	upstream := newUpstreamRequest(req)
	upstream.Stream = true
	return upstream
}
