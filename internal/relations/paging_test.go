package relations_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kongken/urlo/internal/relations"
	"github.com/kongken/urlo/internal/url"
)

// candidateIndex simulates an external index containing stale candidate refs.
type candidateIndex struct{ refs []relations.Ref }

func (i candidateIndex) Refresh(context.Context, string, relations.RecordGetter) error { return nil }
func (i candidateIndex) ScanCodes(context.Context, string, int) ([]string, error)      { return nil, nil }
func (i candidateIndex) List(_ context.Context, _ string, after *url.OwnerCursor, limit int) ([]relations.Ref, error) {
	var out []relations.Ref
	for _, ref := range i.refs {
		if after != nil && (ref.CreatedAt.After(after.CreatedAt) || ref.CreatedAt.Equal(after.CreatedAt) && ref.Code >= after.Code) {
			continue
		}
		out = append(out, ref)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func TestEntirelyStalePageAdvancesUntilLiveRecord(t *testing.T) {
	source := url.NewMemoryStore()
	now := time.Now().UTC()
	if err := source.Create(t.Context(), &url.Record{Code: "0000", OwnerID: "alice", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	index := candidateIndex{}
	for n := 500; n >= 0; n-- {
		index.refs = append(index.refs, relations.Ref{Code: fmt.Sprintf("%04d", n), CreatedAt: now})
	}
	store, err := relations.New(relations.Options{Source: source, Index: index, LinkSpace: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	first, err := store.ListByOwnerPage(t.Context(), "alice", url.PageOptions{PageSize: 1})
	if err != nil || len(first.Records) != 0 || first.NextPageToken == "" {
		t.Fatalf("stale page = %+v %v", first, err)
	}
	second, err := store.ListByOwnerPage(t.Context(), "alice", url.PageOptions{PageSize: 1, PageToken: first.NextPageToken})
	if err != nil || len(second.Records) != 1 || second.Records[0].Code != "0000" || second.NextPageToken != "" {
		t.Fatalf("live page = %+v %v", second, err)
	}
}

type unreadableSource struct{ *url.MemoryStore }

func (unreadableSource) Get(context.Context, string) (*url.Record, error) {
	return nil, errors.New("source outage")
}

func TestSourceErrorsAreNotSilentlySkippedAsMissingLinks(t *testing.T) {
	store, err := relations.New(relations.Options{
		Source: unreadableSource{url.NewMemoryStore()},
		Index:  candidateIndex{refs: []relations.Ref{{Code: "abc", CreatedAt: time.Now().UTC()}}}, LinkSpace: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if _, err := store.ListByOwnerPage(t.Context(), "alice", url.PageOptions{}); err == nil {
		t.Fatal("source outage must not become an empty successful list")
	}
}
