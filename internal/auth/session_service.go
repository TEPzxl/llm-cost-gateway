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

const (
	SessionPlainPrefix = "llmgw_session_"
	defaultSessionTTL  = 24 * time.Hour
)

var (
	ErrInvalidSession     = errors.New("invalid user session")
	ErrMembershipNotFound = errors.New("membership not found")
)

type SessionService struct {
	queries *db.Queries
	hasher  TokenHasher
	now     func() time.Time
	ttl     time.Duration
}

type SessionServiceOption func(*SessionService)

func WithSessionClock(clock func() time.Time) SessionServiceOption {
	return func(s *SessionService) {
		if clock != nil {
			s.now = clock
		}
	}
}

func WithSessionTTL(ttl time.Duration) SessionServiceOption {
	return func(s *SessionService) {
		if ttl > 0 {
			s.ttl = ttl
		}
	}
}

func NewSessionService(queries *db.Queries, tokenHashSecret string, opts ...SessionServiceOption) *SessionService {
	service := &SessionService{
		queries: queries,
		hasher:  NewTokenHasher(tokenHashSecret),
		now:     func() time.Time { return time.Now().UTC() },
		ttl:     defaultSessionTTL,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

type UpsertMemberParams struct {
	OrgID       uuid.UUID
	Email       string
	DisplayName string
	Role        string
}

type UpsertMemberResult struct {
	User       db.User
	Membership db.OrgMembership
}

type PasswordlessMockLoginParams struct {
	OrgSlug string
	Email   string
}

type PasswordlessMockLoginResult struct {
	Token        string
	Session      db.UserSession
	OrgID        uuid.UUID
	OrgSlug      string
	UserID       uuid.UUID
	Email        string
	DisplayName  string
	MembershipID uuid.UUID
	Role         string
}

func (s *SessionService) UpsertMember(ctx context.Context, params UpsertMemberParams) (UpsertMemberResult, error) {
	if params.OrgID == uuid.Nil {
		return UpsertMemberResult{}, ErrOrganizationNotFound
	}
	email := normalizeEmail(params.Email)
	if email == "" {
		return UpsertMemberResult{}, fmt.Errorf("email is required")
	}
	displayName := strings.TrimSpace(params.DisplayName)
	if displayName == "" {
		displayName = email
	}
	role := strings.TrimSpace(params.Role)
	if !ValidRole(role) {
		return UpsertMemberResult{}, fmt.Errorf("invalid role")
	}
	if _, err := s.queries.GetOrganization(ctx, params.OrgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UpsertMemberResult{}, ErrOrganizationNotFound
		}
		return UpsertMemberResult{}, err
	}

	now := s.now()
	user, err := s.queries.UpsertUserByEmail(ctx, db.UpsertUserByEmailParams{
		ID:          uuid.New(),
		Email:       email,
		DisplayName: displayName,
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return UpsertMemberResult{}, err
	}
	membership, err := s.queries.UpsertOrgMembership(ctx, db.UpsertOrgMembershipParams{
		ID:        uuid.New(),
		OrgID:     params.OrgID,
		UserID:    user.ID,
		Role:      role,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return UpsertMemberResult{}, err
	}
	return UpsertMemberResult{User: user, Membership: membership}, nil
}

func (s *SessionService) ListMembers(ctx context.Context, orgID uuid.UUID) ([]db.ListOrgMembersRow, error) {
	if orgID == uuid.Nil {
		return nil, ErrOrganizationNotFound
	}
	return s.queries.ListOrgMembers(ctx, orgID)
}

func (s *SessionService) PasswordlessMockLogin(ctx context.Context, params PasswordlessMockLoginParams) (PasswordlessMockLoginResult, error) {
	orgSlug := strings.TrimSpace(params.OrgSlug)
	email := normalizeEmail(params.Email)
	if orgSlug == "" || email == "" {
		return PasswordlessMockLoginResult{}, ErrMembershipNotFound
	}
	membership, err := s.queries.GetActiveMembershipByOrgSlugAndEmail(ctx, db.GetActiveMembershipByOrgSlugAndEmailParams{
		OrgSlug: orgSlug,
		Email:   email,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PasswordlessMockLoginResult{}, ErrMembershipNotFound
		}
		return PasswordlessMockLoginResult{}, err
	}

	token, err := generateToken(SessionPlainPrefix)
	if err != nil {
		return PasswordlessMockLoginResult{}, err
	}
	now := s.now()
	session, err := s.queries.CreateUserSession(ctx, db.CreateUserSessionParams{
		ID:          uuid.New(),
		OrgID:       membership.OrgID,
		UserID:      membership.UserID,
		TokenPrefix: visibleCredentialPrefix(token, SessionPlainPrefix),
		TokenHash:   s.hasher.Hash(token),
		Status:      "active",
		ExpiresAt:   now.Add(s.ttl),
		CreatedAt:   now,
	})
	if err != nil {
		return PasswordlessMockLoginResult{}, err
	}
	return PasswordlessMockLoginResult{
		Token:        token,
		Session:      session,
		OrgID:        membership.OrgID,
		OrgSlug:      membership.OrgSlug,
		UserID:       membership.UserID,
		Email:        membership.Email,
		DisplayName:  membership.DisplayName,
		MembershipID: membership.MembershipID,
		Role:         membership.Role,
	}, nil
}

func (s *SessionService) Authenticate(ctx context.Context, token string) (AdminTokenPrincipal, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AdminTokenPrincipal{}, ErrInvalidSession
	}
	session, err := s.queries.GetUserSessionByHash(ctx, s.hasher.Hash(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminTokenPrincipal{}, ErrInvalidSession
		}
		return AdminTokenPrincipal{}, err
	}
	if session.Status != "active" || session.MembershipStatus != "active" || session.UserStatus != "active" {
		return AdminTokenPrincipal{}, ErrInvalidSession
	}
	now := s.now()
	if !session.ExpiresAt.After(now) {
		return AdminTokenPrincipal{}, ErrInvalidSession
	}
	if err := s.queries.TouchUserSessionLastUsed(ctx, db.TouchUserSessionLastUsedParams{
		OrgID:      session.OrgID,
		ID:         session.ID,
		LastUsedAt: &now,
	}); err != nil {
		return AdminTokenPrincipal{}, err
	}

	userID := session.UserID
	membershipID := session.MembershipID
	sessionID := session.ID
	return AdminTokenPrincipal{
		OrgID:        session.OrgID,
		ActorType:    ActorUser,
		Role:         session.MembershipRole,
		UserID:       &userID,
		MembershipID: &membershipID,
		SessionID:    &sessionID,
		UserEmail:    session.UserEmail,
		DisplayName:  session.UserDisplayName,
	}, nil
}

func (s *SessionService) Revoke(ctx context.Context, orgID uuid.UUID, sessionID uuid.UUID) (db.UserSession, error) {
	now := s.now()
	return s.queries.RevokeUserSession(ctx, db.RevokeUserSessionParams{
		OrgID:     orgID,
		ID:        sessionID,
		RevokedAt: &now,
	})
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
