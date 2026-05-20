package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const APIKeyPlainPrefix = "llmgw_live_"

var (
	ErrInvalidAPIKey  = errors.New("invalid api key")
	ErrAPIKeyNotFound = errors.New("api key not found")
)

type APIKeyPrincipal struct {
	OrgID    uuid.UUID
	APIKeyID uuid.UUID
	Scopes   []string
	RPMLimit int32
}

type CreateAPIKeyParams struct {
	OrgID     uuid.UUID
	Name      string
	Scopes    []string
	RPMLimit  int32
	ExpiresAt *time.Time
}

type CreateAPIKeyResult struct {
	APIKey db.ApiKey
	Key    string
}

type APIKeyService struct {
	queries *db.Queries
	hasher  TokenHasher
	now     func() time.Time
}

func NewAPIKeyService(queries *db.Queries, tokenHashSecret string) *APIKeyService {
	return &APIKeyService{
		queries: queries,
		hasher:  NewTokenHasher(tokenHashSecret),
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *APIKeyService) CreateAPIKey(ctx context.Context, params CreateAPIKeyParams) (CreateAPIKeyResult, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return CreateAPIKeyResult{}, fmt.Errorf("api key name is required")
	}
	if params.OrgID == uuid.Nil {
		return CreateAPIKeyResult{}, fmt.Errorf("org_id is required")
	}
	if params.RPMLimit <= 0 {
		return CreateAPIKeyResult{}, fmt.Errorf("rpm_limit must be greater than zero")
	}

	scopes := params.Scopes
	if len(scopes) == 0 {
		scopes = []string{"chat.completions"}
	}

	key, err := generateToken(APIKeyPlainPrefix)
	if err != nil {
		return CreateAPIKeyResult{}, err
	}
	now := s.now()
	apiKey, err := s.queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		ID:        uuid.New(),
		OrgID:     params.OrgID,
		Name:      name,
		KeyPrefix: visibleCredentialPrefix(key, APIKeyPlainPrefix),
		KeyHash:   s.hasher.Hash(key),
		Scopes:    scopes,
		Status:    "active",
		RpmLimit:  params.RPMLimit,
		ExpiresAt: params.ExpiresAt,
		CreatedAt: now,
	})
	if err != nil {
		return CreateAPIKeyResult{}, err
	}

	return CreateAPIKeyResult{
		APIKey: apiKey,
		Key:    key,
	}, nil
}

func (s *APIKeyService) ListAPIKeys(ctx context.Context, orgID uuid.UUID) ([]db.ApiKey, error) {
	return s.queries.ListAPIKeys(ctx, orgID)
}

func (s *APIKeyService) RevokeAPIKey(ctx context.Context, orgID uuid.UUID, apiKeyID uuid.UUID) (db.ApiKey, error) {
	now := s.now()
	apiKey, err := s.queries.RevokeAPIKey(ctx, db.RevokeAPIKeyParams{
		OrgID:     orgID,
		ID:        apiKeyID,
		RevokedAt: &now,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ApiKey{}, ErrAPIKeyNotFound
		}
		return db.ApiKey{}, err
	}
	return apiKey, nil
}

func (s *APIKeyService) Authenticate(ctx context.Context, key string) (APIKeyPrincipal, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return APIKeyPrincipal{}, ErrInvalidAPIKey
	}

	apiKey, err := s.queries.GetAPIKeyByHash(ctx, s.hasher.Hash(key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return APIKeyPrincipal{}, ErrInvalidAPIKey
		}
		return APIKeyPrincipal{}, err
	}
	if apiKey.Status != "active" {
		return APIKeyPrincipal{}, ErrInvalidAPIKey
	}
	now := s.now()
	if apiKey.ExpiresAt != nil && !apiKey.ExpiresAt.After(now) {
		return APIKeyPrincipal{}, ErrInvalidAPIKey
	}

	if err := s.queries.TouchAPIKeyLastUsed(ctx, db.TouchAPIKeyLastUsedParams{
		OrgID:      apiKey.OrgID,
		ID:         apiKey.ID,
		LastUsedAt: &now,
	}); err != nil {
		return APIKeyPrincipal{}, err
	}

	return APIKeyPrincipal{
		OrgID:    apiKey.OrgID,
		APIKeyID: apiKey.ID,
		Scopes:   apiKey.Scopes,
		RPMLimit: apiKey.RpmLimit,
	}, nil
}
