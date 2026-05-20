package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func OpenPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		pool := openPoolOrSkip(t, ctx, dsn)
		t.Cleanup(pool.Close)
		return pool
	}

	adminDSN := os.Getenv("TEST_POSTGRES_ADMIN_URL")
	if adminDSN == "" {
		adminDSN = "postgres://llmgw:llmgw@localhost:5432/postgres?sslmode=disable"
	}

	admin := openPoolOrSkip(t, ctx, adminDSN)
	t.Cleanup(admin.Close)

	dbName := "llmgw_test_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+dbName); err != nil {
		t.Skipf("create test database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP DATABASE IF EXISTS `+dbName+` WITH (FORCE)`)
	})

	testDSN := strings.Replace(adminDSN, "/postgres?", "/"+dbName+"?", 1)
	if testDSN == adminDSN {
		t.Fatalf("TEST_POSTGRES_ADMIN_URL must point to postgres database, got %s", adminDSN)
	}

	pool := openPoolOrSkip(t, ctx, testDSN)
	t.Cleanup(pool.Close)
	applyInitialMigration(t, ctx, pool)
	return pool
}

func openPoolOrSkip(t *testing.T, ctx context.Context, dsn string) *pgxpool.Pool {
	t.Helper()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open test postgres pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test postgres unavailable at %s: %v", dsn, err)
	}
	return pool
}

func applyInitialMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration path: runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	migrationPath := filepath.Join(root, "db", "migrations", "000001_init.up.sql")

	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply initial migration: %v", err)
	}
}
