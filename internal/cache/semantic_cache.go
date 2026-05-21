package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const semanticRedisKeyPrefix = "semantic_cache"

var ErrSemanticCacheMiss = errors.New("semantic cache miss")

type SemanticCache struct {
	client    redis.Cmdable
	ttl       time.Duration
	now       func() time.Time
	responses responseCodec
}

type SemanticStoreInput struct {
	OrgID          uuid.UUID
	RequestedModel string
	MessagesHash   string
	Embedding      []float64
	Response       CachedChatResponse
}

type SemanticLookupInput struct {
	OrgID          uuid.UUID
	RequestedModel string
	Embedding      []float64
	Threshold      float64
}

type SemanticMatch struct {
	CacheKeyHash string
	MessagesHash string
	Similarity   float64
	Response     CachedChatResponse
}

type semanticItem struct {
	CacheKeyHash   string                      `json:"cache_key_hash"`
	MessagesHash   string                      `json:"messages_hash"`
	RequestedModel string                      `json:"requested_model"`
	Embedding      []float64                   `json:"embedding"`
	Response       encryptedCachedChatResponse `json:"response"`
	CreatedAt      time.Time                   `json:"created_at"`
}

func NewSemanticCache(client redis.Cmdable, ttl time.Duration, secretEncryptionKey string) (*SemanticCache, error) {
	responses, err := newResponseCodec(secretEncryptionKey)
	if err != nil {
		return nil, err
	}
	return &SemanticCache{
		client:    client,
		ttl:       ttl,
		now:       func() time.Time { return time.Now().UTC() },
		responses: responses,
	}, nil
}

func (c *SemanticCache) Store(ctx context.Context, input SemanticStoreInput) (SemanticMatch, error) {
	if c == nil || c.client == nil {
		return SemanticMatch{}, nil
	}
	if err := validateSemanticStoreInput(input); err != nil {
		return SemanticMatch{}, err
	}
	if c.ttl <= 0 {
		return SemanticMatch{}, fmt.Errorf("semantic cache ttl must be greater than zero")
	}
	cacheKeyHash := semanticCacheKeyHash(input.OrgID, input.RequestedModel, input.MessagesHash)
	response := input.Response
	createdAt := c.now()
	if response.CachedAt.IsZero() {
		response.CachedAt = createdAt
	}
	sealedResponse, err := c.responses.Seal(response)
	if err != nil {
		return SemanticMatch{}, err
	}
	item := semanticItem{
		CacheKeyHash:   cacheKeyHash,
		MessagesHash:   input.MessagesHash,
		RequestedModel: strings.TrimSpace(input.RequestedModel),
		Embedding:      append([]float64(nil), input.Embedding...),
		Response:       sealedResponse,
		CreatedAt:      createdAt,
	}
	body, err := json.Marshal(item)
	if err != nil {
		return SemanticMatch{}, err
	}
	itemKey := semanticItemKey(input.OrgID, input.RequestedModel, cacheKeyHash)
	if err := c.client.Set(ctx, itemKey, body, c.ttl).Err(); err != nil {
		return SemanticMatch{}, err
	}
	if err := c.client.SAdd(ctx, semanticIndexKey(input.OrgID, input.RequestedModel), itemKey).Err(); err != nil {
		return SemanticMatch{}, err
	}
	return SemanticMatch{
		CacheKeyHash: cacheKeyHash,
		MessagesHash: input.MessagesHash,
		Similarity:   1,
		Response:     response,
	}, nil
}

func (c *SemanticCache) Lookup(ctx context.Context, input SemanticLookupInput) (SemanticMatch, error) {
	if c == nil || c.client == nil {
		return SemanticMatch{}, ErrSemanticCacheMiss
	}
	if err := validateSemanticLookupInput(input); err != nil {
		return SemanticMatch{}, err
	}
	keys, err := c.client.SMembers(ctx, semanticIndexKey(input.OrgID, input.RequestedModel)).Result()
	if err != nil {
		return SemanticMatch{}, err
	}
	var best SemanticMatch
	for _, key := range keys {
		raw, err := c.client.Get(ctx, key).Bytes()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				_ = c.client.SRem(ctx, semanticIndexKey(input.OrgID, input.RequestedModel), key).Err()
				continue
			}
			return SemanticMatch{}, err
		}
		var item semanticItem
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		score := CosineSimilarity(input.Embedding, item.Embedding)
		if score > best.Similarity {
			response, err := c.responses.Open(item.Response)
			if err != nil {
				_ = c.client.Del(ctx, key).Err()
				_ = c.client.SRem(ctx, semanticIndexKey(input.OrgID, input.RequestedModel), key).Err()
				continue
			}
			best = SemanticMatch{
				CacheKeyHash: item.CacheKeyHash,
				MessagesHash: item.MessagesHash,
				Similarity:   score,
				Response:     response,
			}
		}
	}
	if best.CacheKeyHash == "" || best.Similarity < input.Threshold {
		return SemanticMatch{}, ErrSemanticCacheMiss
	}
	return best, nil
}

func CosineSimilarity(a []float64, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for index := range a {
		dot += a[index] * b[index]
		normA += a[index] * a[index]
		normB += b[index] * b[index]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func validateSemanticStoreInput(input SemanticStoreInput) error {
	if input.OrgID == uuid.Nil || strings.TrimSpace(input.RequestedModel) == "" || input.MessagesHash == "" || len(input.Embedding) == 0 {
		return fmt.Errorf("invalid semantic cache store input")
	}
	return nil
}

func validateSemanticLookupInput(input SemanticLookupInput) error {
	if input.OrgID == uuid.Nil || strings.TrimSpace(input.RequestedModel) == "" || len(input.Embedding) == 0 {
		return fmt.Errorf("invalid semantic cache lookup input")
	}
	if input.Threshold <= 0 || input.Threshold > 1 {
		return fmt.Errorf("semantic cache threshold must be greater than 0 and less than or equal to 1")
	}
	return nil
}

func semanticIndexKey(orgID uuid.UUID, requestedModel string) string {
	return semanticRedisKeyPrefix + ":index:" + orgID.String() + ":" + sha256String(strings.TrimSpace(requestedModel))
}

func semanticItemKey(orgID uuid.UUID, requestedModel string, cacheKeyHash string) string {
	return semanticRedisKeyPrefix + ":item:" + orgID.String() + ":" + sha256String(strings.TrimSpace(requestedModel)) + ":" + cacheKeyHash
}

func semanticCacheKeyHash(orgID uuid.UUID, requestedModel string, messagesHash string) string {
	return sha256String(orgID.String() + ":" + strings.TrimSpace(requestedModel) + ":" + messagesHash)
}

func sha256String(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
