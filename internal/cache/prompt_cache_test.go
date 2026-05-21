package cache

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

const testSecretEncryptionKey = "0123456789abcdef0123456789abcdef"

func TestPromptCacheMissThenHit(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenRedis(t, ctx)
	cache := newTestPromptCache(t, client, time.Minute)
	key := mustPromptKey(t, nil, nil)
	cached := CachedChatResponse{
		ProviderID:    uuid.New(),
		ModelID:       uuid.New(),
		ProviderName:  "mock-provider",
		ProviderModel: "mock-small",
		Role:          "assistant",
		Content:       "cached response",
		FinishReason:  "stop",
	}

	if _, err := cache.Get(ctx, key); !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("initial Get error = %v, want ErrCacheMiss", err)
	}
	if err := cache.Set(ctx, key, cached); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	got, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Content != cached.Content || got.ProviderID != cached.ProviderID {
		t.Fatalf("cached response = %+v, want %+v", got, cached)
	}
}

func TestPromptCacheKeyVariesByTemperature(t *testing.T) {
	low := 0.1
	high := 0.9
	lowKey := mustPromptKey(t, &low, nil)
	highKey := mustPromptKey(t, &high, nil)

	if lowKey.CacheKeyHash == highKey.CacheKeyHash {
		t.Fatal("cache key hash should vary by temperature")
	}
	if lowKey.MessagesHash != highKey.MessagesHash {
		t.Fatal("messages hash should not vary by temperature")
	}
}

func TestPromptCacheTTLExpiry(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenRedis(t, ctx)
	cache := newTestPromptCache(t, client, 50*time.Millisecond)
	key := mustPromptKey(t, nil, nil)

	if err := cache.Set(ctx, key, CachedChatResponse{ProviderID: uuid.New(), ModelID: uuid.New(), Content: "cached"}); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	if _, err := cache.Get(ctx, key); !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("expired Get error = %v, want ErrCacheMiss", err)
	}
}

func TestPromptCacheDoesNotStorePromptText(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenRedis(t, ctx)
	cache := newTestPromptCache(t, client, time.Minute)
	prompt := "secret prompt text"
	key, err := BuildPromptKey(PromptKeyInput{
		OrgID:          uuid.New(),
		RequestedModel: "fast-chat",
		Messages:       []contract.ChatMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		t.Fatalf("BuildPromptKey returned error: %v", err)
	}
	if strings.Contains(key.RedisKey, prompt) || strings.Contains(key.MessagesHash, prompt) {
		t.Fatalf("cache key leaked prompt text: %+v", key)
	}
	if err := cache.Set(ctx, key, CachedChatResponse{ProviderID: uuid.New(), ModelID: uuid.New(), Content: "cached response"}); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	raw, err := client.Get(ctx, key.RedisKey).Result()
	if err != nil {
		t.Fatalf("Redis Get returned error: %v", err)
	}
	if strings.Contains(raw, prompt) {
		t.Fatalf("cache value leaked prompt text: %s", raw)
	}
	if strings.Contains(raw, "cached response") {
		t.Fatalf("cache value leaked response content: %s", raw)
	}
}

func newTestPromptCache(t *testing.T, client redis.Cmdable, ttl time.Duration) *PromptCache {
	t.Helper()

	cache, err := NewPromptCache(client, ttl, testSecretEncryptionKey)
	if err != nil {
		t.Fatalf("NewPromptCache returned error: %v", err)
	}
	return cache
}

func mustPromptKey(t *testing.T, temperature *float64, maxTokens *int) PromptKey {
	t.Helper()

	key, err := BuildPromptKey(PromptKeyInput{
		OrgID:          uuid.New(),
		RequestedModel: "fast-chat",
		Messages:       []contract.ChatMessage{{Role: "user", Content: "hello"}},
		Temperature:    temperature,
		MaxTokens:      maxTokens,
	})
	if err != nil {
		t.Fatalf("BuildPromptKey returned error: %v", err)
	}
	return key
}
