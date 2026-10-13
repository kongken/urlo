package url_test

import (
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"

	"github.com/kongken/urlo/internal/url"
)

func TestOwnerPageRejectsInvalidAndForeignCursors(t *testing.T) {
	svc := url.NewService(url.Options{})
	for _, opts := range []url.PageOptions{
		{PageSize: -1}, {PageSize: 501}, {PageToken: "not-base64"}, {PageToken: strings.Repeat("a", 4097)},
		{PageToken: url.EncodeOwnerCursor("legacy", "bob", url.OwnerCursor{CreatedAt: time.Now().UTC(), Code: "abc"})},
		{PageToken: url.EncodeOwnerCursor("other-space", "alice", url.OwnerCursor{CreatedAt: time.Now().UTC(), Code: "abc"})},
	} {
		if _, err := svc.ListByOwnerPage(t.Context(), "alice", opts); status.Code(err) != codes.InvalidArgument {
			t.Errorf("opts=%+v error=%v", opts, err)
		}
	}
}

func TestOwnerPagesPreserveOrderingAndDisabledLinks(t *testing.T) {
	store := url.NewMemoryStore()
	now := time.Now().UTC()
	for _, r := range []*url.Record{
		{Code: "a", OwnerID: "alice", CreatedAt: now},
		{Code: "b", OwnerID: "alice", CreatedAt: now, Disabled: true},
		{Code: "other", OwnerID: "bob", CreatedAt: now},
		{Code: "expired", OwnerID: "alice", CreatedAt: now, ExpiresAt: now.Add(-time.Hour)},
	} {
		if err := store.Create(t.Context(), r); err != nil {
			t.Fatal(err)
		}
	}
	svc := url.NewService(url.Options{Store: store})
	first, err := svc.ListByOwnerPage(t.Context(), "alice", url.PageOptions{PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Links) != 1 || first.Links[0].GetCode() != "b" || first.NextPageToken == "" {
		t.Fatalf("first page = %+v", first)
	}
	second, err := svc.ListByOwnerPage(t.Context(), "alice", url.PageOptions{PageSize: 1, PageToken: first.NextPageToken})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Links) != 1 || second.Links[0].GetCode() != "a" || second.NextPageToken != "" {
		t.Fatalf("second page = %+v", second)
	}
}
