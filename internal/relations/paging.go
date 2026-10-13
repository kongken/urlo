package relations

import (
	"context"
	"errors"
	"sync"

	"github.com/kongken/urlo/internal/url"
)

// ListByOwnerPage queries only candidate refs from SQL, then checks owner and
// expiry against current S3 records. A stale index is never an authorization.
func (s *Store) ListByOwnerPage(ctx context.Context, owner string, opts url.PageOptions) (*url.RecordPage, error) {
	if owner == "" {
		return &url.RecordPage{Records: []*url.Record{}}, nil
	}
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if s.readSource == ReadS3 {
		records, err := s.Store.ListByOwner(ctx, owner)
		if err != nil {
			return nil, err
		}
		return url.PageOwnedRecords(records, owner, opts)
	}
	after, err := url.DecodeOwnerCursor(opts.PageToken, s.space, owner)
	if err != nil {
		return nil, err
	}
	page := &url.RecordPage{Records: make([]*url.Record, 0, opts.Size())}
	budget := max(500, opts.Size()*10)
	for scanned := 0; scanned < budget; {
		limit := min(100, opts.Size()+1, budget-scanned)
		refs, err := s.index.List(ctx, owner, after, limit)
		if err != nil {
			s.log.Warn("owner index query failed", "error", err)
			return nil, url.ErrUnavailable
		}
		if len(refs) == 0 {
			return page, nil
		}
		records, err := s.hydrate(ctx, refs)
		if err != nil {
			return nil, err
		}
		for i, ref := range refs {
			r := records[i]
			if r != nil && r.OwnerID == owner && !r.Expired() {
				if len(page.Records) == opts.Size() {
					// Do not consume this lookahead: it must appear on the next page.
					page.NextPageToken = url.EncodeOwnerCursor(s.space, owner, *after)
					return page, nil
				}
				page.Records = append(page.Records, r)
			} else if r == nil || r.OwnerID != owner {
				s.schedule(ref.Code)
			}
			after = &url.OwnerCursor{CreatedAt: ref.CreatedAt, Code: ref.Code}
			scanned++
		}
		if len(refs) < limit {
			return page, nil
		}
	}
	// Even an entirely stale candidate page advances its cursor, so a client
	// can continue rather than repeatedly fetching the same expired/deleted refs.
	page.NextPageToken = url.EncodeOwnerCursor(s.space, owner, *after)
	return page, nil
}

func (s *Store) hydrate(ctx context.Context, refs []Ref) ([]*url.Record, error) {
	records := make([]*url.Record, len(refs))
	errorsByRef := make([]error, len(refs))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, ref := range refs {
		wg.Go(func() {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errorsByRef[i] = ctx.Err()
				return
			}
			r, err := s.Store.Get(ctx, ref.Code)
			if errors.Is(err, url.ErrNotFound) {
				return
			}
			if err == nil && (r == nil || r.Code != ref.Code) {
				err = errors.New("relations: primary record code mismatch")
			}
			records[i], errorsByRef[i] = r, err
		})
	}
	wg.Wait()
	for _, err := range errorsByRef {
		if err != nil {
			return nil, err
		}
	}
	return records, nil
}
