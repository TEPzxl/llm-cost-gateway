package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

func (s *Store) ExecTx(ctx context.Context, fn func(*db.Queries) error) error {
	return s.ExecTxRaw(ctx, func(_ pgx.Tx, q *db.Queries) error {
		return fn(q)
	})
}

func (s *Store) ExecTxRaw(ctx context.Context, fn func(pgx.Tx, *db.Queries) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	q := db.New(tx)
	if err := fn(tx, q); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}
