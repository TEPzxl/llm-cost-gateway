package main

import (
	"context"
	"testing"
	"time"

	"github.com/tep/llm-cost-gateway/internal/config"
	"go.uber.org/zap"
)

func TestRunShutsDownWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	cfg := config.Config{
		AppEnv:                 "test",
		ServerPort:             0,
		DatabaseURL:            "postgres://llmgw:llmgw@localhost:5432/llmgw?sslmode=disable",
		RedisURL:               "redis://localhost:6379/0",
		PlatformBootstrapToken: "bootstrap-token",
		TokenHashSecret:        "token-hash-secret",
		SecretEncryptionKey:    "0123456789abcdef0123456789abcdef",
		LogLevel:               "info",
	}

	go func() {
		done <- run(ctx, cfg, zap.NewNop())
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not shut down after context cancellation")
	}
}
