package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tep/llm-cost-gateway/internal/config"
	"github.com/tep/llm-cost-gateway/internal/events"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	"github.com/tep/llm-cost-gateway/internal/observability"
	"github.com/tep/llm-cost-gateway/internal/ratelimit"
	"github.com/tep/llm-cost-gateway/internal/store"
	"go.uber.org/zap"
)

type App struct {
	config Config
	logger *zap.Logger
	server *http.Server
	store  *store.Store
	redis  *redis.Client
	events io.Closer
}

type Config = config.Config

func New(ctx context.Context, cfg Config, logger *zap.Logger) (*App, error) {
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	redisClient, err := ratelimit.OpenRedis(ctx, cfg.RedisURL)
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("open redis: %w", err)
	}

	publisher, publisherCloser, err := newUsageEventPublisher(cfg)
	if err != nil {
		_ = redisClient.Close()
		st.Close()
		return nil, fmt.Errorf("init usage event publisher: %w", err)
	}

	return newWithDependencies(cfg, logger, st, redisClient, ratelimit.NewRedisLimiter(redisClient), publisher, publisherCloser), nil
}

func NewWithStore(cfg Config, logger *zap.Logger, st *store.Store) *App {
	return newWithDependencies(cfg, logger, st, nil, nil, events.DisabledPublisher{}, nil)
}

func newWithDependencies(cfg Config, logger *zap.Logger, st *store.Store, redisClient *redis.Client, limiter middleware.RateLimiter, usageEventPublisher events.Publisher, usageEventCloser io.Closer) *App {
	router := NewRouter(RouterConfig{
		AppEnv:                 cfg.AppEnv,
		PlatformBootstrapToken: cfg.PlatformBootstrapToken,
		TokenHashSecret:        cfg.TokenHashSecret,
		SecretEncryptionKey:    cfg.SecretEncryptionKey,
		MaxRetries:             cfg.MaxRetries,
		RetryBackoffMS:         cfg.RetryBackoffMS,
		Store:                  st,
		RateLimiter:            limiter,
		Metrics:                observability.NewMetrics(),
		UsageEventPublisher:    usageEventPublisher,
	}, logger)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &App{
		config: cfg,
		logger: logger,
		server: server,
		store:  st,
		redis:  redisClient,
		events: usageEventCloser,
	}
}

func newUsageEventPublisher(cfg Config) (events.Publisher, io.Closer, error) {
	if !cfg.KafkaEnabled {
		return events.DisabledPublisher{}, nil, nil
	}
	producer, err := events.NewKafkaProducer(events.KafkaProducerConfig{
		Brokers: cfg.KafkaBrokers,
		Topic:   cfg.KafkaUsageTopic,
	})
	if err != nil {
		return nil, nil, err
	}
	return producer, producer, nil
}

func (a *App) Run() error {
	a.logger.Info("starting gateway", zap.Int("port", a.config.ServerPort))
	return a.server.ListenAndServe()
}

func (a *App) Shutdown(ctx context.Context) error {
	err := a.server.Shutdown(ctx)
	if a.store != nil {
		a.store.Close()
	}
	if a.redis != nil {
		_ = a.redis.Close()
	}
	if a.events != nil {
		_ = a.events.Close()
	}
	return err
}
