package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/domain"
	"github.com/tep/llm-cost-gateway/internal/ratelimit"
)

type fakeLimiter struct {
	decision ratelimit.Decision
	err      error
}

func (f fakeLimiter) Allow(context.Context, auth.APIKeyPrincipal) (ratelimit.Decision, error) {
	return f.decision, f.err
}

func TestRateLimitAllowsUnderLimit(t *testing.T) {
	principal := auth.APIKeyPrincipal{
		OrgID:    uuid.New(),
		APIKeyID: uuid.New(),
		RPMLimit: 10,
	}
	router := rateLimitTestRouter(principal, fakeLimiter{
		decision: ratelimit.Decision{
			Allowed:   true,
			Limit:     10,
			Remaining: 9,
			ResetAt:   time.Now().Add(time.Minute),
		},
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
}

func TestRateLimitRejectsOverLimit(t *testing.T) {
	principal := auth.APIKeyPrincipal{
		OrgID:    uuid.New(),
		APIKeyID: uuid.New(),
		RPMLimit: 1,
	}
	router := rateLimitTestRouter(principal, fakeLimiter{
		decision: ratelimit.Decision{
			Allowed:   false,
			Limit:     1,
			Remaining: 0,
			ResetAt:   time.Now().Add(time.Minute),
		},
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))

	assertErrorResponse(t, recorder, http.StatusTooManyRequests, domain.CodeRateLimitExceeded)
}

func TestRateLimitFailsClosedWhenLimiterErrors(t *testing.T) {
	principal := auth.APIKeyPrincipal{
		OrgID:    uuid.New(),
		APIKeyID: uuid.New(),
		RPMLimit: 1,
	}
	router := rateLimitTestRouter(principal, fakeLimiter{
		err: errors.New("redis unavailable"),
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))

	assertErrorResponse(t, recorder, http.StatusServiceUnavailable, domain.CodeProviderUnavailable)
}

func TestRateLimitRequiresAPIKeyPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.Use(RateLimit(fakeLimiter{
		decision: ratelimit.Decision{Allowed: true},
	}))
	router.GET("/v1/chat/completions", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))

	assertErrorResponse(t, recorder, http.StatusUnauthorized, domain.CodeUnauthorized)
}

func rateLimitTestRouter(principal auth.APIKeyPrincipal, limiter RateLimiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.Use(func(c *gin.Context) {
		SetAPIKeyPrincipal(c, principal)
		c.Next()
	})
	router.Use(RateLimit(limiter))
	router.GET("/v1/chat/completions", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return router
}

func assertErrorResponse(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if recorder.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, wantStatus, recorder.Body.String())
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v; body = %s", err, recorder.Body.String())
	}
	if body.Error.Code != wantCode {
		t.Fatalf("error.code = %q, want %q; body = %s", body.Error.Code, wantCode, recorder.Body.String())
	}
}
