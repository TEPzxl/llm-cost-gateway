package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/metering"
	"github.com/tep/llm-cost-gateway/internal/observability"
	"github.com/tep/llm-cost-gateway/internal/ratelimit"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type RateLimiter interface {
	Allow(context.Context, auth.APIKeyPrincipal) (ratelimit.Decision, error)
}

type RateLimitRecorder interface {
	RecordFailure(context.Context, metering.RecordFailureInput) (db.RequestLog, error)
}

type RateLimitOption func(*rateLimitConfig)

type rateLimitConfig struct {
	recorder RateLimitRecorder
	metrics  *observability.Metrics
}

func WithRateLimitRecorder(recorder RateLimitRecorder) RateLimitOption {
	return func(cfg *rateLimitConfig) {
		cfg.recorder = recorder
	}
}

func WithRateLimitMetrics(metrics *observability.Metrics) RateLimitOption {
	return func(cfg *rateLimitConfig) {
		cfg.metrics = metrics
	}
}

func RateLimit(limiter RateLimiter, opts ...RateLimitOption) gin.HandlerFunc {
	cfg := rateLimitConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(c *gin.Context) {
		startedAt := time.Now().UTC()
		principal, ok := APIKeyPrincipalFromContext(c)
		if !ok {
			abortUnauthorized(c)
			return
		}
		if limiter == nil {
			abortWithError(c, http.StatusServiceUnavailable, domain.CodeProviderUnavailable, "rate limit service unavailable")
			return
		}

		decision, err := limiter.Allow(c.Request.Context(), principal)
		if err != nil {
			recordRateLimitFailure(c, cfg.recorder, principal, metering.StatusError, http.StatusServiceUnavailable, domain.CodeProviderUnavailable, startedAt)
			abortWithError(c, http.StatusServiceUnavailable, domain.CodeProviderUnavailable, "rate limit service unavailable")
			return
		}
		if !decision.Allowed {
			recordRateLimitFailure(c, cfg.recorder, principal, metering.StatusRateLimited, http.StatusTooManyRequests, domain.CodeRateLimitExceeded, startedAt)
			cfg.metrics.IncRateLimited()
			abortWithError(c, http.StatusTooManyRequests, domain.CodeRateLimitExceeded, "rate limit exceeded")
			return
		}

		c.Next()
	}
}

func recordRateLimitFailure(c *gin.Context, recorder RateLimitRecorder, principal auth.APIKeyPrincipal, status string, statusCode int, errorCode string, startedAt time.Time) {
	if recorder == nil {
		return
	}

	requestID, err := uuid.Parse(RequestIDFromContext(c))
	if err != nil {
		requestID = uuid.New()
	}
	requestHash, requestModel := requestHashAndModel(c)
	_, _ = recorder.RecordFailure(c.Request.Context(), metering.RecordFailureInput{
		RequestID:    requestID,
		OrgID:        principal.OrgID,
		APIKeyID:     &principal.APIKeyID,
		Method:       c.Request.Method,
		Path:         c.Request.URL.Path,
		RequestModel: requestModel,
		Status:       status,
		StatusCode:   int32(statusCode),
		ErrorCode:    errorCode,
		RequestHash:  requestHash,
		LatencyMS:    int32(time.Since(startedAt).Milliseconds()),
		StartedAt:    startedAt,
		CompletedAt:  time.Now().UTC(),
	})
}

func requestHashAndModel(c *gin.Context) (string, string) {
	if c.Request.Body == nil {
		return "", ""
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		return "", ""
	}
	sum := sha256.Sum256(body)
	var parsed struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &parsed)
	return "sha256:" + hex.EncodeToString(sum[:]), parsed.Model
}
