package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/kongken/urlo/migrations"
	"github.com/pressly/goose/v3/lock"
)

func isolatedDatabase(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("URLO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set URLO_TEST_POSTGRES_DSN for migration integration tests")
	}
	uri, err := url.Parse(dsn)
	if err != nil || (uri.Scheme != "postgres" && uri.Scheme != "postgresql") {
		t.Fatal("migration tests require a PostgreSQL URL DSN")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test DSN")
	}
	admin := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = admin.Close() })
	schema := "urlo_migrate_test_" + strings.ToLower(rand.Text())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	query := uri.Query()
	query.Set("search_path", schema)
	uri.RawQuery = query.Encode()
	dsn = uri.String()
	cfg, err = pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*cfg)
	t.Cleanup(func() { _ = db.Close() })
	return dsn, db
}

func TestEmbeddedMigrationsAndRepeatedRuns(t *testing.T) {
	dsn, db := isolatedDatabase(t)
	first, err := migrate(t.Context(), dsn)
	if err != nil || first.Applied != 2 || first.Version != 2 {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	second, err := migrate(t.Context(), dsn)
	if err != nil || second.Applied != 0 || second.Version != 2 {
		t.Fatalf("second=%+v error=%v", second, err)
	}
	var hasID bool
	if err := db.QueryRowContext(t.Context(), `SELECT EXISTS (SELECT 1 FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = 'owner_link_index' AND column_name = 'id')`).Scan(&hasID); err != nil || !hasID {
		t.Fatalf("Ent ID=%v error=%v", hasID, err)
	}
	var output strings.Builder
	if err := run(t.Context(), []string{"--dsn-env=TEST_MIGRATE_DSN"}, func(name string) string {
		if name != "TEST_MIGRATE_DSN" {
			t.Fatalf("env=%s", name)
		}
		return dsn
	}, &output, &output); err != nil || !strings.Contains(output.String(), "applied=0 version=2") {
		t.Fatalf("command error=%v output=%s", err, &output)
	}
}

func TestConcurrentMigrationProcessesSerialize(t *testing.T) {
	dsn, _ := isolatedDatabase(t)
	var wg sync.WaitGroup
	results := make(chan migrationResult, 3)
	failures := make(chan error, 3)
	for range 3 {
		wg.Go(func() {
			result, err := migrate(t.Context(), dsn)
			results <- result
			failures <- err
		})
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	total := 0
	for result := range results {
		total += result.Applied
		if result.Version != 2 {
			t.Fatalf("version=%d", result.Version)
		}
	}
	if total != 2 {
		t.Fatalf("migrations applied %d times", total)
	}
}

func TestMigrationFailureRollsBackAndReleasesLock(t *testing.T) {
	dsn, db := isolatedDatabase(t)
	files := fstest.MapFS{"00001_failure.sql": &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE must_rollback(id integer);\nSELECT 1/0;\n-- +goose Down\nDROP TABLE must_rollback;\n")}}
	if _, err := apply(t.Context(), db, files); err == nil {
		t.Fatal("failed migration reported success")
	}
	var absent bool
	if err := db.QueryRowContext(t.Context(), "SELECT to_regclass('must_rollback') IS NULL").Scan(&absent); err != nil || !absent {
		t.Fatalf("rollback=%v error=%v", absent, err)
	}
	if result, err := migrate(t.Context(), dsn); err != nil || result.Version != 2 {
		t.Fatalf("retry=%+v error=%v", result, err)
	}
}

func TestLockWaitHonorsDeadline(t *testing.T) {
	dsn, db := isolatedDatabase(t)
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(t.Context(), "SELECT pg_advisory_lock($1)", lock.DefaultLockID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	if _, err := migrate(ctx, dsn); err == nil {
		t.Fatal("ignored migration lock deadline")
	}
	if _, err := conn.ExecContext(t.Context(), "SELECT pg_advisory_unlock($1)", lock.DefaultLockID); err != nil {
		t.Fatal(err)
	}
	if result, err := migrate(t.Context(), dsn); err != nil || result.Version != 2 {
		t.Fatalf("after unlock=%+v error=%v", result, err)
	}
}

func TestRefusesNewerDatabaseWithoutDowngrading(t *testing.T) {
	dsn, db := isolatedDatabase(t)
	if _, err := migrate(t.Context(), dsn); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "INSERT INTO goose_db_version(version_id, is_applied) VALUES (999, true)"); err != nil {
		t.Fatal(err)
	}
	if _, err := migrate(t.Context(), dsn); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("ahead schema error=%v", err)
	}
}

func TestCancelledContextDoesNotApplyDDL(t *testing.T) {
	_, db := isolatedDatabase(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	files, err := migrations.Postgres()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := apply(ctx, db, files); err == nil {
		t.Fatal("ignored cancelled context")
	}
}
