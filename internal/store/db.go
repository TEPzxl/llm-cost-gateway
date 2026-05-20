package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

type Store struct {
	Pool    *pgxpool.Pool
	Queries *db.Queries
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return New(pool), nil
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		Pool:    pool,
		Queries: db.New(pool),
	}
}

func (s *Store) Close() {
	s.Pool.Close()
}
