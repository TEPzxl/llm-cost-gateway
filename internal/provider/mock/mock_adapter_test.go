package mock

import (
	"context"
	"testing"
	"time"

	"github.com/tep/llm-cost-gateway/internal/domain"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
)

func TestAdapterReturnsDeterministicResponseAndUsage(t *testing.T) {
	adapter := NewAdapter()

	response, err := adapter.Chat(context.Background(), contract.ChatRequest{
		RequestID: "req_123",
		Provider: contract.ProviderConfig{
			Name: "mock-provider",
			Type: "mock",
		},
		Model: contract.ModelConfig{
			ProviderModelName: "mock-small",
		},
		Messages: []contract.ChatMessage{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	if response.ProviderResponseID != "mock_req_123" {
		t.Fatalf("provider response id = %q, want mock_req_123", response.ProviderResponseID)
	}
	if response.Content != "Mock response" {
		t.Fatalf("content = %q, want Mock response", response.Content)
	}
	if response.Usage.PromptTokens != 20 || response.Usage.CompletionTokens != 30 || response.Usage.TotalTokens != 50 {
		t.Fatalf("usage = %+v, want 20/30/50", response.Usage)
	}
	if response.FinishReason != "stop" {
		t.Fatalf("finish reason = %q, want stop", response.FinishReason)
	}
}

func TestAdapterRejectsStream(t *testing.T) {
	adapter := NewAdapter()

	_, err := adapter.Chat(context.Background(), contract.ChatRequest{Stream: true})
	if contract.ErrorCode(err) != domain.CodeStreamNotSupported {
		t.Fatalf("Chat error code = %q, want %q", contract.ErrorCode(err), domain.CodeStreamNotSupported)
	}
}

func TestAdapterCanSimulateErrorAndDelay(t *testing.T) {
	expectedErr := contract.NewError(domain.CodeProviderError, "forced provider error", nil)
	adapter := NewAdapter(WithError(expectedErr), WithDelay(20*time.Millisecond))

	startedAt := time.Now()
	_, err := adapter.Chat(context.Background(), contract.ChatRequest{})

	if err != expectedErr {
		t.Fatalf("Chat error = %v, want forced error", err)
	}
	if elapsed := time.Since(startedAt); elapsed < 20*time.Millisecond {
		t.Fatalf("delay = %s, want at least 20ms", elapsed)
	}
}
