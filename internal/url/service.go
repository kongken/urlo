package url

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	urlov1 "github.com/kongken/urlo/pkg/proto/urlo/v1"
)

const (
	defaultCodeLen = 6
	maxCodeLen     = 32
	codeAlphabet   = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

// Service implements urlov1.UrlServiceServer on top of a Store.
type Service struct {
	urlov1.UnimplementedUrlServiceServer

	store Store

	mu      sync.RWMutex
	baseURL string
}

type Options struct {
	Store Store
	// BaseURL is prepended to codes when building ShortLink.short_url.
	// e.g. "https://urlo.example".
	BaseURL string
}

func NewService(opts Options) *Service {
	store := opts.Store
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{
		store:   store,
		baseURL: opts.BaseURL,
	}
}

// SetBaseURL updates the base URL used to build ShortLink.short_url.
// Intended to be called once during app init after config is loaded.
func (s *Service) SetBaseURL(baseURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseURL = baseURL
}

// SetStore swaps the underlying Store. Intended to be called once during
// app init after config is loaded.
func (s *Service) SetStore(store Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
}

func (s *Service) Shorten(ctx context.Context, req *urlov1.ShortenRequest) (*urlov1.ShortenResponse, error) {
	return s.ShortenWithOwner(ctx, req, "")
}

// ShortenWithOwner is like Shorten but tags the new record with ownerID.
// Pass "" to create an anonymous link.
func (s *Service) ShortenWithOwner(ctx context.Context, req *urlov1.ShortenRequest, ownerID string) (*urlov1.ShortenResponse, error) {
	if req.GetLongUrl() == "" {
		return nil, status.Error(codes.InvalidArgument, "long_url is required")
	}
	if _, err := url.ParseRequestURI(req.GetLongUrl()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "long_url is not a valid URL: %v", err)
	}
	if req.GetTtlSeconds() < 0 {
		return nil, status.Error(codes.InvalidArgument, "ttl_seconds must be >= 0")
	}

	now := time.Now().UTC()
	var expiresAt time.Time
	if req.GetTtlSeconds() > 0 {
		expiresAt = now.Add(time.Duration(req.GetTtlSeconds()) * time.Second)
	}

	if code := req.GetCustomCode(); code != "" {
		if !isValidCode(code) {
			return nil, status.Error(codes.InvalidArgument, "custom_code must be 1-32 chars from [A-Za-z0-9]")
		}
		r := &Record{Code: code, LongURL: req.GetLongUrl(), OwnerID: ownerID, CreatedAt: now, ExpiresAt: expiresAt}
		if err := s.store.Create(ctx, r); err != nil {
			return nil, mapStoreErr(err, code)
		}
		return &urlov1.ShortenResponse{Link: s.toProto(r)}, nil
	}

	for range 8 {
		code, err := randomCode(defaultCodeLen)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "generate code: %v", err)
		}
		r := &Record{Code: code, LongURL: req.GetLongUrl(), OwnerID: ownerID, CreatedAt: now, ExpiresAt: expiresAt}
		err = s.store.Create(ctx, r)
		if err == nil {
			return &urlov1.ShortenResponse{Link: s.toProto(r)}, nil
		}
		if !errors.Is(err, ErrAlreadyExists) {
			return nil, status.Errorf(codes.Internal, "create: %v", err)
		}
	}
	return nil, status.Error(codes.Internal, "failed to generate unique code after 8 attempts")
}

// ListByOwner returns all (non-expired) ShortLinks owned by ownerID.
func (s *Service) ListByOwner(ctx context.Context, ownerID string) ([]*urlov1.ShortLink, error) {
	if ownerID == "" {
		return nil, status.Error(codes.InvalidArgument, "ownerID is required")
	}
	records, err := s.store.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list: %v", err)
	}
	out := make([]*urlov1.ShortLink, 0, len(records))
	for _, r := range records {
		if r.Expired() {
			continue
		}
		out = append(out, s.toProto(r))
	}
	return out, nil
}

