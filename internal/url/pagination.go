package url

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	urlov1 "github.com/kongken/urlo/pkg/proto/urlo/v1"
)

const (
	DefaultOwnerPageSize = 50
	MaxOwnerPageSize     = 500
)

var (
	ErrInvalidPage = errors.New("url: invalid owner page")
	ErrUnavailable = errors.New("url: owner index unavailable")
)

// PageOptions requests an owner page. Zero PageSize selects the default.
type PageOptions struct {
	PageSize  int
	PageToken string
}

func (o PageOptions) Validate() error {
	if o.PageSize < 0 || o.PageSize > MaxOwnerPageSize || len(o.PageToken) > 4096 {
		return ErrInvalidPage
	}
	return nil
}

func (o PageOptions) Size() int { return cmp.Or(o.PageSize, DefaultOwnerPageSize) }

// OwnerCursor is a database ordering position, not a link identity.
type OwnerCursor struct {
	CreatedAt time.Time `json:"created_at"`
	Code      string    `json:"code"`
}

type ownerToken struct {
	Version int         `json:"v"`
	Space   string      `json:"space"`
	Owner   string      `json:"owner"`
	After   OwnerCursor `json:"after"`
}

// EncodeOwnerCursor binds a continuation position to a read source and owner.
// Tokens are opaque ordering hints, not authentication credentials.
func EncodeOwnerCursor(space, owner string, after OwnerCursor) string {
	b, _ := json.Marshal(ownerToken{Version: 1, Space: space, Owner: owner, After: after})
	return base64.RawURLEncoding.EncodeToString(b)
}

func DecodeOwnerCursor(token, space, owner string) (*OwnerCursor, error) {
	if token == "" {
		return nil, nil
	}
	if len(token) > 4096 {
		return nil, ErrInvalidPage
	}
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidPage
	}
	var v ownerToken
	if err := json.Unmarshal(b, &v); err != nil || v.Version != 1 || v.Space != space || v.Owner != owner || v.After.CreatedAt.IsZero() || !IsValidCode(v.After.Code) {
		return nil, ErrInvalidPage
	}
	return &v.After, nil
}

// RecordPage is hydrated from the authoritative link store.
type RecordPage struct {
	Records       []*Record
	NextPageToken string
}

// OwnerPageStore optionally provides an indexed owner query.
type OwnerPageStore interface {
	ListByOwnerPage(context.Context, string, PageOptions) (*RecordPage, error)
}

// ScanStore enumerates primary-record codes, never owner pointers. Tokens are
// backend-specific; an empty next token marks the end, including empty pages.
type ScanStore interface {
	ScanCodes(ctx context.Context, token string, limit int) (codes []string, next string, err error)
}

// PageOwnedRecords is the compatibility path for stores without an owner index.
// It still reads all owner records; indexed stores should implement OwnerPageStore.
func PageOwnedRecords(records []*Record, owner string, opts PageOptions) (*RecordPage, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	after, err := DecodeOwnerCursor(opts.PageToken, "legacy", owner)
	if err != nil {
		return nil, err
	}
	owned := make([]*Record, 0, len(records))
	for _, r := range records {
		if r.OwnerID == owner && !r.Expired() {
			owned = append(owned, r)
		}
	}
	slices.SortFunc(owned, func(a, b *Record) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), strings.Compare(b.Code, a.Code))
	})
	page := &RecordPage{Records: make([]*Record, 0, opts.Size())}
	for _, r := range owned {
		if after != nil && (r.CreatedAt.After(after.CreatedAt) || r.CreatedAt.Equal(after.CreatedAt) && r.Code >= after.Code) {
			continue
		}
		if len(page.Records) == opts.Size() {
			last := page.Records[len(page.Records)-1]
			page.NextPageToken = EncodeOwnerCursor("legacy", owner, OwnerCursor{CreatedAt: last.CreatedAt, Code: last.Code})
			break
		}
		page.Records = append(page.Records, r)
	}
	return page, nil
}

type OwnerPage struct {
	Links         []*urlov1.ShortLink
	NextPageToken string
}

func (s *Service) ListByOwnerPage(ctx context.Context, owner string, opts PageOptions) (*OwnerPage, error) {
	if owner == "" {
		return nil, status.Error(codes.InvalidArgument, "ownerID is required")
	}
	if err := opts.Validate(); err != nil {
		return nil, mapOwnerQueryErr(err)
	}
	var page *RecordPage
	var err error
	if indexed, ok := s.store.(OwnerPageStore); ok {
		page, err = indexed.ListByOwnerPage(ctx, owner, opts)
	} else {
		var records []*Record
		records, err = s.store.ListByOwner(ctx, owner)
		if err == nil {
			page, err = PageOwnedRecords(records, owner, opts)
		}
	}
	if err != nil {
		return nil, mapOwnerQueryErr(err)
	}
	out := &OwnerPage{Links: make([]*urlov1.ShortLink, 0, len(page.Records)), NextPageToken: page.NextPageToken}
	for _, r := range page.Records {
		if r.OwnerID == owner && !r.Expired() {
			out.Links = append(out.Links, s.toProto(r))
		}
	}
	return out, nil
}

func mapOwnerQueryErr(err error) error {
	switch {
	case errors.Is(err, ErrInvalidPage):
		return status.Error(codes.InvalidArgument, "invalid page_size or page_token")
	case errors.Is(err, ErrUnavailable):
		return status.Error(codes.Unavailable, "owner index temporarily unavailable")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "owner query timed out")
	default:
		return status.Error(codes.Internal, "unable to list links")
	}
}
