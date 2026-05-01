package url

import (
	"context"
	"errors"
	"time"
)

// Record is the persistent representation of a short link.
type Record struct {
	Code       string    `json:"code"`
	LongURL    string    `json:"long_url"`
	OwnerID    string    `json:"owner_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at,omitzero"`
	VisitCount int64     `json:"visit_count"`
}

// Expired reports whether the record has a non-zero ExpiresAt in the past.
func (r *Record) Expired() bool {
	return !r.ExpiresAt.IsZero() && time.Now().UTC().After(r.ExpiresAt)
}

// Store is the storage abstraction used by Service.
//
// Implementations must return ErrNotFound / ErrAlreadyExists for the
// corresponding conditions; other errors propagate to the caller.
type Store interface {
	Create(ctx context.Context, r *Record) error
	Get(ctx context.Context, code string) (*Record, error)
	Delete(ctx context.Context, code string) error
	// IncrVisit atomically (best-effort) increments VisitCount and returns
	// the updated record.
	IncrVisit(ctx context.Context, code string) (*Record, error)
	// ListByOwner returns records whose OwnerID matches ownerID.
	// Empty ownerID is invalid — implementations should return an empty
	// slice rather than every record.
	ListByOwner(ctx context.Context, ownerID string) ([]*Record, error)
}

var (
	ErrNotFound      = errors.New("url: not found")
	ErrAlreadyExists = errors.New("url: already exists")
)
