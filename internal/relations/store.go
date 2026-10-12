// Package relations maintains a rebuildable owner index beside the authoritative
// link store. Database failures must never affect link creation or resolution.
package relations

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kongken/urlo/internal/url"
)

const (
	ReadS3       = "s3"
	ReadPostgres = "postgres"
)

// RecordGetter always reads the authoritative current object, not an event body.
type RecordGetter interface {
	Get(context.Context, string) (*url.Record, error)
}

type Ref struct {
	Code      string
	CreatedAt time.Time
}

// Index is a disposable owner projection. Refresh implementations must serialize
// refreshes for the same key across processes before reading the source.
type Index interface {
	Refresh(context.Context, string, RecordGetter) error
	List(context.Context, string, *url.OwnerCursor, int) ([]Ref, error)
	ScanCodes(context.Context, string, int) ([]string, error)
}

type Options struct {
	Source            url.Store
	Index             Index
	LinkSpace         string
	ReadSource        string
	BufferSize        int
	ReconcileInterval time.Duration
	OperationTimeout  time.Duration
	Logger            *slog.Logger
}

// Store delegates all primary-record operations to Source. Mutation notifications
// are non-blocking; lost notifications are repaired by reconciliation.
type Store struct {
	url.Store
	index       Index
	scanner     url.ScanStore
	space       string
	readSource  string
	queue       chan string
	interval    time.Duration
	timeout     time.Duration
	log         *slog.Logger
	mu          sync.Mutex
	cancel      context.CancelFunc
	closed      atomic.Bool
	wg          sync.WaitGroup
	reconcileMu sync.Mutex
	queued      atomic.Int64
	dropped     atomic.Int64
	refreshed   atomic.Int64
	failed      atomic.Int64
	runs        atomic.Int64
	runFailures atomic.Int64
}

func New(opts Options) (*Store, error) {
	if opts.Source == nil || opts.Index == nil || opts.LinkSpace == "" {
		return nil, errors.New("relations: source, index and link_space are required")
	}
	scanner, ok := opts.Source.(url.ScanStore)
	if !ok {
		return nil, errors.New("relations: source must support primary-record scans")
	}
	readSource := cmp.Or(opts.ReadSource, ReadPostgres)
	if readSource != ReadS3 && readSource != ReadPostgres {
		return nil, errors.New("relations: owner_index_read_source must be s3 or postgres")
	}
	if opts.BufferSize < 0 || opts.ReconcileInterval < 0 || opts.OperationTimeout < 0 {
		return nil, errors.New("relations: invalid queue size or timeout")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Store{
		Store: opts.Source, index: opts.Index, scanner: scanner, space: opts.LinkSpace,
		readSource: readSource, queue: make(chan string, cmp.Or(opts.BufferSize, 1024)),
		interval: cmp.Or(opts.ReconcileInterval, 5*time.Minute),
		timeout:  cmp.Or(opts.OperationTimeout, 10*time.Second), log: logger,
	}, nil
}

func (s *Store) Create(ctx context.Context, r *url.Record) error {
	if err := s.Store.Create(ctx, r); err != nil {
		return err
	}
	s.schedule(r.Code)
	return nil
}

func (s *Store) Update(ctx context.Context, r *url.Record) error {
	if err := s.Store.Update(ctx, r); err != nil {
		return err
	}
	s.schedule(r.Code)
	return nil
}

func (s *Store) Delete(ctx context.Context, code string) error {
	err := s.Store.Delete(ctx, code)
	if err == nil || errors.Is(err, url.ErrNotFound) {
		s.schedule(code)
	}
	return err
}

func (s *Store) schedule(code string) {
	if s.closed.Load() {
		return
	}
	select {
	case s.queue <- code:
		s.queued.Add(1)
	default:
		s.dropped.Add(1)
	}
}

// ListByOwner retains the legacy, unpaginated contract for existing callers.
func (s *Store) ListByOwner(ctx context.Context, owner string) ([]*url.Record, error) {
	if owner == "" {
		return nil, nil
	}
	if s.readSource == ReadS3 {
		return s.Store.ListByOwner(ctx, owner)
	}
	var out []*url.Record
	token := ""
	for {
		page, err := s.ListByOwnerPage(ctx, owner, url.PageOptions{PageSize: url.MaxOwnerPageSize, PageToken: token})
		if err != nil {
			return nil, err
		}
		out = append(out, page.Records...)
		if page.NextPageToken == "" {
			return out, nil
		}
		token = page.NextPageToken
	}
}

// Stats reports cumulative queue and repair counters without querying SQL.
type Stats struct {
	Queued, Dropped, Refreshed, RefreshErrors, Reconciliations, ReconcileErrors int64
}

func (s *Store) Stats() Stats {
	return Stats{s.queued.Load(), s.dropped.Load(), s.refreshed.Load(), s.failed.Load(), s.runs.Load(), s.runFailures.Load()}
}
