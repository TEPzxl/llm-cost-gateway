package app

import (
	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/analytics"
	"github.com/tep/llm-cost-gateway/internal/audit"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/budget"
	promptcache "github.com/tep/llm-cost-gateway/internal/cache"
	"github.com/tep/llm-cost-gateway/internal/costing"
	"github.com/tep/llm-cost-gateway/internal/events"
	gatewayservice "github.com/tep/llm-cost-gateway/internal/gateway"
	"github.com/tep/llm-cost-gateway/internal/http/handlers/admin"
	gatewayhandler "github.com/tep/llm-cost-gateway/internal/http/handlers/gateway"
	"github.com/tep/llm-cost-gateway/internal/http/handlers/health"
	"github.com/tep/llm-cost-gateway/internal/http/handlers/platform"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	"github.com/tep/llm-cost-gateway/internal/metering"
	"github.com/tep/llm-cost-gateway/internal/observability"
	"github.com/tep/llm-cost-gateway/internal/provider"
	"github.com/tep/llm-cost-gateway/internal/routing"
	"github.com/tep/llm-cost-gateway/internal/store"
	"go.uber.org/zap"
)

type RouterConfig struct {
	AppEnv                 string
	PlatformBootstrapToken string
	TokenHashSecret        string
	SecretEncryptionKey    string
	MaxRetries             int
	RetryBackoffMS         int
	Store                  *store.Store
	RateLimiter            middleware.RateLimiter
	Metrics                *observability.Metrics
	UsageEventPublisher    events.Publisher
	UsageAnalytics         *analytics.UsageAnalyticsService
	PromptCache            *promptcache.PromptCache
	Logger                 *zap.Logger
}

func NewRouter(cfg RouterConfig, logger *zap.Logger) *gin.Engine {
	gin.SetMode(ginMode(cfg.AppEnv))
	if logger == nil {
		logger = zap.NewNop()
	}
	cfg.Logger = logger

	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.Logging(logger))
	router.Use(gin.Recovery())
	router.GET("/healthz", health.Health)
	metrics := cfg.Metrics
	if metrics == nil {
		metrics = observability.NewMetrics()
	}
	cfg.Metrics = metrics
	if cfg.Store != nil && cfg.UsageAnalytics == nil {
		cfg.UsageAnalytics = analytics.NewUsageAnalyticsService(cfg.Store, nil, false)
	}
	router.GET("/metrics", gin.WrapH(metrics.Handler()))

	if cfg.Store != nil {
		registerPlatformRoutes(router, cfg)
		registerAdminRoutes(router, cfg)
		if cfg.RateLimiter != nil {
			registerGatewayRoutes(router, cfg)
		}
	}

	return router
}

func registerGatewayRoutes(router *gin.Engine, cfg RouterConfig) {
	apiKeyService := auth.NewAPIKeyService(cfg.Store.Queries, cfg.TokenHashSecret)
	meteringService := metering.NewService(
		cfg.Store,
		costing.NewCalculator(),
		metering.WithUsageAnalyticsSink(cfg.UsageAnalytics),
		metering.WithLogger(cfg.Logger),
		metering.WithMetrics(cfg.Metrics),
	)
	chatService := gatewayservice.NewChatService(
		cfg.Store,
		cfg.SecretEncryptionKey,
		cfg.Metrics,
		gatewayservice.NewRetryPolicy(cfg.MaxRetries, cfg.RetryBackoffMS),
		gatewayservice.WithUsageEventPublisher(cfg.UsageEventPublisher),
		gatewayservice.WithUsageAnalyticsSink(cfg.UsageAnalytics),
		gatewayservice.WithPromptCache(cfg.PromptCache),
		gatewayservice.WithLogger(cfg.Logger),
	)
	chatHandler := gatewayhandler.NewChatCompletionsHandler(chatService)

	group := router.Group("/v1")
	group.Use(middleware.GatewayAPIKeyAuth(apiKeyService))
	group.Use(middleware.RateLimit(cfg.RateLimiter, middleware.WithRateLimitRecorder(meteringService), middleware.WithRateLimitMetrics(cfg.Metrics)))
	group.POST("/chat/completions", chatHandler.Create)
}

