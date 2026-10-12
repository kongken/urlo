package postgres_test

import (
	"context"
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/kongken/urlo/internal/relations/ent"
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
	for _, file := range []string{"00001_owner_link_index.sql", "00002_owner_link_index_ent_id.sql"} {
		applyMigration(t, pool, file, false)
	}
	return pool
}

func applyMigration(t *testing.T, pool *pgxpool.Pool, file string, down bool) {
	t.Helper()
	migration, err := os.ReadFile("../../../migrations/postgres/" + file)
	if err != nil {
		t.Fatal(err)
	}
	upSQL, downSQL, found := strings.Cut(string(migration), "-- +goose Down")
	if !found {
		t.Fatal("migration has no Down section")
	}
	statement := upSQL
	if down {
		statement = downSQL
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(ctx)
	}()
	if _, err := tx.Exec(t.Context(), statement); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func testClient(t *testing.T, pool *pgxpool.Pool) *ent.Client {
	t.Helper()
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, stdlib.OpenDBFromPool(pool))))
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close Ent client: %v", err)
		}
	})
	return client
}

func TestRefreshStoresOnlyOwnerRefs(t *testing.T) {
	client := testClient(t, testPool(t))
	index, err := postgres.New(client, "production", time.Second)
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
