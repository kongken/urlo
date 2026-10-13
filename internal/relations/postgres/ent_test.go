package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kongken/urlo/internal/relations/ent/ownerlinkindex"
	"github.com/kongken/urlo/internal/relations/postgres"
	"github.com/kongken/urlo/internal/url"
)

func TestEntRefreshIsIdempotentAtDatabaseTimePrecision(t *testing.T) {
	client := testClient(t, testPool(t))
	index, err := postgres.New(client, "test", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	source := url.NewMemoryStore()
	r := &url.Record{Code: "abc", OwnerID: "alice", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.UTC)}
	if err := source.Create(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if err := index.Refresh(t.Context(), r.Code, source); err != nil {
		t.Fatal(err)
	}
	row, err := client.OwnerLinkIndex.Query().Only(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := client.OwnerLinkIndex.UpdateOneID(row.ID).SetIndexedAt(old).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	// A conditional Ent upsert returning no row is a successful no-op, not a
	// SQL failure; nanoseconds in the source must not trigger another write.
	for range 2 {
		if err := index.Refresh(t.Context(), r.Code, source); err != nil {
			t.Fatal(err)
		}
	}
	unchanged, err := client.OwnerLinkIndex.Query().Only(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.ID != row.ID || !unchanged.IndexedAt.Equal(old) {
		t.Fatalf("unchanged refresh wrote row: %+v", unchanged)
	}
	r.OwnerID = "bob"
	if err := source.Update(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if err := index.Refresh(t.Context(), r.Code, source); err != nil {
		t.Fatal(err)
	}
	changed, err := client.OwnerLinkIndex.Query().Only(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID != row.ID || changed.OwnerID != "bob" || !changed.IndexedAt.After(old) {
		t.Fatalf("changed refresh = %+v", changed)
	}
}

func TestEntCheckDoesNotRunSchemaMigration(t *testing.T) {
	pool := testPool(t)
	client := testClient(t, pool)
	index, err := postgres.New(client, "test", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// An existing V1 installation must be upgraded externally, not implicitly
	// changed by constructing/probing the new DAO.
	applyMigration(t, pool, "00002_owner_link_index_ent_id.sql", true)
	if err := index.Check(t.Context()); err == nil {
		t.Fatal("Check accepted schema without Ent ID")
	}
	var hasID bool
	if err := pool.QueryRow(t.Context(), `SELECT EXISTS (
SELECT 1 FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = 'owner_link_index' AND column_name = 'id'
)`).Scan(&hasID); err != nil {
		t.Fatal(err)
	}
	if hasID {
		t.Fatal("Check automatically added ID")
	}
	applyMigration(t, pool, "00002_owner_link_index_ent_id.sql", false)
	if err := index.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), "DROP TABLE owner_link_index"); err != nil {
		t.Fatal(err)
	}
	if err := index.Check(t.Context()); err == nil {
		t.Fatal("Check accepted absent table")
	}
	var absent bool
	if err := pool.QueryRow(t.Context(), "SELECT to_regclass('owner_link_index') IS NULL").Scan(&absent); err != nil {
		t.Fatal(err)
	}
	if !absent {
		t.Fatal("Check automatically created table")
	}
}

func TestEntTransactionTimeoutReleasesRefreshLock(t *testing.T) {
	pool := testPool(t)
	client := testClient(t, pool)
	index, err := postgres.New(client, "test", 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	source := url.NewMemoryStore()
	if err := source.Create(t.Context(), &url.Record{Code: "abc", OwnerID: "alice", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	paused := pausedGetter{source: source, read: make(chan struct{}), release: make(chan struct{})}
	defer close(paused.release)
	if err := index.Refresh(t.Context(), "abc", paused); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout = %v", err)
	}
	select {
	case <-paused.read:
	default:
		t.Fatal("timed out before entering source read")
	}
	next, err := postgres.New(client, "test", 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Refresh(t.Context(), "abc", source); err != nil {
		t.Fatalf("rollback did not release lock: %v", err)
	}
	row, err := client.OwnerLinkIndex.Query().Where(ownerlinkindex.CodeEQ("abc")).Only(t.Context())
	if err != nil || row.OwnerID != "alice" {
		t.Fatalf("retry row=%v error=%v", row, err)
	}
}
