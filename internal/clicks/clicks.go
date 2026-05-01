// Package clicks records and lists short-link click events.
//
// Recording happens off the redirect hot path: handlers call Record,
// which buffers an event and returns immediately. A background worker
// drains the buffer into the configured backend (e.g. Redis Stream).
package clicks

import (
	"context"
	"errors"
	"time"
)

// Event is a single recorded click. Fields mirror urlov1.ClickEvent
// but live in this package to keep the recorder backend-agnostic.
type Event struct {
	ID           string
	Code         string
	Timestamp    time.Time
	IPHash       string
	Country      string
	City         string
	Referrer     string
	ReferrerHost string
	UserAgent    string
	Browser      string
	OS           string
	Device       string
	Lang         string
	IsBot        bool
}

// ListOptions narrows ListClicks results.
type ListOptions struct {
	// PageSize bounds the number of returned events. <=0 selects a default.
	PageSize int
	// PageToken is the opaque cursor from a prior response.
	PageToken string
}

// ErrNotFound signals an unknown click record (e.g. invalid cursor).
var ErrNotFound = errors.New("clicks: not found")

// Recorder captures click events and serves recent history.
//
// Record MUST be non-blocking: implementations either enqueue and return,
// or drop on backpressure. List queries the backend directly.
type Recorder interface {
	Record(ctx context.Context, evt Event)
	List(ctx context.Context, code string, opts ListOptions) (events []Event, nextPageToken string, err error)
}

// Nop is a Recorder that discards events and returns empty lists.
type Nop struct{}

func (Nop) Record(context.Context, Event)                                       {}
func (Nop) List(context.Context, string, ListOptions) ([]Event, string, error) { return nil, "", nil }
