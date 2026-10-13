package relations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kongken/urlo/internal/relations"
	"github.com/kongken/urlo/internal/url"
	urlov1 "github.com/kongken/urlo/pkg/proto/urlo/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type unavailableIndex struct{ calls int }

func (i *unavailableIndex) Refresh(context.Context, string, relations.RecordGetter) error {
	i.calls++
	return errors.New("database offline")
}
func (i *unavailableIndex) List(context.Context, string, *url.OwnerCursor, int) ([]relations.Ref, error) {
	i.calls++
	return nil, errors.New("database offline")
}
func (i *unavailableIndex) ScanCodes(context.Context, string, int) ([]string, error) {
	i.calls++
	return nil, errors.New("database offline")
}

func TestDatabaseFailureDoesNotAffectLinkOperations(t *testing.T) {
	index := &unavailableIndex{}
	store, err := relations.New(relations.Options{Source: url.NewMemoryStore(), Index: index, LinkSpace: "test", ReadSource: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	svc := url.NewService(url.Options{Store: store})
	link, err := svc.ShortenWithOwner(t.Context(), &urlov1.ShortenRequest{LongUrl: "https://example.com", CustomCode: "abc"}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(t.Context(), &urlov1.ResolveRequest{Code: link.Link.Code}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetStatsAs(t.Context(), "abc", "bob"); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("owner check = %v", err)
	}
	if _, err := svc.UpdateAs(t.Context(), "abc", "alice", url.UpdatePatch{LongURL: new("https://example.com/updated")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetDisabledAs(t.Context(), "abc", "alice", true, "test"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteAs(t.Context(), "abc", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(t.Context(), &url.Record{Code: "expired", OwnerID: "alice", CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(t.Context(), &urlov1.ResolveRequest{Code: "expired"}); status.Code(err) != codes.NotFound {
		t.Fatalf("expired resolve = %v", err)
	}
	if index.calls != 0 {
		t.Fatalf("link operations called database %d times", index.calls)
	}
	if _, err := svc.ListByOwnerPage(t.Context(), "alice", url.PageOptions{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("list error = %v", err)
	}
}
