// Package expander follows public HTTP redirects to reveal a URL's final
// destination.
package expander

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultTimeout is the total time allowed for one expansion.
	DefaultTimeout = 10 * time.Second
	// DefaultMaxRedirects is the default number of redirects followed.
	DefaultMaxRedirects = 10
	// MaxSupportedRedirects bounds configured redirect limits.
	MaxSupportedRedirects = 32
	// MaxURLLength bounds input and redirect target URLs.
	MaxURLLength = 8192

	maxTimeout             = 30 * time.Second
	maxResponseHeaderBytes = 64 << 10
	defaultUserAgent       = "urlo-url-expander/1.0"
	tcpDialTimeout         = 5 * time.Second
)

var (
	ErrInvalidURL       = errors.New("expander: invalid URL")
	ErrBlockedURL       = errors.New("expander: URL is blocked by network policy")
	ErrTooManyRedirects = errors.New("expander: too many redirects")
	ErrRedirectLoop     = errors.New("expander: redirect loop")
	ErrInvalidRedirect  = errors.New("expander: invalid redirect")
	ErrTimeout          = errors.New("expander: request timed out")
)

// Redirect describes one HTTP redirect response.
type Redirect struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Location   string `json:"location"`
}

// Result is the result of expanding a URL. A non-2xx final HTTP status is
// still a valid result: the URL was reached and is reported to the caller.
type Result struct {
	InputURL      string     `json:"input_url"`
	FinalURL      string     `json:"final_url"`
	StatusCode    int        `json:"status_code"`
	RedirectCount int        `json:"redirect_count"`
	Redirects     []Redirect `json:"redirects"`
}

// Options configures an Expander.
type Options struct {
	// Timeout is the total time allowed for all requests in one expansion.
	// Values <= 0 use DefaultTimeout. Values above 30 seconds are clamped.
	Timeout time.Duration
	// MaxRedirects is the maximum number of redirects to follow. Values <= 0
	// use DefaultMaxRedirects. Values above MaxSupportedRedirects are clamped.
	MaxRedirects int
	// UserAgent identifies requests made by the expander.
	UserAgent string
	// HTTPClient is intended for tests and controlled integrations. When nil,
	// New builds a client with the SSRF-safe transport used in production.
	HTTPClient *http.Client
}

// Expander follows HTTP redirects using a bounded, SSRF-aware client.
type Expander struct {
	client       *http.Client
	timeout      time.Duration
	maxRedirects int
	userAgent    string
}

// New constructs an Expander.
func New(opts Options) *Expander {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > maxTimeout {
		timeout = maxTimeout
	}

	maxRedirects := opts.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = DefaultMaxRedirects
	}
	if maxRedirects > MaxSupportedRedirects {
		maxRedirects = MaxSupportedRedirects
	}

	userAgent := strings.TrimSpace(opts.UserAgent)
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{
			Transport: safeTransport(timeout),
		}
	} else {
		// Do not mutate a caller-owned client. Tests can provide a local
		// RoundTripper without weakening the production default.
		copy := *client
		client = &copy
	}
	client.CheckRedirect = noRedirect
	client.Timeout = timeout

	return &Expander{
		client:       client,
		timeout:      timeout,
		maxRedirects: maxRedirects,
		userAgent:    userAgent,
	}
}

// Expand follows HTTP redirects from rawURL and returns the final response
// URL and status. Only http and https targets are supported.
func (e *Expander) Expand(ctx context.Context, rawURL string) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	current, err := parseTarget(rawURL)
	if err != nil {
		return Result{}, err
	}

	ctx, cancel := context.WithTimeoutCause(ctx, e.timeout, ErrTimeout)
	defer cancel()

	inputURL := current.String()
	visited := map[string]struct{}{visitKey(current): {}}
	redirects := make([]Redirect, 0, min(e.maxRedirects, 4))

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, current.String(), nil)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrInvalidURL, err)
		}
		req.Header.Set("Accept", "*/*")
		req.Header.Set("User-Agent", e.userAgent)

		resp, err := e.client.Do(req)
		if err != nil {
			fetchErr := &FetchError{URL: current.String(), Err: err}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return Result{}, fmt.Errorf("%w: %w", ErrTimeout, fetchErr)
			}
			return Result{}, fetchErr
		}

		statusCode := resp.StatusCode
		location := resp.Header.Get("Location")
		if resp.Body != nil {
			_ = resp.Body.Close()
		}

		if !isRedirect(statusCode) || location == "" {
			return Result{
				InputURL:      inputURL,
				FinalURL:      current.String(),
				StatusCode:    statusCode,
				RedirectCount: len(redirects),
				Redirects:     redirects,
			}, nil
		}

		if len(redirects) >= e.maxRedirects {
			return Result{}, fmt.Errorf("%w: maximum is %d", ErrTooManyRedirects, e.maxRedirects)
		}

		ref, err := url.Parse(location)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrInvalidRedirect, err)
		}
		next := current.ResolveReference(ref)
		if err := validateTarget(next); err != nil {
			return Result{}, fmt.Errorf("%w: redirect target rejected", err)
		}
		key := visitKey(next)
		if _, ok := visited[key]; ok {
			return Result{}, fmt.Errorf("%w: %s", ErrRedirectLoop, next.String())
		}
		visited[key] = struct{}{}

		redirects = append(redirects, Redirect{
			URL:        current.String(),
			StatusCode: statusCode,
			Location:   next.String(),
		})
		current = next
	}
}

