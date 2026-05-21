package app

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/analytics"
	"github.com/tep/llm-cost-gateway/internal/anomaly"
	"github.com/tep/llm-cost-gateway/internal/audit"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/budget"
	promptcache "github.com/tep/llm-cost-gateway/internal/cache"
	"github.com/tep/llm-cost-gateway/internal/costing"
	secretcrypto "github.com/tep/llm-cost-gateway/internal/crypto"
	"github.com/tep/llm-cost-gateway/internal/email"
	"github.com/tep/llm-cost-gateway/internal/embedding"
	"github.com/tep/llm-cost-gateway/internal/events"
	gatewayservice "github.com/tep/llm-cost-gateway/internal/gateway"
	"github.com/tep/llm-cost-gateway/internal/http/handlers/admin"
	gatewayhandler "github.com/tep/llm-cost-gateway/internal/http/handlers/gateway"
	"github.com/tep/llm-cost-gateway/internal/http/handlers/health"
	"github.com/tep/llm-cost-gateway/internal/http/handlers/platform"
	"github.com/tep/llm-cost-gateway/internal/http/middleware"
	"github.com/tep/llm-cost-gateway/internal/metering"
	"github.com/tep/llm-cost-gateway/internal/observability"
	"github.com/tep/llm-cost-gateway/internal/policy"
	"github.com/tep/llm-cost-gateway/internal/provider"
	"github.com/tep/llm-cost-gateway/internal/routing"
	"github.com/tep/llm-cost-gateway/internal/store"
	"go.uber.org/zap"
)

const defaultMaxRequestBodyBytes = 8 << 20

type RouterConfig struct {
	AppEnv                   string
	PlatformBootstrapToken   string
	TokenHashSecret          string
	SecretEncryptionKey      string
	SecretKeyRing            *secretcrypto.SecretKeyRing
	MaxRetries               int
	RetryBackoffMS           int
	Store                    *store.Store
	RateLimiter              middleware.RateLimiter
	Metrics                  *observability.Metrics
	UsageEventPublisher      events.Publisher
	UsageAnalytics           *analytics.UsageAnalyticsService
	PromptCache              *promptcache.PromptCache
	SemanticCache            *promptcache.SemanticCache
	EmbeddingAdapter         embedding.Adapter
	SemanticCacheThreshold   float64
	SemanticCacheMaxTemp     float64
	PasswordlessEmailEnabled bool
	MagicLinkBaseURL         string
	MagicLinkTTL             time.Duration
	EmailSender              email.Sender
	Logger                   *zap.Logger
}

