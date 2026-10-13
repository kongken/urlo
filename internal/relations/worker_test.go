package relations_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kongken/urlo/internal/relations"
	"github.com/kongken/urlo/internal/url"
)

type waitingIndex struct {
	started chan struct{}
	once    sync.Once
}

func (i *waitingIndex) Refresh(ctx context.Context, _ string, _ relations.RecordGetter) error {
	i.once.Do(func() { close(i.started) })
	<-ctx.Done()
	return ctx.Err()
}
func (*waitingIndex) List(context.Context, string, *url.OwnerCursor, int) ([]relations.Ref, error) {
	return nil, nil
}
func (*waitingIndex) ScanCodes(context.Context, string, int) ([]string, error) { return nil, nil }

func TestCloseCancelsBlockedWorkerAndIsIdempotent(t *testing.T) {
	index := &waitingIndex{started: make(chan struct{})}
	source := url.NewMemoryStore()
	store, err := relations.New(relations.Options{Source: source, Index: index, LinkSpace: "test", OperationTimeout: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Create(t.Context(), &url.Record{Code: "abc", OwnerID: "alice", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	store.Start(t.Context())
	select {
	case <-index.started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	closed := make(chan struct{})
	go func() { store.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not cancel blocked database operation")
	}
	store.Close()
	store.Start(t.Context()) // a closed store must not restart workers
}
