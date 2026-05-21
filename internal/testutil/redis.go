package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
)

func OpenRedis(t *testing.T, ctx context.Context) *redis.Client {
	t.Helper()

	dsn := os.Getenv("TEST_REDIS_URL")
	if dsn == "" {
		dsn = "redis://localhost:6379/15"
	}

	opts, err := redis.ParseURL(dsn)
	if err != nil {
		t.Fatalf("parse TEST_REDIS_URL: %v", err)
	}

	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Skipf("test redis unavailable at %s: %v", dsn, err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	return client
}
