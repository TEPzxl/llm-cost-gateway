package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tep/llm-cost-gateway/internal/analytics"
	promptcache "github.com/tep/llm-cost-gateway/internal/cache"
	"github.com/tep/llm-cost-gateway/internal/config"
	"github.com/tep/llm-cost-gateway/internal/embedding"
	"github.com/tep/llm-cost-gateway/internal/events"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	"github.com/tep/llm-cost-gateway/internal/observability"
	"github.com/tep/llm-cost-gateway/internal/ratelimit"
	"github.com/tep/llm-cost-gateway/internal/store"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
)

type App struct {
	config Config
	logger *zap.Logger
	server *http.Server
	store  *store.Store
	redis  *redis.Client
	events io.Closer
	traces *sdktrace.TracerProvider
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
	tracerProvider, err := observability.NewTracerProvider(ctx, observability.TracingConfig{
		Enabled:     cfg.TracingEnabled,
		Endpoint:    cfg.TracingOTLPEndpoint,
		Insecure:    cfg.TracingInsecure,
		ServiceName: cfg.TracingServiceName,
	})
	if err != nil {
		_ = redisClient.Close()
		st.Close()
		return nil, fmt.Errorf("init tracing: %w", err)
	}

	publisher, publisherCloser, err := newUsageEventPublisher(cfg)
	if err != nil {
		if tracerProvider != nil {
			_ = tracerProvider.Shutdown(ctx)
		}
		_ = redisClient.Close()
		st.Close()
		return nil, fmt.Errorf("init usage event publisher: %w", err)
	}
	usageAnalytics, analyticsCloser, err := newUsageAnalyticsService(ctx, cfg, st)
	if err != nil {
		if tracerProvider != nil {
			_ = tracerProvider.Shutdown(ctx)
		}
		if publisherCloser != nil {
			_ = publisherCloser.Close()
		}
		_ = redisClient.Close()
		st.Close()
		return nil, fmt.Errorf("init usage analytics: %w", err)
	}

	return newWithDependencies(cfg, logger, st, redisClient, ratelimit.NewRedisLimiter(redisClient), publisher, publisherCloser, usageAnalytics, analyticsCloser, tracerProvider), nil
}

func NewWithStore(cfg Config, logger *zap.Logger, st *store.Store) *App {
	return newWithDependencies(cfg, logger, st, nil, nil, events.DisabledPublisher{}, nil, analytics.NewUsageAnalyticsService(st, nil, false), nil, nil)
}

func newWithDependencies(cfg Config, logger *zap.Logger, st *store.Store, redisClient *redis.Client, limiter middleware.RateLimiter, usageEventPublisher events.Publisher, usageEventCloser io.Closer, usageAnalytics *analytics.UsageAnalyticsService, analyticsCloser io.Closer, tracerProvider *sdktrace.TracerProvider) *App {
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
		UsageAnalytics:         usageAnalytics,
		PromptCache:            newPromptCache(cfg, redisClient),
		SemanticCache:          newSemanticCache(cfg, redisClient),
		EmbeddingAdapter:       embedding.NewMockAdapter(),
		SemanticCacheThreshold: cfg.SemanticCacheThreshold,
		SemanticCacheMaxTemp:   cfg.SemanticCacheMaxTemp,
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
		events: multiCloser(usageEventCloser, analyticsCloser),
		traces: tracerProvider,
	}
}

func newPromptCache(cfg Config, redisClient redis.Cmdable) *promptcache.PromptCache {
	if !cfg.PromptCacheEnabled || redisClient == nil {
		return nil
	}
	return promptcache.NewPromptCache(redisClient, time.Duration(cfg.PromptCacheTTLSeconds)*time.Second)
}

func newSemanticCache(cfg Config, redisClient redis.Cmdable) *promptcache.SemanticCache {
	if !cfg.SemanticCacheEnabled || redisClient == nil {
		return nil
	}
	return promptcache.NewSemanticCache(redisClient, time.Duration(cfg.PromptCacheTTLSeconds)*time.Second)
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

func newUsageAnalyticsService(ctx context.Context, cfg Config, st *store.Store) (*analytics.UsageAnalyticsService, io.Closer, error) {
	if !cfg.ClickHouseEnabled {
		return analytics.NewUsageAnalyticsService(st, nil, false), nil, nil
	}
	client, err := analytics.NewClickHouseClient(analytics.ClickHouseConfig{
		URL:      cfg.ClickHouseURL,
		Database: cfg.ClickHouseDatabase,
		Username: cfg.ClickHouseUsername,
		Password: cfg.ClickHousePassword,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := client.Ping(ctx); err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return analytics.NewUsageAnalyticsService(st, client, true), client, nil
}

func multiCloser(closers ...io.Closer) io.Closer {
	return closerFunc(func() error {
		for _, closer := range closers {
			if closer != nil {
				_ = closer.Close()
			}
		}
		return nil
	})
}

type closerFunc func() error

func (f closerFunc) Close() error {
	return f()
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
	if a.traces != nil {
		_ = a.traces.Shutdown(ctx)
	}
	return err
}
