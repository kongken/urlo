package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kongken/urlo/internal/relations"
	"github.com/kongken/urlo/internal/relations/postgres"
	"github.com/kongken/urlo/internal/url"
)

func TestCaseSensitiveCodesScopeAndMicrosecondCursor(t *testing.T) {
	pool := testPool(t)
	index, _ := postgres.New(pool, "alpha", time.Second)
	other, _ := postgres.New(pool, "beta", time.Second)
	source := url.NewMemoryStore()
	ts := time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.UTC)
	for _, code := range []string{"Abc", "abc"} {
		if err := source.Create(t.Context(), &url.Record{Code: code, OwnerID: "alice", CreatedAt: ts}); err != nil {
			t.Fatal(err)
		}
		if err := index.Refresh(t.Context(), code, source); err != nil {
			t.Fatal(err)
		}
	}
	if err := other.Refresh(t.Context(), "abc", source); err != nil {
		t.Fatal(err)
	}
	first, err := index.List(t.Context(), "alice", nil, 1)
	if err != nil || len(first) != 1 || first[0].Code != "abc" || first[0].CreatedAt.Nanosecond()%1000 != 0 {
		t.Fatalf("first = %+v, error=%v", first, err)
	}
	second, err := index.List(t.Context(), "alice", &url.OwnerCursor{Code: first[0].Code, CreatedAt: first[0].CreatedAt}, 1)
	if err != nil || len(second) != 1 || second[0].Code != "Abc" {
		t.Fatalf("second = %+v, error=%v", second, err)
	}
	refs, err := other.List(t.Context(), "alice", nil, 10)
	if err != nil || len(refs) != 1 {
		t.Fatalf("other space = %+v, error=%v", refs, err)
	}
}

