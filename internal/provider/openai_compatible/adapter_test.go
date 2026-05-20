package openai_compatible

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tep/llm-cost-gateway/internal/domain"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
)

func TestAdapterSendsOpenAICompatibleRequestAndParsesResponse(t *testing.T) {
	var capturedAuth string
	var capturedPath string
	var capturedModel string
	var capturedStream bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path

		var request struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		capturedModel = request.Model
		capturedStream = request.Stream
		if len(request.Messages) != 1 || request.Messages[0].Content != "hello" {
			t.Fatalf("messages = %+v, want one hello message", request.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_123",
			"choices":[{"index":0,"message":{"role":"assistant","content":"upstream response"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":11,"completion_tokens":13,"total_tokens":24}
		}`))
	}))
	defer server.Close()

	adapter := NewAdapter()
	response, err := adapter.Chat(context.Background(), contract.ChatRequest{
		Provider: contract.ProviderConfig{
			Type:      "openai_compatible",
			BaseURL:   server.URL + "/v1",
			APIKey:    "provider-secret-key",
			TimeoutMS: 1000,
		},
		Model: contract.ModelConfig{
			ProviderModelName: "gpt-test",
		},
		Messages: []contract.ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	if capturedAuth != "Bearer provider-secret-key" {
		t.Fatalf("Authorization = %q, want bearer API key", capturedAuth)
	}
	if capturedPath != "/v1/chat/completions" {
		t.Fatalf("upstream path = %q, want /v1/chat/completions", capturedPath)
	}
	if capturedModel != "gpt-test" {
		t.Fatalf("upstream model = %q, want gpt-test", capturedModel)
	}
	if capturedStream {
		t.Fatal("upstream stream = true, want false")
	}
	if response.ProviderResponseID != "chatcmpl_123" {
		t.Fatalf("provider response id = %q, want chatcmpl_123", response.ProviderResponseID)
	}
	if response.Content != "upstream response" {
		t.Fatalf("content = %q, want upstream response", response.Content)
	}
	if response.Usage.PromptTokens != 11 || response.Usage.CompletionTokens != 13 || response.Usage.TotalTokens != 24 {
		t.Fatalf("usage = %+v, want 11/13/24", response.Usage)
	}
}

func TestAdapterRejectsStream(t *testing.T) {
	adapter := NewAdapter()

	_, err := adapter.Chat(context.Background(), contract.ChatRequest{Stream: true})
	if contract.ErrorCode(err) != domain.CodeStreamNotSupported {
		t.Fatalf("Chat error code = %q, want %q", contract.ErrorCode(err), domain.CodeStreamNotSupported)
	}
}

func TestAdapterStandardizesUpstreamStatusErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantCode   string
	}{
		{name: "401", statusCode: http.StatusUnauthorized, wantCode: domain.CodeProviderError},
		{name: "429", statusCode: http.StatusTooManyRequests, wantCode: domain.CodeProviderError},
		{name: "500", statusCode: http.StatusInternalServerError, wantCode: domain.CodeProviderUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"error":{"message":"upstream error"}}`))
			}))
			defer server.Close()

			adapter := NewAdapter()
			_, err := adapter.Chat(context.Background(), validChatRequest(server.URL))
			if contract.ErrorCode(err) != tt.wantCode {
				t.Fatalf("Chat error code = %q, want %q", contract.ErrorCode(err), tt.wantCode)
			}
		})
	}
}

func TestAdapterReturnsTimeoutError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewAdapter()
	request := validChatRequest(server.URL)
	request.Provider.TimeoutMS = 1

	_, err := adapter.Chat(context.Background(), request)
	if contract.ErrorCode(err) != domain.CodeProviderTimeout {
		t.Fatalf("Chat error code = %q, want %q", contract.ErrorCode(err), domain.CodeProviderTimeout)
	}
}

func TestAdapterReturnsUsageMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_missing_usage",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	adapter := NewAdapter()
	_, err := adapter.Chat(context.Background(), validChatRequest(server.URL))
	if contract.ErrorCode(err) != domain.CodeUsageMissing {
		t.Fatalf("Chat error code = %q, want %q", contract.ErrorCode(err), domain.CodeUsageMissing)
	}
}

func validChatRequest(baseURL string) contract.ChatRequest {
	return contract.ChatRequest{
		Provider: contract.ProviderConfig{
			Type:      "openai_compatible",
			BaseURL:   baseURL,
			APIKey:    "provider-secret-key",
			TimeoutMS: 1000,
		},
		Model: contract.ModelConfig{
			ProviderModelName: "gpt-test",
		},
		Messages: []contract.ChatMessage{{Role: "user", Content: "hello"}},
	}
}
