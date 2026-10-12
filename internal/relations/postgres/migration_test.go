package postgres_test

import (
	"testing"
	"time"

	"github.com/kongken/urlo/internal/relations/ent/ownerlinkindex"
	"github.com/kongken/urlo/internal/relations/postgres"
	"github.com/kongken/urlo/internal/url"
)

func TestProjectionHasUniqueEntIDsWithoutChangingCodeNamespace(t *testing.T) {
	pool := testPool(t)
	client := testClient(t, pool)
	source := url.NewMemoryStore()
	if err := source.Create(t.Context(), &url.Record{Code: "abc", OwnerID: "alice", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	for _, space := range []string{"alpha", "beta"} {
		index, err := postgres.New(client, space, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err := index.Refresh(t.Context(), "abc", source); err != nil {
			t.Fatal(err)
		}
	}
	var total, ids int
	if err := pool.QueryRow(t.Context(), "SELECT count(*), count(DISTINCT id) FROM owner_link_index WHERE code = 'abc'").Scan(&total, &ids); err != nil {
		t.Fatal(err)
	}
	if total != 2 || ids != 2 {
		t.Fatalf("rows=%d unique IDs=%d", total, ids)
	}
}

func TestEntMigrationPreservesExistingRowsAcrossUpgradeAndRollback(t *testing.T) {
	pool := testPool(t)
	applyMigration(t, pool, "00002_owner_link_index_ent_id.sql", true)
	created := time.Date(2026, 1, 1, 0, 0, 0, 123456000, time.UTC)
	for _, space := range []string{"alpha", "beta"} {
		if _, err := pool.Exec(t.Context(), `INSERT INTO owner_link_index (link_space, code, owner_id, link_created_at) VALUES ($1, 'abc', 'alice', $2)`, space, created); err != nil {
			t.Fatal(err)
		}
	}
	applyMigration(t, pool, "00002_owner_link_index_ent_id.sql", false)
	client := testClient(t, pool)
	for _, space := range []string{"alpha", "beta"} {
		row, err := client.OwnerLinkIndex.Query().Where(ownerlinkindex.LinkSpaceEQ(space)).Only(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if row.ID <= 0 || row.Code != "abc" || row.OwnerID != "alice" || !row.LinkCreatedAt.Equal(created) {
			t.Fatalf("migrated row = %+v", row)
		}
	}
	// The pre-Ent release's conflict target remains valid during rollout.
	if _, err := pool.Exec(t.Context(), `INSERT INTO owner_link_index (link_space, code, owner_id, link_created_at)
VALUES ('alpha', 'abc', 'bob', $1)
ON CONFLICT (link_space, code) DO UPDATE SET owner_id = EXCLUDED.owner_id`, created); err != nil {
		t.Fatal(err)
	}
	applyMigration(t, pool, "00002_owner_link_index_ent_id.sql", true)
	var rows int
	var owner string
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM owner_link_index").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), "SELECT owner_id FROM owner_link_index WHERE link_space = 'alpha' AND code = 'abc'").Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if rows != 2 || owner != "bob" {
		t.Fatalf("rollback rows=%d owner=%q", rows, owner)
	}
	applyMigration(t, pool, "00002_owner_link_index_ent_id.sql", false)
	reapplied, err := client.OwnerLinkIndex.Query().Where(ownerlinkindex.OwnerIDEQ("bob")).Only(t.Context())
	if err != nil || reapplied.OwnerID != "bob" {
		t.Fatalf("reapplied row=%v error=%v", reapplied, err)
	}
}
