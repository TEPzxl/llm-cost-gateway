package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
)

const redisKeyPrefix = "prompt_exact"

var (
	ErrCacheMiss       = errors.New("prompt cache miss")
	ErrInvalidCacheKey = errors.New("invalid prompt cache key")
)

type PromptKeyInput struct {
	OrgID          uuid.UUID
	RequestedModel string
	Messages       []contract.ChatMessage
	Temperature    *float64
	MaxTokens      *int
}

type PromptKey struct {
	RedisKey     string
	CacheKeyHash string
	MessagesHash string
}

type CachedChatResponse struct {
	ProviderID    uuid.UUID `json:"provider_id"`
	ModelID       uuid.UUID `json:"model_id"`
	ProviderName  string    `json:"provider_name"`
	ProviderModel string    `json:"provider_model"`
	Role          string    `json:"role"`
	Content       string    `json:"content"`
	FinishReason  string    `json:"finish_reason"`
	CachedAt      time.Time `json:"cached_at"`
}

type PromptCache struct {
	client redis.Cmdable
	ttl    time.Duration
	now    func() time.Time
}

func NewPromptCache(client redis.Cmdable, ttl time.Duration) *PromptCache {
	return &PromptCache{
		client: client,
		ttl:    ttl,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func BuildPromptKey(input PromptKeyInput) (PromptKey, error) {
	if input.OrgID == uuid.Nil {
		return PromptKey{}, fmt.Errorf("%w: org_id is required", ErrInvalidCacheKey)
	}
	if strings.TrimSpace(input.RequestedModel) == "" {
		return PromptKey{}, fmt.Errorf("%w: requested_model is required", ErrInvalidCacheKey)
	}
	if len(input.Messages) == 0 {
		return PromptKey{}, fmt.Errorf("%w: messages are required", ErrInvalidCacheKey)
	}

	messagesJSON, err := json.Marshal(input.Messages)
	if err != nil {
		return PromptKey{}, err
	}
	messagesHash := sha256Hex(messagesJSON)
	keyPayload := promptKeyPayload{
		OrgID:          input.OrgID.String(),
		RequestedModel: strings.TrimSpace(input.RequestedModel),
		MessagesHash:   messagesHash,
		Temperature:    floatPtrKey(input.Temperature),
		MaxTokens:      intPtrKey(input.MaxTokens),
	}
	keyJSON, err := json.Marshal(keyPayload)
	if err != nil {
		return PromptKey{}, err
	}
	cacheKeyHash := sha256Hex(keyJSON)
	return PromptKey{
		RedisKey:     redisKeyPrefix + ":" + input.OrgID.String() + ":" + cacheKeyHash,
		CacheKeyHash: cacheKeyHash,
		MessagesHash: messagesHash,
	}, nil
}

func (c *PromptCache) Get(ctx context.Context, key PromptKey) (CachedChatResponse, error) {
	if c == nil || c.client == nil {
		return CachedChatResponse{}, ErrCacheMiss
	}
	raw, err := c.client.Get(ctx, key.RedisKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return CachedChatResponse{}, ErrCacheMiss
		}
		return CachedChatResponse{}, err
	}
	var item CachedChatResponse
	if err := json.Unmarshal(raw, &item); err != nil {
		return CachedChatResponse{}, err
	}
	return item, nil
}

func (c *PromptCache) Set(ctx context.Context, key PromptKey, item CachedChatResponse) error {
	if c == nil || c.client == nil {
		return nil
	}
	ttl := c.ttl
	if ttl <= 0 {
		return fmt.Errorf("prompt cache ttl must be greater than zero")
	}
	if item.CachedAt.IsZero() {
		item.CachedAt = c.now()
	}
	body, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key.RedisKey, body, ttl).Err()
}

type promptKeyPayload struct {
	OrgID          string `json:"org_id"`
	RequestedModel string `json:"requested_model"`
	MessagesHash   string `json:"messages_hash"`
	Temperature    string `json:"temperature"`
	MaxTokens      string `json:"max_tokens"`
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func floatPtrKey(value *float64) string {
	if value == nil {
		return "null"
	}
	return strconv.FormatFloat(*value, 'g', -1, 64)
}

func intPtrKey(value *int) string {
	if value == nil {
		return "null"
	}
	return strconv.Itoa(*value)
}
