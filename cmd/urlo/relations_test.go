package main

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/kongken/urlo/internal/config"
	"github.com/kongken/urlo/internal/url"
)

func TestRelationsDisabledKeepsPrimaryStore(t *testing.T) {
	for _, driver := range []string{"", "none"} {
		store := url.NewMemoryStore()
		got, closeStore, err := buildRelations(t.Context(), config.RelationsConfig{Driver: driver}, config.StorageConfig{}, store)
		if err != nil || got != store {
			t.Fatalf("driver=%q got=%v err=%v", driver, got, err)
		}
		closeStore()
	}
}

func TestRelationsConfigRejectsInvalidValuesAndRedactsDSN(t *testing.T) {
	base := config.RelationsConfig{Driver: "postgres", LinkSpace: "test", Postgres: config.PostgresRelationsConfig{DSNEnv: "URLO_UNIT_PG_DSN"}}
	t.Setenv("URLO_UNIT_PG_DSN", "postgres://user:secret-password@%invalid/test")
	for _, change := range []func(*config.RelationsConfig){
		func(c *config.RelationsConfig) { c.Driver = "mongo" },
		func(c *config.RelationsConfig) { c.LinkSpace = "" },
		func(c *config.RelationsConfig) { c.OwnerIndexReadSource = "invalid" },
		func(c *config.RelationsConfig) { c.BufferSize = -1 },
		func(c *config.RelationsConfig) { c.Postgres.MaxConns = -1 },
		func(c *config.RelationsConfig) {},
	} {
		c := base
		change(&c)
		_, _, err := buildRelations(t.Context(), c, config.StorageConfig{Driver: "s3"}, url.NewMemoryStore())
		if err == nil || strings.Contains(err.Error(), "secret-password") {
			t.Fatalf("config error = %v", err)
		}
	}
	if _, _, err := buildRelations(t.Context(), base, config.StorageConfig{Driver: "memory"}, url.NewMemoryStore()); err == nil {
		t.Fatal("PostgreSQL must not replace the primary link backend")
	}
}

func TestDatabaseOfflineStillStartsWithPrimaryStore(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	t.Setenv("URLO_UNIT_PG_DSN", "postgres://test@"+addr+"/test?sslmode=disable")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	store, closeStore, err := buildRelations(ctx, config.RelationsConfig{
		Driver: "postgres", LinkSpace: "test", OwnerIndexReadSource: "postgres",
		Postgres: config.PostgresRelationsConfig{DSNEnv: "URLO_UNIT_PG_DSN", ConnectTimeoutSeconds: 1, QueryTimeoutMS: 100},
	}, config.StorageConfig{Driver: "s3"}, url.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeStore)
	if err := store.Create(t.Context(), &url.Record{Code: "abc"}); err != nil {
		t.Fatalf("primary create while database offline: %v", err)
	}
	if _, err := store.Get(t.Context(), "abc"); err != nil {
		t.Fatal(err)
	}
	closeStore()
	closeStore() // lifecycle cleanup is idempotent
}
