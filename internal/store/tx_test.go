package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

func TestStoreExecTxRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t, ctx)
	resetTestDatabase(t, ctx, st)

	orgID := uuid.New()
	expectedErr := errors.New("force rollback")

	err := st.ExecTx(ctx, func(q *db.Queries) error {
		_, createErr := q.CreateOrganization(ctx, db.CreateOrganizationParams{
			ID:        orgID,
			Name:      "Rollback Org",
			Slug:      "rollback-org",
			Status:    "active",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})
		if createErr != nil {
			return createErr
		}
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("ExecTx error = %v, want %v", err, expectedErr)
	}

	_, err = st.Queries.GetOrganization(ctx, orgID)
	if err == nil {
		t.Fatal("GetOrganization after rollback returned nil error, want not found")
	}
}
