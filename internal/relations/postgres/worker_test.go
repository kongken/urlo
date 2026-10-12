package postgres_test

import (
	"testing"
	"time"

	"github.com/kongken/urlo/internal/relations"
	"github.com/kongken/urlo/internal/relations/postgres"
	"github.com/kongken/urlo/internal/url"
)

func TestBackgroundWorkerMakesCreatedLinksVisible(t *testing.T) {
	client := testClient(t, testPool(t))
	index, _ := postgres.New(client, "test", time.Second)
	store, err := relations.New(relations.Options{Source: url.NewMemoryStore(), Index: index, LinkSpace: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	store.Start(t.Context())
	for _, code := range []string{"a", "b"} {
		if err := store.Create(t.Context(), &url.Record{Code: code, OwnerID: "alice", CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		refs, err := index.List(t.Context(), "alice", nil, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(refs) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background index = %v", refs)
		}
		time.Sleep(10 * time.Millisecond)
	}
	first, err := store.ListByOwnerPage(t.Context(), "alice", url.PageOptions{PageSize: 1})
	if err != nil || len(first.Records) != 1 || first.NextPageToken == "" {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	second, err := store.ListByOwnerPage(t.Context(), "alice", url.PageOptions{PageSize: 1, PageToken: first.NextPageToken})
	if err != nil || len(second.Records) != 1 || second.NextPageToken != "" || second.Records[0].Code == first.Records[0].Code {
		t.Fatalf("second=%+v error=%v", second, err)
	}
}
