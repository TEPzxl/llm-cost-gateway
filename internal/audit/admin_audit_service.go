package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type AdminAuditService struct {
	queries *db.Queries
	now     func() time.Time
}

type AdminAuditOption func(*AdminAuditService)

func WithClock(clock func() time.Time) AdminAuditOption {
	return func(s *AdminAuditService) {
		if clock != nil {
			s.now = clock
		}
	}
}

func NewAdminAuditService(queries *db.Queries, opts ...AdminAuditOption) *AdminAuditService {
	service := &AdminAuditService{
		queries: queries,
		now:     func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

type RecordInput struct {
	OrgID             uuid.UUID
	ActorAdminTokenID *uuid.UUID
	ActorUserID       *uuid.UUID
	Action            string
	ResourceType      string
	ResourceID        *uuid.UUID
	RequestID         string
}

func (s *AdminAuditService) Record(ctx context.Context, input RecordInput) (db.AdminAuditLog, error) {
	if input.OrgID == uuid.Nil {
		return db.AdminAuditLog{}, fmt.Errorf("org_id is required")
	}
	if (input.ActorAdminTokenID == nil || *input.ActorAdminTokenID == uuid.Nil) &&
		(input.ActorUserID == nil || *input.ActorUserID == uuid.Nil) {
		return db.AdminAuditLog{}, fmt.Errorf("actor is required")
	}
	if input.ActorAdminTokenID != nil && input.ActorUserID != nil {
		return db.AdminAuditLog{}, fmt.Errorf("only one actor is allowed")
	}
	if input.Action == "" {
		return db.AdminAuditLog{}, fmt.Errorf("action is required")
	}
	if input.ResourceType == "" {
		return db.AdminAuditLog{}, fmt.Errorf("resource_type is required")
	}
	if input.RequestID == "" {
		input.RequestID = uuid.NewString()
	}
	return s.queries.InsertAdminAuditLog(ctx, db.InsertAdminAuditLogParams{
		ID:                uuid.New(),
		OrgID:             input.OrgID,
		ActorAdminTokenID: input.ActorAdminTokenID,
		ActorUserID:       input.ActorUserID,
		Action:            input.Action,
		ResourceType:      input.ResourceType,
		ResourceID:        input.ResourceID,
		RequestID:         input.RequestID,
		CreatedAt:         s.now(),
	})
}

func (s *AdminAuditService) List(ctx context.Context, orgID uuid.UUID, limit int32, offset int32) ([]db.AdminAuditLog, error) {
	return s.queries.ListAdminAuditLogs(ctx, db.ListAdminAuditLogsParams{
		OrgID:  orgID,
		Limit:  limit,
		Offset: offset,
	})
}
