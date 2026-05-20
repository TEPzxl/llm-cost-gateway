package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/tep/llm-cost-gateway/internal/auth"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestRedisLimiterAllowsUntilLimitThenBlocks(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenRedis(t, ctx)

	now := time.Date(2026, 5, 20, 10, 15, 30, 0, time.UTC)
	limiter := NewRedisLimiter(client, WithClock(func() time.Time { return now }))
	principal := auth.APIKeyPrincipal{
		OrgID:    uuid.New(),
		APIKeyID: uuid.New(),
		RPMLimit: 2,
	}

	first, err := limiter.Allow(ctx, principal)
	if err != nil {
		t.Fatalf("first allow: %v", err)
	}
	if !first.Allowed || first.Remaining != 1 {
		t.Fatalf("first decision = %+v, want allowed with 1 remaining", first)
	}

	second, err := limiter.Allow(ctx, principal)
	if err != nil {
		t.Fatalf("second allow: %v", err)
	}
	if !second.Allowed || second.Remaining != 0 {
		t.Fatalf("second decision = %+v, want allowed with 0 remaining", second)
	}

	third, err := limiter.Allow(ctx, principal)
	if err != nil {
		t.Fatalf("third allow: %v", err)
	}
	if third.Allowed || third.Remaining != 0 {
		t.Fatalf("third decision = %+v, want blocked with 0 remaining", third)
	}
}

func TestRedisLimiterScopesByAPIKey(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenRedis(t, ctx)

	now := time.Date(2026, 5, 20, 10, 15, 30, 0, time.UTC)
	limiter := NewRedisLimiter(client, WithClock(func() time.Time { return now }))
	orgID := uuid.New()
	apiKeyA := auth.APIKeyPrincipal{OrgID: orgID, APIKeyID: uuid.New(), RPMLimit: 1}
	apiKeyB := auth.APIKeyPrincipal{OrgID: orgID, APIKeyID: uuid.New(), RPMLimit: 1}

	if decision, err := limiter.Allow(ctx, apiKeyA); err != nil || !decision.Allowed {
		t.Fatalf("first api key A decision = %+v, err = %v; want allowed", decision, err)
	}
	if decision, err := limiter.Allow(ctx, apiKeyA); err != nil || decision.Allowed {
		t.Fatalf("second api key A decision = %+v, err = %v; want blocked", decision, err)
	}
	if decision, err := limiter.Allow(ctx, apiKeyB); err != nil || !decision.Allowed {
		t.Fatalf("api key B decision = %+v, err = %v; want allowed", decision, err)
	}
}

func TestRedisLimiterResetsWhenWindowChanges(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenRedis(t, ctx)

	now := time.Date(2026, 5, 20, 10, 15, 30, 0, time.UTC)
	limiter := NewRedisLimiter(client, WithClock(func() time.Time { return now }))
	principal := auth.APIKeyPrincipal{
		OrgID:    uuid.New(),
		APIKeyID: uuid.New(),
		RPMLimit: 1,
	}

	if decision, err := limiter.Allow(ctx, principal); err != nil || !decision.Allowed {
		t.Fatalf("first decision = %+v, err = %v; want allowed", decision, err)
	}
	if decision, err := limiter.Allow(ctx, principal); err != nil || decision.Allowed {
		t.Fatalf("second decision = %+v, err = %v; want blocked", decision, err)
	}

	now = now.Add(time.Minute)
	if decision, err := limiter.Allow(ctx, principal); err != nil || !decision.Allowed {
		t.Fatalf("next window decision = %+v, err = %v; want allowed", decision, err)
	}
}

func TestRedisLimiterFailsClosedWhenRedisUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	client := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 20 * time.Millisecond,
		ReadTimeout: 20 * time.Millisecond,
	})
	defer client.Close()

	limiter := NewRedisLimiter(client)
	principal := auth.APIKeyPrincipal{
		OrgID:    uuid.New(),
		APIKeyID: uuid.New(),
		RPMLimit: 1,
	}

	decision, err := limiter.Allow(ctx, principal)
	if err == nil {
		t.Fatalf("Allow err = nil, decision = %+v; want redis error", decision)
	}
	if decision.Allowed {
		t.Fatalf("decision = %+v, want fail closed", decision)
	}
}
