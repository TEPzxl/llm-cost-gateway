package cache

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/embedding"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestSemanticCacheSimilarRequestHit(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenRedis(t, ctx)
	cache := NewSemanticCache(client, time.Minute)
	adapter := embedding.NewMockAdapter()
	orgID := uuid.New()
	storedVector := mustEmbed(t, adapter, "hello world")
	lookupVector := mustEmbed(t, adapter, "Hello, world!")

	stored, err := cache.Store(ctx, SemanticStoreInput{
		OrgID:          orgID,
		RequestedModel: "fast-chat",
		MessagesHash:   "messages-hash",
		Embedding:      storedVector,
		Response:       CachedChatResponse{ProviderID: uuid.New(), ModelID: uuid.New(), Content: "cached response"},
	})
	if err != nil {
		t.Fatalf("Store returned error: %v", err)
	}
	match, err := cache.Lookup(ctx, SemanticLookupInput{
		OrgID:          orgID,
		RequestedModel: "fast-chat",
		Embedding:      lookupVector,
		Threshold:      0.99,
	})
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if match.CacheKeyHash != stored.CacheKeyHash || match.Response.Content != "cached response" {
		t.Fatalf("match = %+v, want stored response", match)
	}
}

func TestSemanticCacheDissimilarRequestMiss(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenRedis(t, ctx)
	cache := NewSemanticCache(client, time.Minute)
	adapter := embedding.NewMockAdapter()
	orgID := uuid.New()

	if _, err := cache.Store(ctx, SemanticStoreInput{
		OrgID:          orgID,
		RequestedModel: "fast-chat",
		MessagesHash:   "messages-hash",
		Embedding:      mustEmbed(t, adapter, "hello world"),
		Response:       CachedChatResponse{ProviderID: uuid.New(), ModelID: uuid.New(), Content: "cached response"},
	}); err != nil {
		t.Fatalf("Store returned error: %v", err)
	}
	_, err := cache.Lookup(ctx, SemanticLookupInput{
		OrgID:          orgID,
		RequestedModel: "fast-chat",
		Embedding:      mustEmbed(t, adapter, "goodbye mars"),
		Threshold:      0.9,
	})
	if !errors.Is(err, ErrSemanticCacheMiss) {
		t.Fatalf("Lookup error = %v, want ErrSemanticCacheMiss", err)
	}
}

func TestSemanticCacheThresholdApplies(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenRedis(t, ctx)
	cache := NewSemanticCache(client, time.Minute)
	adapter := embedding.NewMockAdapter()
	orgID := uuid.New()

	if _, err := cache.Store(ctx, SemanticStoreInput{
		OrgID:          orgID,
		RequestedModel: "fast-chat",
		MessagesHash:   "messages-hash",
		Embedding:      mustEmbed(t, adapter, "hello world"),
		Response:       CachedChatResponse{ProviderID: uuid.New(), ModelID: uuid.New(), Content: "cached response"},
	}); err != nil {
		t.Fatalf("Store returned error: %v", err)
	}
	match, err := cache.Lookup(ctx, SemanticLookupInput{
		OrgID:          orgID,
		RequestedModel: "fast-chat",
		Embedding:      mustEmbed(t, adapter, "hello there"),
		Threshold:      0.4,
	})
	if err != nil {
		t.Fatalf("Lookup low threshold returned error: %v", err)
	}
	if match.Similarity < 0.4 {
		t.Fatalf("similarity = %f, want >= 0.4", match.Similarity)
	}
	_, err = cache.Lookup(ctx, SemanticLookupInput{
		OrgID:          orgID,
		RequestedModel: "fast-chat",
		Embedding:      mustEmbed(t, adapter, "hello there"),
		Threshold:      0.9,
	})
	if !errors.Is(err, ErrSemanticCacheMiss) {
		t.Fatalf("Lookup high threshold error = %v, want ErrSemanticCacheMiss", err)
	}
}

func TestSemanticCacheDoesNotStorePromptText(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenRedis(t, ctx)
	cache := NewSemanticCache(client, time.Minute)
	adapter := embedding.NewMockAdapter()
	orgID := uuid.New()
	prompt := "secret prompt text"

	match, err := cache.Store(ctx, SemanticStoreInput{
		OrgID:          orgID,
		RequestedModel: "fast-chat",
		MessagesHash:   "messages-hash",
		Embedding:      mustEmbed(t, adapter, prompt),
		Response:       CachedChatResponse{ProviderID: uuid.New(), ModelID: uuid.New(), Content: "cached response"},
	})
	if err != nil {
		t.Fatalf("Store returned error: %v", err)
	}
	raw, err := client.Get(ctx, semanticItemKey(orgID, "fast-chat", match.CacheKeyHash)).Result()
	if err != nil {
		t.Fatalf("Redis Get returned error: %v", err)
	}
	if strings.Contains(raw, prompt) {
		t.Fatalf("semantic cache value leaked prompt text: %s", raw)
	}
}

func mustEmbed(t *testing.T, adapter *embedding.MockAdapter, text string) []float64 {
	t.Helper()

	vector, err := adapter.Embed(context.Background(), text)
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	return vector
}
