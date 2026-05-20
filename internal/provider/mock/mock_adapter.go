package mock

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tep/llm-cost-gateway/internal/domain"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
)

const (
	mockPromptTokens     = 20
	mockCompletionTokens = 30
)

type Adapter struct {
	err   error
	delay time.Duration
}

type Option func(*Adapter)

func NewAdapter(options ...Option) *Adapter {
	adapter := &Adapter{}
	for _, option := range options {
		option(adapter)
	}
	return adapter
}

func WithError(err error) Option {
	return func(a *Adapter) {
		a.err = err
	}
}

func WithDelay(delay time.Duration) Option {
	return func(a *Adapter) {
		a.delay = delay
	}
}

func (a *Adapter) Chat(ctx context.Context, req contract.ChatRequest) (*contract.ChatResponse, error) {
	if req.Stream {
		return nil, contract.NewError(domain.CodeStreamNotSupported, "stream is not supported in v0.1", nil)
	}

	startedAt := time.Now()
	if a.delay > 0 {
		timer := time.NewTimer(a.delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, contract.NewError(domain.CodeProviderTimeout, "mock provider context cancelled", ctx.Err())
		case <-timer.C:
		}
	}
	if a.err != nil {
		return nil, a.err
	}

	responseID := "mock_response"
	if req.RequestID != "" {
		responseID = "mock_" + req.RequestID
	}
	return &contract.ChatResponse{
		ProviderResponseID: responseID,
		Content:            "Mock response",
		Role:               "assistant",
		FinishReason:       "stop",
		Usage: contract.Usage{
			PromptTokens:     mockPromptTokens,
			CompletionTokens: mockCompletionTokens,
			TotalTokens:      mockPromptTokens + mockCompletionTokens,
		},
		LatencyMS: time.Since(startedAt).Milliseconds(),
		RawUsage: map[string]any{
			"prompt_tokens":     mockPromptTokens,
			"completion_tokens": mockCompletionTokens,
			"total_tokens":      mockPromptTokens + mockCompletionTokens,
		},
	}, nil
}

func (a *Adapter) StreamChat(ctx context.Context, req contract.ChatRequest) (contract.ChatStream, error) {
	stream := &mockStream{events: make(chan contract.StreamEvent, 3)}
	go func() {
		defer close(stream.events)
		if a.delay > 0 {
			timer := time.NewTimer(a.delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				stream.err = contract.NewError(domain.CodeProviderTimeout, "mock provider context cancelled", ctx.Err())
				return
			case <-timer.C:
			}
		}
		if a.err != nil {
			stream.err = a.err
			return
		}

		stream.events <- contract.StreamEvent{Data: mustJSON(map[string]any{
			"id": "mock_" + req.RequestID,
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]string{
						"role":    "assistant",
						"content": "Mock response",
					},
				},
			},
		})}
		usage := &contract.StreamUsage{
			PromptTokens:     mockPromptTokens,
			CompletionTokens: mockCompletionTokens,
			TotalTokens:      mockPromptTokens + mockCompletionTokens,
			Raw: map[string]any{
				"prompt_tokens":     mockPromptTokens,
				"completion_tokens": mockCompletionTokens,
				"total_tokens":      mockPromptTokens + mockCompletionTokens,
			},
		}
		stream.events <- contract.StreamEvent{Data: mustJSON(map[string]any{
			"id":      "mock_" + req.RequestID,
			"choices": []map[string]any{},
			"usage":   usage.Raw,
		}), Usage: usage}
		stream.events <- contract.StreamEvent{Data: "[DONE]", Done: true}
	}()
	return stream, nil
}

type mockStream struct {
	events chan contract.StreamEvent
	err    error
}

func (s *mockStream) Events() <-chan contract.StreamEvent {
	return s.events
}

func (s *mockStream) Err() error {
	return s.err
}

func (s *mockStream) Close() error {
	return nil
}

func mustJSON(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(body)
}
