package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/kongken/urlo/migrations"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

type migrationResult struct {
	Applied int
	Version int64
}

func migrate(ctx context.Context, dsn string) (migrationResult, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return migrationResult{}, errors.New("invalid PostgreSQL DSN (connection details redacted)")
	}
	cfg.ConnectTimeout = 5 * time.Second
	cfg.RuntimeParams["application_name"] = "urlo-migrate"
	cfg.RuntimeParams["timezone"] = "UTC"
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	// Each provider leases one connection for both locking and migration DDL.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	files, err := migrations.Postgres()
	if err != nil {
		return migrationResult{}, err
	}
	result, err := apply(ctx, db, files)
	if err != nil {
		return result, &redactedError{cause: err, dsn: dsn, password: cfg.Password}
	}
	return result, nil
}

func apply(ctx context.Context, db *sql.DB, files fs.FS) (migrationResult, error) {
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(1, 3600), lock.WithUnlockTimeout(1, 3))
	if err != nil {
		return migrationResult{}, err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files,
		goose.WithSessionLocker(boundedLocker{locker}),
		goose.WithDisableGlobalRegistry(true), goose.WithLogger(goose.NopLogger()),
	)
	if err != nil {
		return migrationResult{}, err
	}
	current, target, err := provider.GetVersions(ctx)
	if err != nil {
		return migrationResult{}, err
	}
	if current > target {
		return migrationResult{}, fmt.Errorf("database version %d is newer than bundled version %d; refusing release downgrade", current, target)
	}
	applied, err := provider.Up(ctx)
	if err != nil {
		return migrationResult{}, err
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		return migrationResult{}, err
	}
	if version != target {
		return migrationResult{}, fmt.Errorf("database version %d does not match bundled version %d", version, target)
	}
	return migrationResult{Applied: len(applied), Version: version}, nil
}

// Goose detaches unlock from the migration context; still bound its cleanup.
type boundedLocker struct{ lock.SessionLocker }

func (l boundedLocker) SessionUnlock(parent context.Context, conn *sql.Conn) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	return l.SessionLocker.SessionUnlock(ctx, conn)
}

// Keep useful failures and error identity without exposing DSN/password values.
type redactedError struct {
	cause         error
	dsn, password string
}

func (e *redactedError) Error() string {
	message := strings.ReplaceAll(e.cause.Error(), e.dsn, "[redacted DSN]")
	if e.password != "" {
		message = strings.ReplaceAll(message, e.password, "[redacted password]")
	}
	return message
}
func (e *redactedError) Unwrap() error { return e.cause }
