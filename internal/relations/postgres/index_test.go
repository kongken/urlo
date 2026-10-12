package postgres_test

import (
	"context"
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kongken/urlo/internal/relations/postgres"
	"github.com/kongken/urlo/internal/url"
)

// Every test gets its own schema; never migrate or truncate a caller's tables.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("URLO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set URLO_TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 4
	admin, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "urlo_test_" + strings.ToLower(rand.Text())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("clean test schema: %v", err)
		}
	})
	cfg = cfg.Copy()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migration, err := os.ReadFile("../../../migrations/postgres/00001_owner_link_index.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, _, _ := strings.Cut(string(migration), "-- +goose Down")
	if _, err := pool.Exec(t.Context(), up); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestRefreshStoresOnlyOwnerRefs(t *testing.T) {
	pool := testPool(t)
	index, err := postgres.New(pool, "production", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	source := url.NewMemoryStore()
	if err := source.Create(t.Context(), &url.Record{Code: "abc", OwnerID: "alice", CreatedAt: time.Now().UTC(), LongURL: "https://example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := index.Refresh(t.Context(), "abc", source); err != nil {
		t.Fatal(err)
	}
	refs, err := index.List(t.Context(), "alice", nil, 10)
	if err != nil || len(refs) != 1 || refs[0].Code != "abc" {
		t.Fatalf("owner refs = %+v, error = %v", refs, err)
	}
}
