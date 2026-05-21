package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tep/llm-cost-gateway/internal/email"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	MagicLinkPlainPrefix = "llmgw_magic_"
	defaultMagicLinkTTL  = 15 * time.Minute
)

var ErrInvalidMagicLink = errors.New("invalid magic link")

type MagicLinkConfig struct {
	BaseURL string
	TTL     time.Duration
}

type MagicLinkService struct {
	store       *store.Store
	hashKeyRing *TokenHashKeyRing
	sender      email.Sender
	config      MagicLinkConfig
	now         func() time.Time
	ttl         time.Duration
}

type MagicLinkServiceOption func(*MagicLinkService)

func WithMagicLinkClock(clock func() time.Time) MagicLinkServiceOption {
	return func(s *MagicLinkService) {
		if clock != nil {
			s.now = clock
		}
	}
}

func WithMagicLinkHashKeyRing(keyRing *TokenHashKeyRing) MagicLinkServiceOption {
	return func(s *MagicLinkService) {
		if keyRing != nil {
			s.hashKeyRing = keyRing
		}
	}
}

func NewMagicLinkService(st *store.Store, tokenHashSecret string, sender email.Sender, config MagicLinkConfig, opts ...MagicLinkServiceOption) *MagicLinkService {
	ttl := config.TTL
	if ttl <= 0 {
		ttl = defaultMagicLinkTTL
	}
	keyRing, _ := NewSingleTokenHashKeyRing(tokenHashSecret)
	service := &MagicLinkService{
		store:       st,
		hashKeyRing: keyRing,
		sender:      sender,
		config:      config,
		now:         func() time.Time { return time.Now().UTC() },
		ttl:         ttl,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

type MagicLinkRequestParams struct {
	OrgSlug string
	Email   string
}

type MagicLinkConsumeParams struct {
	Token string
}

func (s *MagicLinkService) Request(ctx context.Context, params MagicLinkRequestParams) error {
	if s.store == nil || s.sender == nil {
		return fmt.Errorf("magic link service is not configured")
	}
	orgSlug := strings.TrimSpace(params.OrgSlug)
	userEmail := normalizeEmail(params.Email)
	if orgSlug == "" || userEmail == "" {
		return nil
	}

	membership, err := s.store.Queries.GetActiveMembershipByOrgSlugAndEmail(ctx, db.GetActiveMembershipByOrgSlugAndEmailParams{
		OrgSlug: orgSlug,
		Email:   userEmail,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	token, err := generateToken(MagicLinkPlainPrefix)
	if err != nil {
		return err
	}
	now := s.now()
	if _, err := s.store.Queries.CreateMagicLinkToken(ctx, db.CreateMagicLinkTokenParams{
		ID:          uuid.New(),
		OrgID:       membership.OrgID,
		UserID:      membership.UserID,
		Email:       membership.Email,
		TokenPrefix: visibleCredentialPrefix(token, MagicLinkPlainPrefix),
		TokenHash:   s.hashKeyRing.Hash(token),
		Status:      "active",
		ExpiresAt:   now.Add(s.ttl),
		CreatedAt:   now,
	}); err != nil {
		return err
	}

	return s.sender.Send(ctx, email.Message{
		To:       membership.Email,
		Subject:  "LLM Cost Gateway 登录链接",
		TextBody: fmt.Sprintf("请在 %s 前打开以下链接登录：\n\n%s", now.Add(s.ttl).Format(time.RFC3339), s.magicLinkURL(token)),
	})
}

func (s *MagicLinkService) Consume(ctx context.Context, params MagicLinkConsumeParams) (PasswordlessMockLoginResult, error) {
	token := strings.TrimSpace(params.Token)
	if token == "" {
		return PasswordlessMockLoginResult{}, ErrInvalidMagicLink
	}

	sessionToken, err := generateToken(SessionPlainPrefix)
	if err != nil {
		return PasswordlessMockLoginResult{}, err
	}
	now := s.now()
	var result PasswordlessMockLoginResult
	err = s.store.ExecTx(ctx, func(q *db.Queries) error {
		var magicLink db.ConsumeMagicLinkTokenRow
		var found bool
		for _, tokenHash := range s.hashKeyRing.CandidateHashes(token) {
			item, err := q.ConsumeMagicLinkToken(ctx, db.ConsumeMagicLinkTokenParams{
				TokenHash:  tokenHash,
				ConsumedAt: now,
				Now:        now,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				return err
			}
			magicLink = item
			found = true
			break
		}
		if !found {
			return ErrInvalidMagicLink
		}
		session, err := q.CreateUserSession(ctx, db.CreateUserSessionParams{
			ID:          uuid.New(),
			OrgID:       magicLink.OrgID,
			UserID:      magicLink.UserID,
			TokenPrefix: visibleCredentialPrefix(sessionToken, SessionPlainPrefix),
			TokenHash:   s.hashKeyRing.Hash(sessionToken),
			Status:      "active",
			ExpiresAt:   now.Add(defaultSessionTTL),
			CreatedAt:   now,
		})
		if err != nil {
			return err
		}
		result = PasswordlessMockLoginResult{
			Token:        sessionToken,
			Session:      session,
			OrgID:        magicLink.OrgID,
			OrgSlug:      magicLink.OrgSlug,
			UserID:       magicLink.UserID,
			Email:        magicLink.Email,
			DisplayName:  magicLink.DisplayName,
			MembershipID: magicLink.MembershipID,
			Role:         magicLink.Role,
		}
		return nil
	})
	if err != nil {
		return PasswordlessMockLoginResult{}, err
	}
	return result, nil
}

func (s *MagicLinkService) magicLinkURL(token string) string {
	parsed, err := url.Parse(s.config.BaseURL)
	if err != nil {
		return s.config.BaseURL
	}
	query := parsed.Query()
	query.Set("magic_token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
