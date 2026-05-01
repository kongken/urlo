package url

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	urlov1 "github.com/kongken/urlo/pkg/proto/urlo/v1"
)

func TestShortenAndResolve(t *testing.T) {
	s := NewService(Options{BaseURL: "https://urlo.example/"})
	ctx := context.Background()

	resp, err := s.Shorten(ctx, &urlov1.ShortenRequest{LongUrl: "https://example.com/foo"})
	if err != nil {
		t.Fatalf("Shorten: %v", err)
	}
	link := resp.GetLink()
	if link.GetCode() == "" {
		t.Fatal("expected non-empty code")
	}
	if link.GetShortUrl() != "https://urlo.example/"+link.GetCode() {
		t.Errorf("unexpected short_url: %s", link.GetShortUrl())
	}

	got, err := s.Resolve(ctx, &urlov1.ResolveRequest{Code: link.GetCode()})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.GetLink().GetLongUrl() != "https://example.com/foo" {
		t.Errorf("long_url mismatch: %s", got.GetLink().GetLongUrl())
	}
	if got.GetLink().GetVisitCount() != 1 {
		t.Errorf("expected visit_count=1, got %d", got.GetLink().GetVisitCount())
	}
}

func TestShortenCustomCodeAndDuplicate(t *testing.T) {
	s := NewService(Options{})
	ctx := context.Background()

	if _, err := s.Shorten(ctx, &urlov1.ShortenRequest{LongUrl: "https://a.test", CustomCode: "abc"}); err != nil {
		t.Fatalf("first Shorten: %v", err)
	}
	_, err := s.Shorten(ctx, &urlov1.ShortenRequest{LongUrl: "https://b.test", CustomCode: "abc"})
	if status.Code(err) != codes.AlreadyExists {
		t.Errorf("expected AlreadyExists, got %v", err)
	}
}

func TestShortenInvalidInput(t *testing.T) {
	s := NewService(Options{})
	ctx := context.Background()

	cases := []struct {
		name string
		req  *urlov1.ShortenRequest
		want codes.Code
	}{
		{"empty url", &urlov1.ShortenRequest{}, codes.InvalidArgument},
		{"bad url", &urlov1.ShortenRequest{LongUrl: "not a url"}, codes.InvalidArgument},
		{"negative ttl", &urlov1.ShortenRequest{LongUrl: "https://x.test", TtlSeconds: -1}, codes.InvalidArgument},
		{"bad custom code", &urlov1.ShortenRequest{LongUrl: "https://x.test", CustomCode: "no/slash"}, codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Shorten(ctx, tc.req)
			if status.Code(err) != tc.want {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestResolveExpired(t *testing.T) {
	s := NewService(Options{})
	ctx := context.Background()

	resp, err := s.Shorten(ctx, &urlov1.ShortenRequest{LongUrl: "https://x.test", TtlSeconds: 1})
	if err != nil {
		t.Fatalf("Shorten: %v", err)
	}
	code := resp.GetLink().GetCode()

	mem := s.store.(*MemoryStore)
	mem.mu.Lock()
	r := mem.records[code]
	r.ExpiresAt = time.Now().Add(-time.Second)
	mem.records[code] = r
	mem.mu.Unlock()

	_, err = s.Resolve(ctx, &urlov1.ResolveRequest{Code: code})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	s := NewService(Options{})
	ctx := context.Background()

	resp, _ := s.Shorten(ctx, &urlov1.ShortenRequest{LongUrl: "https://x.test"})
	code := resp.GetLink().GetCode()

	if _, err := s.Delete(ctx, &urlov1.DeleteRequest{Code: code}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Resolve(ctx, &urlov1.ResolveRequest{Code: code}); status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound after delete, got %v", err)
	}
	if _, err := s.Delete(ctx, &urlov1.DeleteRequest{Code: code}); status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound on second delete, got %v", err)
	}
}

func TestShortenCodeLengthRequest(t *testing.T) {
	s := NewService(Options{})
	ctx := context.Background()

	resp, err := s.Shorten(ctx, &urlov1.ShortenRequest{
		LongUrl:    "https://example.com",
		CodeLength: 10,
	})
	if err != nil {
		t.Fatalf("Shorten: %v", err)
	}
	if got := len(resp.GetLink().GetCode()); got != 10 {
		t.Fatalf("code length = %d, want 10", got)
	}

	// Below minimum: rejected.
	if _, err := s.Shorten(ctx, &urlov1.ShortenRequest{
		LongUrl:    "https://example.com",
		CodeLength: 3,
	}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for code_length=3, got %v", err)
	}

	// Above maximum: rejected.
	if _, err := s.Shorten(ctx, &urlov1.ShortenRequest{
		LongUrl:    "https://example.com",
		CodeLength: 99,
	}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for code_length=99, got %v", err)
	}

	// Custom code wins; code_length is ignored when custom_code is set.
	resp, err = s.Shorten(ctx, &urlov1.ShortenRequest{
		LongUrl:    "https://example.com/two",
		CustomCode: "myCustom",
		CodeLength: 12,
	})
	if err != nil {
		t.Fatalf("Shorten with custom_code: %v", err)
	}
	if resp.GetLink().GetCode() != "myCustom" {
		t.Errorf("custom_code ignored: got %q", resp.GetLink().GetCode())
	}
}

func TestCodeLengthConfigurable(t *testing.T) {
	cases := []struct {
		name     string
		opt      int
		setLen   int
		wantLen  int
	}{
		{"default zero", 0, 0, DefaultCodeLen},
		{"below min raised", 3, 0, MinCodeLen},
		{"explicit 10", 10, 0, 10},
		{"set after init clamped", 0, 1, MinCodeLen},
		{"above max clamped", 99, 0, 32},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewService(Options{CodeLen: tc.opt})
			if tc.setLen != 0 {
				s.SetCodeLength(tc.setLen)
			}
			if got := s.CodeLength(); got != tc.wantLen {
				t.Fatalf("CodeLength() = %d, want %d", got, tc.wantLen)
			}
			resp, err := s.Shorten(context.Background(), &urlov1.ShortenRequest{LongUrl: "https://example.com"})
			if err != nil {
				t.Fatalf("Shorten: %v", err)
			}
			if got := len(resp.GetLink().GetCode()); got != tc.wantLen {
				t.Fatalf("generated code length = %d, want %d", got, tc.wantLen)
			}
		})
	}
}