func TestOwnerPageFiltersStaleRefsWithoutLosingLiveRecords(t *testing.T) {
	pool := testPool(t)
	index, _ := postgres.New(pool, "test", 3*time.Second)
	source := url.NewMemoryStore()
	now := time.Now().UTC()
	for _, r := range []*url.Record{
		{Code: "missing", OwnerID: "alice", CreatedAt: now.Add(3 * time.Second)},
		{Code: "reused", OwnerID: "alice", CreatedAt: now.Add(2 * time.Second)},
		{Code: "expired", OwnerID: "alice", CreatedAt: now.Add(time.Second), ExpiresAt: now.Add(-time.Hour)},
		{Code: "valid", OwnerID: "alice", CreatedAt: now, Disabled: true},
	} {
		if err := source.Create(t.Context(), r); err != nil {
			t.Fatal(err)
		}
		if err := index.Refresh(t.Context(), r.Code, source); err != nil {
			t.Fatal(err)
		}
	}
	if err := source.Delete(t.Context(), "missing"); err != nil {
		t.Fatal(err)
	}
	if err := source.Update(t.Context(), &url.Record{Code: "reused", OwnerID: "bob", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	store, err := relations.New(relations.Options{Source: source, Index: index, LinkSpace: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	page, err := store.ListByOwnerPage(t.Context(), "alice", url.PageOptions{PageSize: 1})
	if err != nil || len(page.Records) != 1 || page.Records[0].Code != "valid" || page.NextPageToken != "" {
		t.Fatalf("hydrated page = %+v, error=%v", page, err)
	}
	// A different owner/space cannot reuse this ordering cursor.
	foreign := url.EncodeOwnerCursor("test", "bob", url.OwnerCursor{Code: "valid", CreatedAt: now})
	if _, err := store.ListByOwnerPage(t.Context(), "alice", url.PageOptions{PageToken: foreign}); !errors.Is(err, url.ErrInvalidPage) {
		t.Fatalf("foreign cursor = %v", err)
	}
}

type failedGetter struct{}

func (failedGetter) Get(context.Context, string) (*url.Record, error) {
	return nil, errors.New("S3 temporarily unavailable")
}

func TestRefreshDoesNotDeleteOnSourceErrorsAndRepairsCodeReuse(t *testing.T) {
	pool := testPool(t)
	index, _ := postgres.New(pool, "test", time.Second)
	source := url.NewMemoryStore()
	if err := source.Create(t.Context(), &url.Record{Code: "abc", OwnerID: "alice", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := index.Refresh(t.Context(), "abc", source); err != nil {
		t.Fatal(err)
	}
	if err := index.Refresh(t.Context(), "abc", failedGetter{}); err == nil {
		t.Fatal("expected source error")
	}
	refs, err := index.List(t.Context(), "alice", nil, 10)
	if err != nil || len(refs) != 1 {
		t.Fatalf("source failure deleted projection: %v %v", refs, err)
	}
	if err := source.Delete(t.Context(), "abc"); err != nil {
		t.Fatal(err)
	}
	if err := source.Create(t.Context(), &url.Record{Code: "abc", OwnerID: "bob", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	// A delayed deletion signal rereads the current object, not its old owner.
	if err := index.Refresh(t.Context(), "abc", source); err != nil {
		t.Fatal(err)
	}
	refs, err = index.List(t.Context(), "bob", nil, 10)
	if err != nil || len(refs) != 1 {
		t.Fatalf("reused code = %v %v", refs, err)
	}
	if err := source.Delete(t.Context(), "abc"); err != nil {
		t.Fatal(err)
	}
	if err := index.Refresh(t.Context(), "abc", source); err != nil {
		t.Fatal(err)
	}
	refs, err = index.List(t.Context(), "bob", nil, 10)
	if err != nil || len(refs) != 0 {
		t.Fatalf("deleted code = %v %v", refs, err)
	}
}

func TestReconcileRepairsLostQueueWorkAndOrphans(t *testing.T) {
	pool := testPool(t)
	index, _ := postgres.New(pool, "test", time.Second)
	source := url.NewMemoryStore()
	store, err := relations.New(relations.Options{Source: source, Index: index, LinkSpace: "test", BufferSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	for _, code := range []string{"a", "b", "c"} {
		if err := store.Create(t.Context(), &url.Record{Code: code, OwnerID: "alice", CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := source.Delete(t.Context(), "b"); err != nil { // no notification
		t.Fatal(err)
	}
	if err := store.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	refs, err := index.List(t.Context(), "alice", nil, 10)
	if err != nil || len(refs) != 2 || store.Stats().Dropped != 2 {
		t.Fatalf("repaired refs=%v stats=%+v error=%v", refs, store.Stats(), err)
	}
}

type pausedGetter struct {
	source  *url.MemoryStore
	read    chan struct{}
	release chan struct{}
}

func (g pausedGetter) Get(ctx context.Context, code string) (*url.Record, error) {
	r, err := g.source.Get(ctx, code)
	close(g.read)
	select {
	case <-g.release:
		return r, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestConcurrentRefreshCannotOverwriteNewerOwnerWithDelayedRead(t *testing.T) {
	pool := testPool(t)
	first, _ := postgres.New(pool, "test", 5*time.Second)
	second, _ := postgres.New(pool, "test", 5*time.Second)
	source := url.NewMemoryStore()
	if err := source.Create(t.Context(), &url.Record{Code: "abc", OwnerID: "alice", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	paused := pausedGetter{source: source, read: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(paused.release) })
	t.Cleanup(release)
	oldDone := make(chan error, 1)
	go func() { oldDone <- first.Refresh(t.Context(), "abc", paused) }()
	select {
	case <-paused.read:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not read source")
	}
	if err := source.Update(t.Context(), &url.Record{Code: "abc", OwnerID: "bob", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	newDone := make(chan error, 1)
	go func() { newDone <- second.Refresh(t.Context(), "abc", source) }()
	select {
	case err := <-newDone:
		t.Fatalf("refresh escaped per-key serialization: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	if err := <-oldDone; err != nil {
		t.Fatal(err)
	}
	if err := <-newDone; err != nil {
		t.Fatal(err)
	}
	refs, err := second.List(t.Context(), "bob", nil, 10)
	if err != nil || len(refs) != 1 {
		t.Fatalf("new owner overwritten: %v %v", refs, err)
	}
}
