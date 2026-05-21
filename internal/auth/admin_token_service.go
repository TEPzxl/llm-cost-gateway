package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	AdminTokenPlainPrefix = "llmgw_admin_"
	tokenRandomBytes      = 32
	tokenPrefixSuffixLen  = 8
)

var (
	ErrInvalidToken         = errors.New("invalid admin token")
	ErrOrganizationNotFound = errors.New("organization not found")
)

type AdminTokenPrincipal struct {
	OrgID        uuid.UUID
	AdminTokenID uuid.UUID
	ActorType    string
	Role         string
	UserID       *uuid.UUID
	MembershipID *uuid.UUID
	SessionID    *uuid.UUID
	Scopes       []string
}

type AdminTokenAuthenticator interface {
	Authenticate(ctx context.Context, token string) (AdminTokenPrincipal, error)
}

type CreateAdminTokenParams struct {
	OrgID     uuid.UUID
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

type CreateAdminTokenResult struct {
	AdminToken db.AdminToken
	Token      string
}

type AdminTokenService struct {
	queries *db.Queries
	hasher  TokenHasher
	now     func() time.Time
}

func NewAdminTokenService(queries *db.Queries, tokenHashSecret string) *AdminTokenService {
	return &AdminTokenService{
		queries: queries,
		hasher:  NewTokenHasher(tokenHashSecret),
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *AdminTokenService) CreateAdminToken(ctx context.Context, params CreateAdminTokenParams) (CreateAdminTokenResult, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return CreateAdminTokenResult{}, fmt.Errorf("admin token name is required")
	}
	if params.OrgID == uuid.Nil {
		return CreateAdminTokenResult{}, ErrOrganizationNotFound
	}
	if _, err := s.queries.GetOrganization(ctx, params.OrgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CreateAdminTokenResult{}, ErrOrganizationNotFound
		}
		return CreateAdminTokenResult{}, err
	}

	scopes := params.Scopes
	if len(scopes) == 0 {
		scopes = []string{"admin:*"}
	}

	token, err := generateToken(AdminTokenPlainPrefix)
	if err != nil {
		return CreateAdminTokenResult{}, err
	}
	now := s.now()
	adminToken, err := s.queries.CreateAdminToken(ctx, db.CreateAdminTokenParams{
		ID:          uuid.New(),
		OrgID:       params.OrgID,
		Name:        name,
		TokenPrefix: visibleCredentialPrefix(token, AdminTokenPlainPrefix),
		TokenHash:   s.hasher.Hash(token),
		Scopes:      scopes,
		Status:      "active",
		ExpiresAt:   params.ExpiresAt,
		CreatedAt:   now,
	})
	if err != nil {
		return CreateAdminTokenResult{}, err
	}

	return CreateAdminTokenResult{
		AdminToken: adminToken,
		Token:      token,
	}, nil
}

func (s *AdminTokenService) Authenticate(ctx context.Context, token string) (AdminTokenPrincipal, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AdminTokenPrincipal{}, ErrInvalidToken
	}

	adminToken, err := s.queries.GetAdminTokenByHash(ctx, s.hasher.Hash(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminTokenPrincipal{}, ErrInvalidToken
		}
		return AdminTokenPrincipal{}, err
	}
	if adminToken.Status != "active" {
		return AdminTokenPrincipal{}, ErrInvalidToken
	}
	now := s.now()
	if adminToken.ExpiresAt != nil && !adminToken.ExpiresAt.After(now) {
		return AdminTokenPrincipal{}, ErrInvalidToken
	}

	if err := s.queries.TouchAdminTokenLastUsed(ctx, db.TouchAdminTokenLastUsedParams{
		OrgID:      adminToken.OrgID,
		ID:         adminToken.ID,
		LastUsedAt: &now,
	}); err != nil {
		return AdminTokenPrincipal{}, err
	}

	return AdminTokenPrincipal{
		OrgID:        adminToken.OrgID,
		AdminTokenID: adminToken.ID,
		ActorType:    ActorServiceToken,
		Role:         RoleOwner,
		Scopes:       adminToken.Scopes,
	}, nil
}

func generateToken(prefix string) (string, error) {
	random := make([]byte, tokenRandomBytes)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(random), nil
}

func visibleCredentialPrefix(token string, plainPrefix string) string {
	maxLen := len(plainPrefix) + tokenPrefixSuffixLen
	if len(token) <= maxLen {
		return token
	}
	return token[:maxLen]
}