func registerPlatformRoutes(router *gin.Engine, cfg RouterConfig) {
	adminTokenService := auth.NewAdminTokenService(cfg.Store.Queries, cfg.TokenHashSecret)
	orgHandler := platform.NewOrgHandler(cfg.Store.Queries)
	adminTokenHandler := platform.NewAdminTokenHandler(adminTokenService)

	group := router.Group("/api/v1/platform")
	group.Use(middleware.PlatformAuth(cfg.PlatformBootstrapToken))
	group.POST("/orgs", orgHandler.Create)
	group.POST("/orgs/:org_id/admin-tokens", adminTokenHandler.Create)
}

func registerAdminRoutes(router *gin.Engine, cfg RouterConfig) {
	adminTokenService := auth.NewAdminTokenService(cfg.Store.Queries, cfg.TokenHashSecret)
	apiKeyService := auth.NewAPIKeyService(cfg.Store.Queries, cfg.TokenHashSecret)
	providerService := provider.NewService(cfg.Store, cfg.SecretEncryptionKey)
	providerHealthService := provider.NewHealthService(cfg.Store, cfg.SecretEncryptionKey)
	pricingService := costing.NewPricingService(cfg.Store)
	routePolicyService := routing.NewService(cfg.Store)
	budgetService := budget.NewService(cfg.Store.Queries)
	budgetAlertService := budget.NewAlertService(cfg.Store)
	auditService := audit.NewAdminAuditService(cfg.Store.Queries)
	meHandler := admin.NewMeHandler(cfg.Store.Queries)
	apiKeyHandler := admin.NewAPIKeyHandler(apiKeyService)
	providerHandler := admin.NewProviderHandler(providerService)
	providerHealthHandler := admin.NewProviderHealthHandler(providerHealthService)
	modelHandler := admin.NewModelHandler(providerService, pricingService)
	routePolicyHandler := admin.NewRoutePolicyHandler(routePolicyService)
	budgetHandler := admin.NewBudgetHandler(budgetService)
	budgetAlertHandler := admin.NewBudgetAlertHandler(budgetAlertService)
	auditHandler := admin.NewAuditHandler(auditService)
	cacheEventHandler := admin.NewCacheEventHandler(cfg.Store.Queries)
	requestLogHandler := admin.NewRequestLogHandler(cfg.Store.Queries)
	usageHandler := admin.NewUsageHandler(cfg.Store.Queries)
	analyticsHandler := admin.NewAnalyticsHandler(cfg.UsageAnalytics, cfg.Store.Queries)

	group := router.Group("/api/v1/admin")
	group.Use(middleware.AdminTokenAuth(adminTokenService))
	group.Use(middleware.AdminAudit(auditService))
	group.GET("/me", meHandler.Get)
	group.POST("/api-keys", apiKeyHandler.Create)
	group.GET("/api-keys", apiKeyHandler.List)
	group.POST("/api-keys/:api_key_id/revoke", apiKeyHandler.Revoke)
	group.POST("/providers", providerHandler.Create)
	group.GET("/providers", providerHandler.List)
	group.GET("/providers/health", providerHealthHandler.List)
	group.POST("/providers/:provider_id/health-check", providerHealthHandler.Check)
	group.POST("/models", modelHandler.Create)
	group.GET("/models", modelHandler.List)
	group.PATCH("/models/:model_id/pricing", modelHandler.UpdatePricing)
	group.GET("/models/:model_id/pricing-versions", modelHandler.ListPricingVersions)
	group.POST("/route-policies", routePolicyHandler.Create)
	group.GET("/route-policies", routePolicyHandler.List)
	group.POST("/budgets", budgetHandler.Create)
	group.GET("/budgets", budgetHandler.List)
	group.GET("/budgets/status", budgetHandler.Status)
	group.POST("/budget-alerts", budgetAlertHandler.Create)
	group.GET("/budget-alerts", budgetAlertHandler.List)
	group.GET("/budget-alert-deliveries", budgetAlertHandler.ListDeliveries)
	group.GET("/audit-logs", auditHandler.List)
	group.GET("/cache-events", cacheEventHandler.List)
	group.GET("/request-logs", requestLogHandler.List)
	group.GET("/usage/summary", usageHandler.Summary)
	group.GET("/analytics/daily-cost", analyticsHandler.DailyCostTrend)
	group.GET("/analytics/model-cost-breakdown", analyticsHandler.ModelCostBreakdown)
	group.GET("/analytics/provider-latency", analyticsHandler.ProviderLatency)
	group.GET("/analytics/error-rate", analyticsHandler.ErrorRateTrend)
}

func ginMode(appEnv string) string {
	switch appEnv {
	case "production":
		return gin.ReleaseMode
	case "test":
		return gin.TestMode
	default:
		return gin.DebugMode
	}
}