func NewRouter(cfg RouterConfig, logger *zap.Logger) *gin.Engine {
	gin.SetMode(ginMode(cfg.AppEnv))
	if logger == nil {
		logger = zap.NewNop()
	}
	cfg.Logger = logger

	router := gin.New()
	router.Use(middleware.RequestID())
	router.Use(middleware.RequestBodyLimit(defaultMaxRequestBodyBytes))
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
	if cfg.AppEnv != "production" {
		router.GET("/metrics", gin.WrapH(metrics.Handler()))
	}

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
		gatewayservice.WithSemanticCache(cfg.SemanticCache, cfg.EmbeddingAdapter, cfg.SemanticCacheThreshold, cfg.SemanticCacheMaxTemp),
		gatewayservice.WithSecretKeyRing(cfg.SecretKeyRing),
		gatewayservice.WithPublicOutboundOnly(cfg.AppEnv == "production"),
		gatewayservice.WithLogger(cfg.Logger),
	)
	chatHandler := gatewayhandler.NewChatCompletionsHandler(chatService)

	group := router.Group("/v1")
	group.Use(middleware.GatewayAPIKeyAuth(apiKeyService))
	group.Use(middleware.RequireAPIKeyScope(auth.APIKeyScopeChatCompletions))
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
	sessionService := auth.NewSessionService(cfg.Store.Queries, cfg.TokenHashSecret)
	var magicLinkService *auth.MagicLinkService
	if cfg.PasswordlessEmailEnabled && cfg.EmailSender != nil {
		magicLinkService = auth.NewMagicLinkService(cfg.Store, cfg.TokenHashSecret, cfg.EmailSender, auth.MagicLinkConfig{
			BaseURL: cfg.MagicLinkBaseURL,
			TTL:     cfg.MagicLinkTTL,
		})
	}
	apiKeyService := auth.NewAPIKeyService(cfg.Store.Queries, cfg.TokenHashSecret)
	publicOutboundOnly := cfg.AppEnv == "production"
	providerService := provider.NewService(cfg.Store, cfg.SecretEncryptionKey, provider.WithPublicOutboundOnly(publicOutboundOnly), provider.WithSecretKeyRing(cfg.SecretKeyRing))
	providerHealthService := provider.NewHealthService(cfg.Store, cfg.SecretEncryptionKey, provider.WithHealthPublicOutboundOnly(publicOutboundOnly), provider.WithHealthSecretKeyRing(cfg.SecretKeyRing))
	pricingService := costing.NewPricingService(cfg.Store)
	routePolicyService := routing.NewService(cfg.Store)
	budgetService := budget.NewService(cfg.Store.Queries)
	budgetAlertService := budget.NewAlertService(cfg.Store, budget.WithAlertPublicOutboundOnly(publicOutboundOnly))
	anomalyService := anomaly.NewService(cfg.Store)
	auditService := audit.NewAdminAuditService(cfg.Store.Queries)
	contentPolicyService := policy.NewService(cfg.Store)
	meHandler := admin.NewMeHandler(cfg.Store.Queries)
	apiKeyHandler := admin.NewAPIKeyHandler(apiKeyService)
	providerHandler := admin.NewProviderHandler(providerService)
	providerHealthHandler := admin.NewProviderHealthHandler(providerHealthService)
	modelHandler := admin.NewModelHandler(providerService, pricingService)
	routePolicyHandler := admin.NewRoutePolicyHandler(routePolicyService)
	budgetHandler := admin.NewBudgetHandler(budgetService)
	budgetAlertHandler := admin.NewBudgetAlertHandler(budgetAlertService)
	anomalyPolicyHandler := admin.NewAnomalyPolicyHandler(anomalyService)
	auditHandler := admin.NewAuditHandler(auditService)
	sessionHandler := admin.NewSessionHandler(sessionService, magicLinkService)
	membersHandler := admin.NewMembersHandler(sessionService)
	contentPolicyHandler := admin.NewContentPolicyHandler(contentPolicyService)
	cacheEventHandler := admin.NewCacheEventHandler(cfg.Store.Queries)
	requestLogHandler := admin.NewRequestLogHandler(cfg.Store.Queries)
	usageHandler := admin.NewUsageHandler(cfg.Store.Queries)
	analyticsHandler := admin.NewAnalyticsHandler(cfg.UsageAnalytics, cfg.Store.Queries)

	if cfg.AppEnv != "production" {
		publicGroup := router.Group("/api/v1/admin")
		publicGroup.POST("/sessions/passwordless-mock", sessionHandler.PasswordlessMockLogin)
	}
	if magicLinkService != nil {
		publicGroup := router.Group("/api/v1/admin")
		publicGroup.POST("/sessions/passwordless/request", sessionHandler.RequestMagicLink)
		publicGroup.POST("/sessions/passwordless/verify", sessionHandler.VerifyMagicLink)
	}

	group := router.Group("/api/v1/admin")
	group.Use(middleware.AdminAuth(adminTokenService, sessionService))
	group.Use(middleware.AdminAudit(auditService))
	group.GET("/me", meHandler.Get)

	membersGroup := group.Group("", middleware.RequireAdminPermission(middleware.CanManageMembers))
	membersGroup.POST("/members", membersHandler.Create)
	group.GET("/members", middleware.RequireAdminPermission(middleware.CanViewMembers), membersHandler.List)

	tokensGroup := group.Group("", middleware.RequireAdminPermission(middleware.CanManageTokens))
	tokensGroup.POST("/api-keys", apiKeyHandler.Create)
	tokensGroup.GET("/api-keys", apiKeyHandler.List)
	tokensGroup.POST("/api-keys/:api_key_id/revoke", apiKeyHandler.Revoke)

	configGroup := group.Group("", middleware.RequireAdminPermission(middleware.CanManageConfiguration))
	configGroup.POST("/providers", providerHandler.Create)
	configGroup.GET("/providers", providerHandler.List)
	configGroup.GET("/providers/health", providerHealthHandler.List)
	configGroup.POST("/providers/:provider_id/health-check", providerHealthHandler.Check)
	configGroup.POST("/models", modelHandler.Create)
	configGroup.GET("/models", modelHandler.List)
	configGroup.PATCH("/models/:model_id/pricing", modelHandler.UpdatePricing)
	configGroup.GET("/models/:model_id/pricing-versions", modelHandler.ListPricingVersions)
	configGroup.POST("/route-policies", routePolicyHandler.Create)
	configGroup.GET("/route-policies", routePolicyHandler.List)
	configGroup.POST("/budgets", budgetHandler.Create)
	configGroup.GET("/budgets", budgetHandler.List)
	configGroup.GET("/budgets/status", budgetHandler.Status)
	configGroup.POST("/budget-alerts", budgetAlertHandler.Create)
	configGroup.GET("/budget-alerts", budgetAlertHandler.List)
	configGroup.GET("/budget-alert-deliveries", budgetAlertHandler.ListDeliveries)
	configGroup.POST("/anomaly-policies", anomalyPolicyHandler.Create)
	configGroup.GET("/anomaly-policies", anomalyPolicyHandler.List)
	configGroup.POST("/content-policies", contentPolicyHandler.Create)
	configGroup.GET("/content-policies", contentPolicyHandler.List)

	viewGroup := group.Group("", middleware.RequireAdminPermission(middleware.CanView))
	viewGroup.GET("/audit-logs", auditHandler.List)
	viewGroup.GET("/cache-events", cacheEventHandler.List)
	viewGroup.GET("/request-logs", requestLogHandler.List)
	viewGroup.GET("/usage/summary", usageHandler.Summary)
	viewGroup.GET("/analytics/daily-cost", analyticsHandler.DailyCostTrend)
	viewGroup.GET("/analytics/model-cost-breakdown", analyticsHandler.ModelCostBreakdown)
	viewGroup.GET("/analytics/provider-latency", analyticsHandler.ProviderLatency)
	viewGroup.GET("/analytics/error-rate", analyticsHandler.ErrorRateTrend)
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
