package url

import (
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"sync"
	"sync/atomic"
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

type entry struct {
	longURL    string
	createdAt  time.Time
	expiresAt  time.Time
	visitCount atomic.Int64
}

// Service implements urlov1.UrlServiceServer with in-memory storage.
// Concurrency-safe; suitable for single-instance deployments and tests.
type Service struct {
	urlov1.UnimplementedUrlServiceServer

	baseURL string

	mu    sync.RWMutex
	links map[string]*entry
}

type Options struct {
	// BaseURL is prepended to codes when building ShortLink.short_url.
	// e.g. "https://urlo.example".
	BaseURL string
}

func NewService(opts Options) *Service {
	return &Service{
		baseURL: opts.BaseURL,
		links:   make(map[string]*entry),
	}
}

// SetBaseURL updates the base URL used to build ShortLink.short_url.
// Intended to be called once during app init after config is loaded.
func (s *Service) SetBaseURL(baseURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseURL = baseURL
}

func (s *Service) Shorten(ctx context.Context, req *urlov1.ShortenRequest) (*urlov1.ShortenResponse, error) {
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

	code := req.GetCustomCode()
	s.mu.Lock()
	defer s.mu.Unlock()

	if code != "" {
		if !isValidCode(code) {
			return nil, status.Error(codes.InvalidArgument, "custom_code must be 1-32 chars from [A-Za-z0-9]")
		}
		if _, exists := s.links[code]; exists {
			return nil, status.Errorf(codes.AlreadyExists, "code %q already exists", code)
		}
	} else {
		generated, err := s.generateUniqueCodeLocked(defaultCodeLen)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "generate code: %v", err)
		}
		code = generated
	}

	e := &entry{
		longURL:   req.GetLongUrl(),
		createdAt: now,
		expiresAt: expiresAt,
	}
	s.links[code] = e

	return &urlov1.ShortenResponse{Link: s.toProto(code, e)}, nil
}

func (s *Service) Resolve(ctx context.Context, req *urlov1.ResolveRequest) (*urlov1.ResolveResponse, error) {
	code := req.GetCode()
	if code == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}

	s.mu.RLock()
	e, ok := s.links[code]
	s.mu.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "code %q not found", code)
	}
	if expired(e) {
		s.mu.Lock()
		delete(s.links, code)
		s.mu.Unlock()
		return nil, status.Errorf(codes.NotFound, "code %q expired", code)
	}

	e.visitCount.Add(1)
	return &urlov1.ResolveResponse{Link: s.toProto(code, e)}, nil
}

func (s *Service) GetStats(ctx context.Context, req *urlov1.GetStatsRequest) (*urlov1.GetStatsResponse, error) {
	code := req.GetCode()
	if code == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}

	s.mu.RLock()
	e, ok := s.links[code]
	s.mu.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "code %q not found", code)
	}
	return &urlov1.GetStatsResponse{Link: s.toProto(code, e)}, nil
}

func (s *Service) Delete(ctx context.Context, req *urlov1.DeleteRequest) (*urlov1.DeleteResponse, error) {
	code := req.GetCode()
	if code == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.links[code]; !ok {
		return nil, status.Errorf(codes.NotFound, "code %q not found", code)
	}
	delete(s.links, code)
	return &urlov1.DeleteResponse{}, nil
}

func (s *Service) toProto(code string, e *entry) *urlov1.ShortLink {
	link := &urlov1.ShortLink{
		Code:       code,
		LongUrl:    e.longURL,
		ShortUrl:   s.buildShortURL(code),
		CreatedAt:  timestamppb.New(e.createdAt),
		VisitCount: e.visitCount.Load(),
	}
	if !e.expiresAt.IsZero() {
		link.ExpiresAt = timestamppb.New(e.expiresAt)
	}
	return link
}

func (s *Service) buildShortURL(code string) string {
	if s.baseURL == "" {
		return "/" + code
	}
	return trimRightSlash(s.baseURL) + "/" + code
}

func (s *Service) generateUniqueCodeLocked(length int) (string, error) {
	for range 8 {
		code, err := randomCode(length)
		if err != nil {
			return "", err
		}
		if _, exists := s.links[code]; !exists {
			return code, nil
		}
	}
	return "", errors.New("failed to generate unique code after 8 attempts")
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

func expired(e *entry) bool {
	return !e.expiresAt.IsZero() && time.Now().UTC().After(e.expiresAt)
}

func trimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
