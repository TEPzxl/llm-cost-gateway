package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/tep/llm-cost-gateway/internal/app"
	"github.com/tep/llm-cost-gateway/internal/config"
	"github.com/tep/llm-cost-gateway/internal/observability"
	"go.uber.org/zap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	logger, err := observability.NewLogger(cfg.LogLevel, cfg.AppEnv)
	if err != nil {
		log.Fatalf("init logger: %v", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Fatal("gateway stopped", zap.Error(err))
	}
}

func run(ctx context.Context, cfg config.Config, logger *zap.Logger) error {
	return runWithFactory(ctx, cfg, logger, func(ctx context.Context, cfg config.Config, logger *zap.Logger) (gatewayServer, error) {
		return app.New(ctx, cfg, logger)
	})
}

type gatewayServer interface {
	Run() error
	Shutdown(ctx context.Context) error
}

type gatewayFactory func(ctx context.Context, cfg config.Config, logger *zap.Logger) (gatewayServer, error)

func runWithFactory(ctx context.Context, cfg config.Config, logger *zap.Logger, factory gatewayFactory) error {
	gateway, err := factory(ctx, cfg, logger)
	if err != nil {
		return err
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- gateway.Run()
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := gateway.Shutdown(shutdownCtx); err != nil {
			return err
		}

		select {
		case err := <-serverErr:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
	}
}
