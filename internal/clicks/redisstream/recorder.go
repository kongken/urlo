// Package redisstream implements a clicks.Recorder backed by Redis Streams.
//
// Each short link gets its own stream `clicks:{code}` capped via XADD MAXLEN.
// Record is non-blocking: events go through an in-process buffered channel
// drained by a single worker goroutine that batches XADD pipelines.
package redisstream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kongken/urlo/internal/clicks"
)

// Options configures Recorder.
type Options struct {
	Client *redis.Client
	// KeyPrefix is prepended to every stream key (default "clicks").
	KeyPrefix string
	// MaxLen caps each per-code stream's length. 0 disables capping.
	MaxLen int64
	// BufferSize bounds the in-memory channel. When full, Record drops
	// the event (logged at WARN) instead of blocking the redirect path.
	BufferSize int
	// FlushInterval is the maximum time an event may sit in the buffer
	// before being flushed (default 200ms).
	FlushInterval time.Duration
	// BatchSize is the maximum number of events flushed per pipeline
	// (default 64).
	BatchSize int
}

// Recorder is a clicks.Recorder that writes to Redis Streams.
type Recorder struct {
	client    *redis.Client
	keyPrefix string
	maxLen    int64
	batchSize int

	in    chan clicks.Event
	stop  chan struct{}
	doneW sync.WaitGroup
}

// New constructs a Recorder and starts its background worker. Call
// Close on shutdown to drain pending events.
func New(opts Options) (*Recorder, error) {
	if opts.Client == nil {
		return nil, errors.New("redisstream: nil redis client")
	}
	if opts.KeyPrefix == "" {
		opts.KeyPrefix = "clicks"
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = 1024
	}
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = 200 * time.Millisecond
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 64
	}

	r := &Recorder{
		client:    opts.Client,
		keyPrefix: opts.KeyPrefix,
		maxLen:    opts.MaxLen,
		batchSize: opts.BatchSize,
		in:        make(chan clicks.Event, opts.BufferSize),
		stop:      make(chan struct{}),
	}
	r.doneW.Add(1)
	go r.run(opts.FlushInterval)
	return r, nil
}

// Close stops the worker, draining any pending events with the given
// deadline. Safe to call multiple times.
func (r *Recorder) Close(ctx context.Context) error {
	select {
	case <-r.stop:
		return nil
	default:
		close(r.stop)
	}
	done := make(chan struct{})
	go func() {
		r.doneW.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Record enqueues evt. Drops on full buffer; never blocks.
func (r *Recorder) Record(_ context.Context, evt clicks.Event) {
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}
	select {
	case r.in <- evt:
	default:
		slog.Warn("clicks recorder buffer full, dropping event", "code", evt.Code)
	}
}

// List returns up to opts.PageSize events for code, newest first.
// PageToken is a stream id; events strictly older than it are returned.
func (r *Recorder) List(ctx context.Context, code string, opts clicks.ListOptions) ([]clicks.Event, string, error) {
	if code == "" {
		return nil, "", errors.New("redisstream: empty code")
	}
	size := opts.PageSize
	if size <= 0 || size > 500 {
		if size <= 0 {
			size = 50
		} else {
			size = 500
		}
	}
	end := "+"
	if opts.PageToken != "" {
		// Exclusive end via "(" prefix returns ids strictly older.
		end = "(" + opts.PageToken
	}

	msgs, err := r.client.XRevRangeN(ctx, r.streamKey(code), end, "-", int64(size)).Result()
	if err != nil {
		return nil, "", fmt.Errorf("redisstream: xrevrange: %w", err)
	}
	out := make([]clicks.Event, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, decode(m))
	}
	next := ""
	if len(msgs) == size && size > 0 {
		next = msgs[len(msgs)-1].ID
	}
	return out, next, nil
}

func (r *Recorder) streamKey(code string) string {
	return r.keyPrefix + ":" + code
}

func (r *Recorder) run(flushEvery time.Duration) {
	defer r.doneW.Done()
	ticker := time.NewTicker(flushEvery)
	defer ticker.Stop()

	batch := make([]clicks.Event, 0, r.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := r.flush(batch); err != nil {
			slog.Warn("clicks recorder flush failed", "err", err, "n", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case evt := <-r.in:
			batch = append(batch, evt)
			if len(batch) >= r.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-r.stop:
			// Drain remaining events without blocking.
			for {
				select {
				case evt := <-r.in:
					batch = append(batch, evt)
					if len(batch) >= r.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func (r *Recorder) flush(batch []clicks.Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pipe := r.client.Pipeline()
	for _, evt := range batch {
		args := &redis.XAddArgs{
			Stream: r.streamKey(evt.Code),
			Values: encode(evt),
		}
		if r.maxLen > 0 {
			args.MaxLen = r.maxLen
			args.Approx = true
		}
		pipe.XAdd(ctx, args)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func encode(e clicks.Event) map[string]any {
	return map[string]any{
		"code":          e.Code,
		"ts_unix":       strconv.FormatInt(e.Timestamp.UnixMilli(), 10),
		"ip_hash":       e.IPHash,
		"country":       e.Country,
		"city":          e.City,
		"referrer":      e.Referrer,
		"referrer_host": e.ReferrerHost,
		"user_agent":    e.UserAgent,
		"browser":       e.Browser,
		"os":            e.OS,
		"device":        e.Device,
		"lang":          e.Lang,
		"is_bot":        boolStr(e.IsBot),
	}
}

func decode(m redis.XMessage) clicks.Event {
	get := func(k string) string {
		if v, ok := m.Values[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	tsMillis, _ := strconv.ParseInt(get("ts_unix"), 10, 64)
	ts := time.UnixMilli(tsMillis).UTC()
	return clicks.Event{
		ID:           m.ID,
		Code:         get("code"),
		Timestamp:    ts,
		IPHash:       get("ip_hash"),
		Country:      get("country"),
		City:         get("city"),
		Referrer:     get("referrer"),
		ReferrerHost: get("referrer_host"),
		UserAgent:    get("user_agent"),
		Browser:      get("browser"),
		OS:           get("os"),
		Device:       get("device"),
		Lang:         get("lang"),
		IsBot:        get("is_bot") == "1",
	}
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
