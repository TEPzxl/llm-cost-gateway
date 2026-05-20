package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/tep/llm-cost-gateway/internal/config"
	"go.uber.org/zap"
)

type App struct {
	config Config
	logger *zap.Logger
	server *http.Server
}

type Config = config.Config

func New(cfg Config, logger *zap.Logger) *App {
	router := NewRouter(cfg.AppEnv)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &App{
		config: cfg,
		logger: logger,
		server: server,
	}
}

func (a *App) Run() error {
	a.logger.Info("starting gateway", zap.Int("port", a.config.ServerPort))
	return a.server.ListenAndServe()
}

func (a *App) Shutdown(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}
