package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kongken/urlo/internal/config"
	"github.com/kongken/urlo/internal/relations"
	"github.com/kongken/urlo/internal/relations/postgres"
	"github.com/kongken/urlo/internal/url"
)

func buildRelations(ctx context.Context, c config.RelationsConfig, storage config.StorageConfig, source url.Store) (url.Store, func(), error) {
	if c.Driver == "" || c.Driver == "none" {
		return source, func() {}, nil
	}
	if c.Driver != "postgres" {
		return nil, nil, fmt.Errorf("unknown relations driver %q", c.Driver)
	}
	if storage.Driver != "s3" {
		return nil, nil, errors.New("relations.postgres requires storage.driver=s3; primary links stay on S3")
	}
	if strings.TrimSpace(c.LinkSpace) == "" || len(c.LinkSpace) > 256 || strings.ContainsRune(c.LinkSpace, '\x00') {
		return nil, nil, errors.New("relations.link_space is required and must be at most 256 bytes")
	}
	readSource := cmp.Or(c.OwnerIndexReadSource, relations.ReadS3)
	if readSource != relations.ReadS3 && readSource != relations.ReadPostgres {
		return nil, nil, errors.New("relations.owner_index_read_source must be s3 or postgres")
	}
	if c.BufferSize < 0 || c.BufferSize > 100000 || c.ReconcileSeconds < 0 || c.ReconcileSeconds > 86400 {
		return nil, nil, errors.New("invalid relations buffer_size or reconcile_interval_seconds")
	}
	pc := c.Postgres
	maxConns := cmp.Or(pc.MaxConns, 10)
	connectSeconds := cmp.Or(pc.ConnectTimeoutSeconds, 3)
	queryMS := cmp.Or(pc.QueryTimeoutMS, 1000)
	if maxConns < 1 || maxConns > 100 || connectSeconds < 1 || connectSeconds > 60 || queryMS < 1 || queryMS > 60000 {
		return nil, nil, errors.New("invalid relations.postgres connection limits or timeouts")
	}
	envName := cmp.Or(pc.DSNEnv, "URLO_POSTGRES_DSN")
	dsn := os.Getenv(envName)
	if dsn == "" {
		return nil, nil, fmt.Errorf("PostgreSQL DSN environment variable %q is empty", envName)
	}
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// Parse errors may embed the connection string and its password.
		return nil, nil, errors.New("invalid PostgreSQL DSN in relations.postgres.dsn_env")
	}
	poolConfig.MaxConns = int32(maxConns)
	poolConfig.MinConns = 0
	poolConfig.ConnConfig.ConnectTimeout = time.Duration(connectSeconds) * time.Second
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "urlo-owner-index"
	poolConfig.ConnConfig.RuntimeParams["timezone"] = "UTC"
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnLifetimeJitter = 5 * time.Minute
	poolConfig.PingTimeout = time.Duration(connectSeconds) * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, nil, errors.New("unable to create PostgreSQL connection pool")
	}
	index, err := postgres.New(pool, c.LinkSpace, time.Duration(queryMS)*time.Millisecond)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	store, err := relations.New(relations.Options{
		Source: source, Index: index, LinkSpace: c.LinkSpace, ReadSource: readSource,
		BufferSize: c.BufferSize, ReconcileInterval: time.Duration(c.ReconcileSeconds) * time.Second,
		OperationTimeout: time.Duration(max(queryMS, connectSeconds*1000)+5000) * time.Millisecond,
	})
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	closeResources := sync.OnceFunc(func() {
		store.Close()
		pool.Close()
	})
	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(connectSeconds)*time.Second)
	probeErr := pool.Ping(probeCtx)
	cancel()
	if probeErr == nil {
		probeErr = index.Check(ctx)
	}
	if probeErr != nil {
		slog.Warn("PostgreSQL owner index unavailable; core S3 operations remain enabled", "error", probeErr)
	}
	store.Start(ctx)
	slog.Info("owner index configured", "read_source", readSource, "link_space", c.LinkSpace)
	return store, closeResources, nil
}
