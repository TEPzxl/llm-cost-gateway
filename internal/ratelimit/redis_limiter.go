package ratelimit

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tep/llm-cost-gateway/internal/auth"
)

const defaultWindowTTL = 2 * time.Minute

type Decision struct {
	Allowed   bool
	Limit     int32
	Remaining int32
	ResetAt   time.Time
	Key       string
}

type RedisLimiter struct {
	client    redis.Cmdable
	clock     func() time.Time
	windowTTL time.Duration
}

type Option func(*RedisLimiter)

func WithClock(clock func() time.Time) Option {
	return func(l *RedisLimiter) {
		if clock != nil {
			l.clock = clock
		}
	}
}

func NewRedisLimiter(client redis.Cmdable, opts ...Option) *RedisLimiter {
	limiter := &RedisLimiter{
		client:    client,
		clock:     func() time.Time { return time.Now().UTC() },
		windowTTL: defaultWindowTTL,
	}
	for _, opt := range opts {
		opt(limiter)
	}
	return limiter
}

func (l *RedisLimiter) Allow(ctx context.Context, principal auth.APIKeyPrincipal) (Decision, error) {
	now := l.clock().UTC()
	unixMinute := now.Unix() / int64(time.Minute/time.Second)
	resetAt := time.Unix((unixMinute+1)*int64(time.Minute/time.Second), 0).UTC()
	key := fmt.Sprintf("rl:api_key:%s:minute:%d", principal.APIKeyID.String(), unixMinute)

	decision := Decision{
		Allowed:   false,
		Limit:     principal.RPMLimit,
		Remaining: 0,
		ResetAt:   resetAt,
		Key:       key,
	}
	if principal.RPMLimit <= 0 {
		return decision, fmt.Errorf("rpm_limit must be greater than zero")
	}
	if l.client == nil {
		return decision, fmt.Errorf("redis limiter client is nil")
	}

	count, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return decision, err
	}
	if count == 1 {
		if err := l.client.Expire(ctx, key, l.windowTTL).Err(); err != nil {
			return decision, err
		}
	}

	remaining := int64(principal.RPMLimit) - count
	if remaining < 0 {
		remaining = 0
	}
	if remaining > math.MaxInt32 {
		remaining = math.MaxInt32
	}

	decision.Allowed = count <= int64(principal.RPMLimit)
	decision.Remaining = int32(remaining)
	return decision, nil
}
