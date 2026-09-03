package expander

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testResponse(req *http.Request, statusCode int, location string) *http.Response {
	header := make(http.Header)
	if location != "" {
		header.Set("Location", location)
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("ignored")),
		Request:    req,
	}
}

func TestExpandFollowsRedirectChain(t *testing.T) {
	var requests []string
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests = append(requests, req.URL.String())
			switch req.URL.String() {
			case "https://short.com/start":
				return testResponse(req, http.StatusMovedPermanently, "/step-1"), nil
			case "https://short.com/step-1":
				return testResponse(req, http.StatusFound, "HTTP://destination.com/article"), nil
			case "http://destination.com/article":
				return testResponse(req, http.StatusOK, ""), nil
			default:
				return nil, errors.New("unexpected URL: " + req.URL.String())
			}
		}),
	}
	expander := New(Options{HTTPClient: client, MaxRedirects: 3})

	got, err := expander.Expand(t.Context(), " https://short.com/start ")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if got.InputURL != "https://short.com/start" {
		t.Errorf("InputURL = %q", got.InputURL)
	}
	if got.FinalURL != "http://destination.com/article" {
		t.Errorf("FinalURL = %q", got.FinalURL)
	}
	if got.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d", got.StatusCode)
	}
	if got.RedirectCount != 2 || len(got.Redirects) != 2 {
		t.Fatalf("redirect count = %d, redirects = %#v", got.RedirectCount, got.Redirects)
	}
	if got.Redirects[0].Location != "https://short.com/step-1" {
		t.Errorf("first redirect location = %q", got.Redirects[0].Location)
	}
	if got.Redirects[1].Location != "http://destination.com/article" {
		t.Errorf("second redirect location = %q", got.Redirects[1].Location)
	}
	if len(requests) != 3 {
		t.Errorf("request count = %d, want 3", len(requests))
	}
}

func TestExpandReturnsFinalNonSuccessResponse(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return testResponse(req, http.StatusNotFound, ""), nil
		}),
	}

	got, err := New(Options{HTTPClient: client}).Expand(t.Context(), "https://destination.com/missing")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if got.FinalURL != "https://destination.com/missing" {
		t.Errorf("FinalURL = %q", got.FinalURL)
	}
	if got.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d", got.StatusCode)
	}
}

func TestExpandRejectsInvalidAndBlockedTargets(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want error
	}{
		{name: "empty", raw: "", want: ErrInvalidURL},
		{name: "relative", raw: "/path", want: ErrInvalidURL},
		{name: "unsupported scheme", raw: "ftp://example.com/file", want: ErrInvalidURL},
		{name: "credentials", raw: "https://user:pass@example.com/", want: ErrInvalidURL},
		{name: "loopback", raw: "http://127.0.0.1/", want: ErrBlockedURL},
		{name: "localhost", raw: "http://localhost/", want: ErrBlockedURL},
		{name: "ipv6 loopback", raw: "http://[::1]/", want: ErrBlockedURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(Options{}).Expand(t.Context(), tc.raw)
			if !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want errors.Is(_, %v)", err, tc.want)
			}
		})
	}
}

func TestExpandRejectsBlockedRedirect(t *testing.T) {
	calls := 0
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return testResponse(req, http.StatusFound, "http://127.0.0.1/private"), nil
		}),
	}

	_, err := New(Options{HTTPClient: client}).Expand(t.Context(), "https://short.com/start")
	if !errors.Is(err, ErrBlockedURL) {
		t.Fatalf("error = %v, want blocked URL", err)
	}
	if calls != 1 {
		t.Errorf("request count = %d, want 1", calls)
	}
}

func TestExpandStopsAtRedirectLimit(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return testResponse(req, http.StatusFound, req.URL.Path+"/next"), nil
		}),
	}

	_, err := New(Options{HTTPClient: client, MaxRedirects: 2}).Expand(t.Context(), "https://short.com/start")
	if !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("error = %v, want too many redirects", err)
	}
}

func TestExpandDetectsRedirectLoop(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/start" {
				return testResponse(req, http.StatusFound, "/next"), nil
			}
			return testResponse(req, http.StatusFound, "/start"), nil
		}),
	}

	_, err := New(Options{HTTPClient: client}).Expand(t.Context(), "https://short.com/start")
	if !errors.Is(err, ErrRedirectLoop) {
		t.Fatalf("error = %v, want redirect loop", err)
	}
}

func TestExpandTimesOut(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}),
	}

	_, err := New(Options{HTTPClient: client, Timeout: time.Millisecond}).Expand(t.Context(), "https://slow.com/")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v, want timeout", err)
	}
}

func TestSafeDialContextRejectsPrivateAddresses(t *testing.T) {
	_, err := safeDialContext(context.Background(), "tcp", "127.0.0.1:80")
	if !errors.Is(err, ErrBlockedURL) {
		t.Fatalf("error = %v, want blocked URL", err)
	}
}
