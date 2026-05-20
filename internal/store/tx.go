package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

func (s *Store) ExecTx(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	q := db.New(tx)
	if err := fn(q); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}
