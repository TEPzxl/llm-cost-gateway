package openai_compatible

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
)

func TestLiveOpenAICompatibleProvider(t *testing.T) {
	if os.Getenv("LIVE_PROVIDER_TESTS_ENABLED") != "true" {
		t.Skip("set LIVE_PROVIDER_TESTS_ENABLED=true to run live provider integration tests")
	}

	cfg := liveProviderConfigFromEnv(t)
	adapter := NewAdapter()

	t.Run("chat completion returns usage", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.timeoutMS)*time.Millisecond+5*time.Second)
		defer cancel()

		request := cfg.chatRequest(false)
		response, err := adapter.Chat(ctx, request)
		if err != nil {
			t.Fatalf("Chat returned error: %v", err)
		}
		if strings.TrimSpace(response.ProviderResponseID) == "" {
			t.Fatal("provider response id is empty")
		}
		if response.Usage.TotalTokens <= 0 {
			t.Fatalf("total tokens = %d, want > 0", response.Usage.TotalTokens)
		}
		if response.Usage.PromptTokens < 0 || response.Usage.CompletionTokens < 0 {
			t.Fatalf("usage has negative fields: %+v", response.Usage)
		}
	})

	t.Run("stream chat receives data and done event", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.timeoutMS)*time.Millisecond+5*time.Second)
		defer cancel()

		request := cfg.chatRequest(true)
		stream, err := adapter.StreamChat(ctx, request)
		if err != nil {
			t.Fatalf("StreamChat returned error: %v", err)
		}
		defer stream.Close()

		var dataEvents int
		var done bool
		for event := range stream.Events() {
			if event.Data != "" && !event.Done {
				dataEvents++
			}
			if event.Done {
				done = true
			}
		}
		if err := stream.Err(); err != nil {
			t.Fatalf("stream error = %v", err)
		}
		if dataEvents == 0 {
			t.Fatal("stream produced no data events")
		}
		if !done {
			t.Fatal("stream did not produce a done event")
		}
	})
}

type liveProviderConfig struct {
	baseURL   string
	apiKey    string
	model     string
	timeoutMS int
	maxTokens int
}

func liveProviderConfigFromEnv(t *testing.T) liveProviderConfig {
	t.Helper()

	cfg := liveProviderConfig{
		baseURL:   strings.TrimRight(strings.TrimSpace(os.Getenv("LIVE_PROVIDER_BASE_URL")), "/"),
		apiKey:    strings.TrimSpace(os.Getenv("LIVE_PROVIDER_API_KEY")),
		model:     strings.TrimSpace(os.Getenv("LIVE_PROVIDER_MODEL")),
		timeoutMS: liveProviderIntEnv(t, "LIVE_PROVIDER_TIMEOUT_MS", 30000),
		maxTokens: liveProviderIntEnv(t, "LIVE_PROVIDER_MAX_TOKENS", 16),
	}
	if cfg.baseURL == "" {
		t.Fatal("LIVE_PROVIDER_BASE_URL is required")
	}
	if cfg.apiKey == "" {
		t.Fatal("LIVE_PROVIDER_API_KEY is required")
	}
	if cfg.model == "" {
		t.Fatal("LIVE_PROVIDER_MODEL is required")
	}
	if cfg.timeoutMS <= 0 {
		t.Fatal("LIVE_PROVIDER_TIMEOUT_MS must be positive")
	}
	if cfg.maxTokens <= 0 {
		t.Fatal("LIVE_PROVIDER_MAX_TOKENS must be positive")
	}
	return cfg
}

func liveProviderIntEnv(t *testing.T, name string, fallback int) int {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("%s must be an integer", name)
	}
	return parsed
}

func (cfg liveProviderConfig) chatRequest(stream bool) contract.ChatRequest {
	maxTokens := cfg.maxTokens
	return contract.ChatRequest{
		Provider: contract.ProviderConfig{
			Type:      "openai_compatible",
			BaseURL:   cfg.baseURL,
			APIKey:    cfg.apiKey,
			TimeoutMS: int32(cfg.timeoutMS),
		},
		Model: contract.ModelConfig{
			ProviderModelName: cfg.model,
		},
		Messages: []contract.ChatMessage{
			{Role: "user", Content: "Reply with exactly: live-provider-ok"},
		},
		MaxTokens: &maxTokens,
		Stream:    stream,
	}
}
