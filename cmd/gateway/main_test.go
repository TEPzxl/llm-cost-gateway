package main

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/tep/llm-cost-gateway/internal/config"
	"go.uber.org/zap"
)

func TestRunShutsDownWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	gateway := newFakeGateway()

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
		done <- runWithFactory(ctx, cfg, zap.NewNop(), func(context.Context, config.Config, *zap.Logger) (gatewayServer, error) {
			return gateway, nil
		})
	}()

	select {
	case <-gateway.runStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("gateway Run was not called")
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not shut down after context cancellation")
	}

	select {
	case <-gateway.shutdownCalled:
	default:
		t.Fatal("gateway Shutdown was not called")
	}
}

type fakeGateway struct {
	runStarted     chan struct{}
	shutdownCalled chan struct{}
	once           sync.Once
}

func newFakeGateway() *fakeGateway {
	return &fakeGateway{
		runStarted:     make(chan struct{}),
		shutdownCalled: make(chan struct{}),
	}
}

func (g *fakeGateway) Run() error {
	close(g.runStarted)
	<-g.shutdownCalled
	return http.ErrServerClosed
}

func (g *fakeGateway) Shutdown(context.Context) error {
	g.once.Do(func() {
		close(g.shutdownCalled)
	})
	return nil
}