// DeleteAs deletes a record, enforcing owner. If the record has an owner,
// ownerID must match. If the record has no owner, deletion is allowed
// (legacy/anonymous links).
func (s *Service) DeleteAs(ctx context.Context, code, ownerID string) error {
	if code == "" {
		return status.Error(codes.InvalidArgument, "code is required")
	}
	r, err := s.store.Get(ctx, code)
	if err != nil {
		return mapStoreErr(err, code)
	}
	if r.OwnerID != "" && r.OwnerID != ownerID {
		return status.Error(codes.PermissionDenied, "not owner")
	}
	if err := s.store.Delete(ctx, code); err != nil {
		return mapStoreErr(err, code)
	}
	return nil
}

// GetStatsAs returns stats with the same ownership rules as DeleteAs.
func (s *Service) GetStatsAs(ctx context.Context, code, ownerID string) (*urlov1.ShortLink, error) {
	if code == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}
	r, err := s.store.Get(ctx, code)
	if err != nil {
		return nil, mapStoreErr(err, code)
	}
	if r.OwnerID != "" && r.OwnerID != ownerID {
		return nil, status.Error(codes.PermissionDenied, "not owner")
	}
	return s.toProto(r), nil
}

func (s *Service) Resolve(ctx context.Context, req *urlov1.ResolveRequest) (*urlov1.ResolveResponse, error) {
	code := req.GetCode()
	if code == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}

	r, err := s.store.Get(ctx, code)
	if err != nil {
		return nil, mapStoreErr(err, code)
	}
	if r.Expired() {
		_ = s.store.Delete(ctx, code)
		return nil, status.Errorf(codes.NotFound, "code %q expired", code)
	}

	updated, err := s.store.IncrVisit(ctx, code)
	if err != nil {
		// Visit-count failures shouldn't break resolution; log via gRPC trace.
		return &urlov1.ResolveResponse{Link: s.toProto(r)}, nil
	}
	return &urlov1.ResolveResponse{Link: s.toProto(updated)}, nil
}

func (s *Service) GetStats(ctx context.Context, req *urlov1.GetStatsRequest) (*urlov1.GetStatsResponse, error) {
	code := req.GetCode()
	if code == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}
	r, err := s.store.Get(ctx, code)
	if err != nil {
		return nil, mapStoreErr(err, code)
	}
	return &urlov1.GetStatsResponse{Link: s.toProto(r)}, nil
}

func (s *Service) Delete(ctx context.Context, req *urlov1.DeleteRequest) (*urlov1.DeleteResponse, error) {
	code := req.GetCode()
	if code == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}
	if err := s.store.Delete(ctx, code); err != nil {
		return nil, mapStoreErr(err, code)
	}
	return &urlov1.DeleteResponse{}, nil
}

func (s *Service) toProto(r *Record) *urlov1.ShortLink {
	link := &urlov1.ShortLink{
		Code:       r.Code,
		LongUrl:    r.LongURL,
		ShortUrl:   s.buildShortURL(r.Code),
		CreatedAt:  timestamppb.New(r.CreatedAt),
		VisitCount: r.VisitCount,
	}
	if !r.ExpiresAt.IsZero() {
		link.ExpiresAt = timestamppb.New(r.ExpiresAt)
	}
	return link
}

func (s *Service) buildShortURL(code string) string {
	s.mu.RLock()
	base := s.baseURL
	s.mu.RUnlock()
	if base == "" {
		return "/" + code
	}
	return trimRightSlash(base) + "/" + code
}

func mapStoreErr(err error, code string) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return status.Errorf(codes.NotFound, "code %q not found", code)
	case errors.Is(err, ErrAlreadyExists):
		return status.Errorf(codes.AlreadyExists, "code %q already exists", code)
	default:
		return status.Error(codes.Internal, fmt.Sprintf("store: %v", err))
	}
}

func randomCode(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for i, b := range buf {
		out[i] = codeAlphabet[int(b)%len(codeAlphabet)]
	}
	return string(out), nil
}

func isValidCode(s string) bool {
	if len(s) == 0 || len(s) > maxCodeLen {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func trimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
