package s3store

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"

	"github.com/kongken/urlo/internal/url"
)

// fakeS3 is a minimal in-memory S3 API implementation that honors the
// subset of behaviors we care about: If-None-Match: * for conditional
// creates, NoSuchKey on missing reads, and per-bucket key namespacing.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte // key: bucket/key
}

func newFakeS3() *fakeS3 { return &fakeS3{objects: make(map[string][]byte)} }

func (f *fakeS3) k(bucket, key string) string { return bucket + "/" + key }

func (f *fakeS3) PutObject(_ context.Context, in *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := f.k(aws.ToString(in.Bucket), aws.ToString(in.Key))
	if aws.ToString(in.IfNoneMatch) == "*" {
		if _, exists := f.objects[k]; exists {
			return nil, &smithy.GenericAPIError{Code: "PreconditionFailed", Message: "object exists"}
		}
	}
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.objects[k] = body
	return &awss3.PutObjectOutput{}, nil
}

func (f *fakeS3) GetObject(_ context.Context, in *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, ok := f.objects[f.k(aws.ToString(in.Bucket), aws.ToString(in.Key))]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return &awss3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func (f *fakeS3) HeadObject(_ context.Context, in *awss3.HeadObjectInput, _ ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.objects[f.k(aws.ToString(in.Bucket), aws.ToString(in.Key))]; !ok {
		return nil, &types.NotFound{}
	}
	return &awss3.HeadObjectOutput{}, nil
}

func (f *fakeS3) DeleteObject(_ context.Context, in *awss3.DeleteObjectInput, _ ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, f.k(aws.ToString(in.Bucket), aws.ToString(in.Key)))
	return &awss3.DeleteObjectOutput{}, nil
}

func (f *fakeS3) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.objects))
	for k := range f.objects {
		out = append(out, k)
	}
	return out
}

func newStore(t *testing.T, prefix string) (*Store, *fakeS3) {
	t.Helper()
	fake := newFakeS3()
	s, err := New(Options{Client: fake, Bucket: "bkt", Prefix: prefix})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, fake
}

func TestPrefixDefault(t *testing.T) {
	s, fake := newStore(t, "")
	if err := s.Create(context.Background(), &url.Record{Code: "abc", LongURL: "https://x.test"}); err != nil {
		t.Fatal(err)
	}
	keys := fake.keys()
	want := "bkt/links/abc.json"
	if len(keys) != 1 || keys[0] != want {
		t.Errorf("keys=%v, want [%s]", keys, want)
	}
}

func TestPrefixCustomNested(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"urlo/links", "bkt/urlo/links/abc.json"},
		{"/envs/prod/urlo/", "bkt/envs/prod/urlo/abc.json"},
		{"single", "bkt/single/abc.json"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			s, fake := newStore(t, tc.input)
			if err := s.Create(context.Background(), &url.Record{Code: "abc", LongURL: "https://x.test"}); err != nil {
				t.Fatal(err)
			}
			keys := fake.keys()
			if len(keys) != 1 || keys[0] != tc.want {
				t.Errorf("keys=%v, want [%s]", keys, tc.want)
			}
		})
	}
}

func TestCreateConflict(t *testing.T) {
	s, _ := newStore(t, "p")
	ctx := context.Background()
	if err := s.Create(ctx, &url.Record{Code: "abc", LongURL: "https://x.test"}); err != nil {
		t.Fatal(err)
	}
	err := s.Create(ctx, &url.Record{Code: "abc", LongURL: "https://y.test"})
	if !errors.Is(err, url.ErrAlreadyExists) {
		t.Errorf("got %v, want ErrAlreadyExists", err)
	}
}

func TestGetNotFound(t *testing.T) {
	s, _ := newStore(t, "p")
	_, err := s.Get(context.Background(), "missing")
	if !errors.Is(err, url.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestRoundTripAndIncrVisit(t *testing.T) {
	s, _ := newStore(t, "p")
	ctx := context.Background()
	rec := &url.Record{Code: "xyz", LongURL: "https://x.test"}
	if err := s.Create(ctx, rec); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, "xyz")
	if err != nil || got.LongURL != "https://x.test" {
		t.Fatalf("Get: rec=%+v err=%v", got, err)
	}

	updated, err := s.IncrVisit(ctx, "xyz")
	if err != nil {
		t.Fatal(err)
	}
	if updated.VisitCount != 1 {
		t.Errorf("visit=%d, want 1", updated.VisitCount)
	}

	again, _ := s.Get(ctx, "xyz")
	if again.VisitCount != 1 {
		t.Errorf("persisted visit=%d, want 1", again.VisitCount)
	}
}

func TestDelete(t *testing.T) {
	s, _ := newStore(t, "p")
	ctx := context.Background()
	if err := s.Delete(ctx, "missing"); !errors.Is(err, url.ErrNotFound) {
		t.Errorf("delete missing: %v, want ErrNotFound", err)
	}

	_ = s.Create(ctx, &url.Record{Code: "abc", LongURL: "https://x.test"})
	if err := s.Delete(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "abc"); !errors.Is(err, url.ErrNotFound) {
		t.Errorf("after delete: %v, want ErrNotFound", err)
	}
}

func TestPrefixIsolation(t *testing.T) {
	// Ensures records under different prefixes don't collide in the same bucket.
	fake := newFakeS3()
	a, _ := New(Options{Client: fake, Bucket: "bkt", Prefix: "appA"})
	b, _ := New(Options{Client: fake, Bucket: "bkt", Prefix: "appB"})
	ctx := context.Background()

	_ = a.Create(ctx, &url.Record{Code: "same", LongURL: "https://a.test"})
	_ = b.Create(ctx, &url.Record{Code: "same", LongURL: "https://b.test"})

	got, _ := a.Get(ctx, "same")
	if got.LongURL != "https://a.test" {
		t.Errorf("appA got %q", got.LongURL)
	}
	got, _ = b.Get(ctx, "same")
	if got.LongURL != "https://b.test" {
		t.Errorf("appB got %q", got.LongURL)
	}

	keys := fake.keys()
	if len(keys) != 2 {
		t.Errorf("keys=%v, want 2 entries", keys)
	}
	for _, k := range keys {
		if !strings.Contains(k, "/appA/") && !strings.Contains(k, "/appB/") {
			t.Errorf("unexpected key %q", k)
		}
	}
}
