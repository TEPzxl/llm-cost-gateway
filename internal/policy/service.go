package policy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	ActionAllow  = "allow"
	ActionRedact = "redact"
	ActionBlock  = "block"
)

type Service struct {
	store *store.Store
	now   func() time.Time
}

type CreateContentPolicyParams struct {
	OrgID     uuid.UUID
	Name      string
	PIIAction string
}

func NewService(st *store.Store) *Service {
	return &Service{
		store: st,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) CreateContentPolicy(ctx context.Context, params CreateContentPolicyParams) (db.ContentPolicy, error) {
	if params.OrgID == uuid.Nil {
		return db.ContentPolicy{}, fmt.Errorf("org_id is required")
	}
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return db.ContentPolicy{}, fmt.Errorf("name is required")
	}
	if !validAction(params.PIIAction) {
		return db.ContentPolicy{}, fmt.Errorf("pii_action must be allow, redact or block")
	}
	now := s.now()
	return s.store.Queries.CreateContentPolicy(ctx, db.CreateContentPolicyParams{
		ID:        uuid.New(),
		OrgID:     params.OrgID,
		Name:      name,
		PiiAction: params.PIIAction,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (s *Service) ListContentPolicies(ctx context.Context, orgID uuid.UUID) ([]db.ContentPolicy, error) {
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("org_id is required")
	}
	return s.store.Queries.ListContentPolicies(ctx, orgID)
}

func (s *Service) ActivePIIAction(ctx context.Context, orgID uuid.UUID) (string, error) {
	if orgID == uuid.Nil {
		return ActionAllow, fmt.Errorf("org_id is required")
	}
	item, err := s.store.Queries.GetActiveContentPolicy(ctx, orgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ActionAllow, nil
		}
		return ActionAllow, err
	}
	if !validAction(item.PiiAction) {
		return ActionAllow, nil
	}
	return item.PiiAction, nil
}

func validAction(action string) bool {
	switch action {
	case ActionAllow, ActionRedact, ActionBlock:
		return true
	default:
		return false
	}
}