// FetchError identifies a failure while contacting a target URL.
type FetchError struct {
	URL string
	Err error
}

func (e *FetchError) Error() string {
	return fmt.Sprintf("fetch %q: %v", e.URL, e.Err)
}

func (e *FetchError) Unwrap() error {
	return e.Err
}

func noRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func parseTarget(raw string) (*url.URL, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > MaxURLLength || strings.ContainsAny(value, "\r\n") {
		return nil, ErrInvalidURL
	}

	u, err := url.ParseRequestURI(value)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return nil, fmt.Errorf("%w: only http and https are supported", ErrInvalidURL)
	}
	if u.Host == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("%w: absolute URL with a host is required", ErrInvalidURL)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: URL credentials are not supported", ErrInvalidURL)
	}

	u.Scheme = strings.ToLower(u.Scheme)
	if err := validateTarget(u); err != nil {
		return nil, err
	}
	return u, nil
}

func validateTarget(u *url.URL) error {
	if u == nil || u.Host == "" {
		return ErrInvalidURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}
	if len(u.String()) > MaxURLLength {
		return fmt.Errorf("%w: URL exceeds %d characters", ErrInvalidURL, MaxURLLength)
	}
	if u.User != nil {
		return fmt.Errorf("%w: URL credentials are not supported", ErrInvalidURL)
	}

	host := u.Hostname()
	if err := validateHostName(host); err != nil {
		return err
	}
	if ip, ok := parseIPHost(host); ok && !isPublicIP(ip) {
		return fmt.Errorf("%w: %s", ErrBlockedURL, host)
	}
	return nil
}

func validateHostName(host string) error {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return ErrInvalidURL
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		host == "metadata" || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".home.arpa") ||
		strings.HasSuffix(host, ".test") || strings.HasSuffix(host, ".invalid") ||
		strings.HasSuffix(host, ".example") {
		return fmt.Errorf("%w: host %q", ErrBlockedURL, host)
	}
	return nil
}

func parseIPHost(host string) (netip.Addr, bool) {
	if before, _, ok := strings.Cut(host, "%"); ok {
		host = before
	}
	ip, err := netip.ParseAddr(host)
	return ip, err == nil
}

func isPublicIP(ip netip.Addr) bool {
	if ip.Is4In6() {
		ip = ip.Unmap()
	}
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

var blockedPrefixes = [...]netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),   // Shared address space.
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments.
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1.
	netip.MustParsePrefix("198.18.0.0/15"),   // Benchmarking.
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2.
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3.
	netip.MustParsePrefix("2001:db8::/32"),   // Documentation.
}

func safeTransport(timeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy:                  nil,
		DialContext:            safeDialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           16,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    min(timeout, 5*time.Second),
		ResponseHeaderTimeout:  timeout,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: maxResponseHeaderBytes,
	}
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split target address: %w", err)
	}
	if err := validateHostName(host); err != nil {
		return nil, err
	}

	var ips []netip.Addr
	if ip, ok := parseIPHost(host); ok {
		ips = []netip.Addr{ip}
	} else {
		ips, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("resolve %q: no addresses", host)
		}
	}

	for _, ip := range ips {
		if !isPublicIP(ip) {
			return nil, fmt.Errorf("%w: %s resolves to %s", ErrBlockedURL, host, ip)
		}
	}

	dialer := net.Dialer{Timeout: tcpDialTimeout}
	var lastErr error
	for _, ip := range ips {
		ip = ip.Unmap()
		if network == "tcp4" && !ip.Is4() {
			continue
		}
		if network == "tcp6" && ip.Is4() {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		return nil, fmt.Errorf("no address matches network %q", network)
	}
	return nil, fmt.Errorf("dial %q: %w", host, lastErr)
}

func isRedirect(statusCode int) bool {
	switch statusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func visitKey(u *url.URL) string {
	copy := *u
	copy.Host = strings.ToLower(copy.Host)
	copy.Fragment = ""
	return copy.String()
}
