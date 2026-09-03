package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kongken/urlo/internal/auth"
	"github.com/kongken/urlo/internal/clicks"
	"github.com/kongken/urlo/internal/expander"
	"github.com/kongken/urlo/internal/ratelimit"
	"github.com/kongken/urlo/internal/url"
	urlov1 "github.com/kongken/urlo/pkg/proto/urlo/v1"
)

// shortLinkDTO mirrors urlov1.ShortLink for the HTTP/JSON wire. Using a
// dedicated DTO lets stdlib json.Marshal emit time.Time as RFC 3339 and
// int64 as a JSON number; the proto-generated struct would otherwise
// serialize Timestamp as {seconds, nanos} via the default encoder.
type shortLinkDTO struct {
	Code       string     `json:"code"`
	LongURL    string     `json:"long_url"`
	ShortURL   string     `json:"short_url"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	VisitCount int64      `json:"visit_count"`
	Disabled   bool       `json:"disabled,omitempty"`
}

func toShortLinkDTO(l *urlov1.ShortLink) shortLinkDTO {
	dto := shortLinkDTO{
		Code:       l.GetCode(),
		LongURL:    l.GetLongUrl(),
		ShortURL:   l.GetShortUrl(),
		VisitCount: l.GetVisitCount(),
	}
	if t := l.GetCreatedAt(); t != nil {
		dto.CreatedAt = t.AsTime()
	}
	if t := l.GetExpiresAt(); t != nil {
		ts := t.AsTime()
		dto.ExpiresAt = &ts
	}
	return dto
}

// clickEventDTO mirrors urlov1.ClickEvent for the HTTP/JSON wire.
type clickEventDTO struct {
	ID           string    `json:"id"`
	Code         string    `json:"code"`
	Ts           time.Time `json:"ts"`
	IPHash       string    `json:"ip_hash,omitempty"`
	Country      string    `json:"country,omitempty"`
	City         string    `json:"city,omitempty"`
	Referrer     string    `json:"referrer,omitempty"`
	ReferrerHost string    `json:"referrer_host,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	Browser      string    `json:"browser,omitempty"`
	OS           string    `json:"os,omitempty"`
	Device       string    `json:"device,omitempty"`
	Lang         string    `json:"lang,omitempty"`
	IsBot        bool      `json:"is_bot,omitempty"`
}

type analyticsItemDTO struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type analyticsResponseDTO struct {
	Code      string             `json:"code"`
	StatsType string             `json:"stats_type"`
	From      string             `json:"from,omitempty"`
	To        string             `json:"to,omitempty"`
	Items     []analyticsItemDTO `json:"items"`
}

func toClickEventDTO(e *urlov1.ClickEvent) clickEventDTO {
	dto := clickEventDTO{
		ID:           e.GetId(),
		Code:         e.GetCode(),
		IPHash:       e.GetIpHash(),
		Country:      e.GetCountry(),
		City:         e.GetCity(),
		Referrer:     e.GetReferrer(),
		ReferrerHost: e.GetReferrerHost(),
		UserAgent:    e.GetUserAgent(),
		Browser:      e.GetBrowser(),
		OS:           e.GetOs(),
		Device:       e.GetDevice(),
		Lang:         e.GetLang(),
		IsBot:        e.GetIsBot(),
	}
	if t := e.GetTs(); t != nil {
		dto.Ts = t.AsTime()
	}
	return dto
}

// RegisterRoutes wires the URL shortener HTTP API onto r, backed by svc.
//
// Routes:
//
//	POST   /api/v1/auth/google         -> exchange Google ID token for session cookie
//	POST   /api/v1/auth/logout         -> clear session cookie
//	GET    /api/v1/auth/me             -> current user (200 / 401)
//	POST   /api/v1/expand               -> expand a public third-party URL
//	GET    /api/v1/urls                -> list current user's links (auth required)
//	POST   /api/v1/urls                -> Shorten (anonymous OK; tags owner if logged in)
//	GET    /api/v1/urls/availability   -> check custom-code availability
//	GET    /api/v1/urls/:code          -> Resolve
//	GET    /api/v1/urls/:code/stats    -> GetStats (owner-checked if owned)
//	GET    /api/v1/urls/:code/clicks   -> ListClicks (owner-checked if owned)
//	DELETE /api/v1/urls/:code          -> Delete   (owner-checked if owned)
//	GET    /:code                      -> 302 to the long URL
func RegisterRoutes(r *gin.Engine, svc *url.Service, opts ...Option) {
	o := options{}
	for _, fn := range opts {
		fn(&o)
	}
	if o.externalExpander == nil {
		o.externalExpander = expander.New(expander.Options{})
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	api := r.Group("/api/v1")

	// Optional-auth middleware on every API route: decodes session cookie
	// if present, otherwise lets request through anonymously.
	if o.sessions != nil && o.cookieName != "" {
		api.Use(auth.Middleware(o.sessions, o.cookieName, false))
	}

	// Auth endpoints
	if o.verifier != nil && o.sessions != nil && o.cookieName != "" {
		api.POST("/auth/google", o.apiLimited("auth_google", handleGoogleLogin(o))...)
		api.POST("/auth/logout", handleLogout(o))
		api.GET("/auth/me", handleMe())
	} else {
		api.POST("/auth/google", o.apiLimited("auth_google", handleAuthDisabled)...)
		api.POST("/auth/logout", handleAuthDisabled)
		api.GET("/auth/me", handleAuthDisabled)
	}

	api.POST("/expand", o.apiLimited("expand", handleExpand(o.externalExpander))...)

	// User links (requires auth)
	api.GET("/urls", requireAuth(), handleListMine(svc))

	api.POST("/urls", o.apiLimited("shorten", handleShorten(svc))...)

	api.GET("/urls/availability", o.apiLimited("availability", handleAvailability(svc))...)
	api.GET("/urls/:code", o.apiLimited("resolve", handleResolve(svc))...)
	api.GET("/urls/:code/lookup", o.apiLimited("lookup", handleLookup(svc))...)
	api.GET("/urls/:code/stats", o.apiLimited("stats", handleGetStats(svc))...)
	api.GET("/urls/:code/analytics", o.apiLimited("analytics", handleAnalytics(svc))...)
	api.GET("/urls/:code/clicks", o.apiLimited("clicks", handleListClicks(svc))...)
	api.GET("/urls/:code/status", o.apiLimited("get_status", handleGetStatus(svc))...)
	api.PATCH("/urls/:code", o.apiLimited("update", handleUpdate(svc))...)
	api.PATCH("/urls/:code/status", o.apiLimited("status", handleStatus(svc))...)
	api.DELETE("/urls/:code", o.apiLimited("delete", handleDelete(svc))...)

	r.GET("/:code", handleRedirect(svc, o.ipHashSalt))
}

// Option customises route registration.
type Option func(*options)

type options struct {
	apiLimiter       *ratelimit.Limiter
	verifier         auth.Verifier
	sessions         auth.Sessions
	cookieName       string
	cookieSecure     bool
	cookieTTL        time.Duration
	ipHashSalt       string
	externalExpander *expander.Expander
}

// WithIPHashSalt sets the salt mixed into hashed client IPs in click
// records. Empty disables IP hashing entirely.
func WithIPHashSalt(salt string) Option {
	return func(o *options) { o.ipHashSalt = salt }
}

// WithAPILimiter applies a per-IP rate limiter to public API endpoints.
func WithAPILimiter(l *ratelimit.Limiter) Option {
	return func(o *options) { o.apiLimiter = l }
}

// WithShortenLimiter applies a per-IP rate limiter to public API endpoints.
//
// Deprecated: use WithAPILimiter.
func WithShortenLimiter(l *ratelimit.Limiter) Option {
	return WithAPILimiter(l)
}

// WithAuth wires Google login + session-cookie auth into the API. When
// any of verifier/sessions/cookieName are zero, auth is disabled.
func WithAuth(v auth.Verifier, s auth.Sessions, cookieName string, cookieSecure bool, cookieTTL time.Duration) Option {
	return func(o *options) {
		o.verifier = v
		o.sessions = s
		o.cookieName = cookieName
		o.cookieSecure = cookieSecure
		o.cookieTTL = cookieTTL
	}
}

// WithExpander sets the client used by the public URL expansion endpoint.
// Passing nil restores the default SSRF-safe client.
func WithExpander(e *expander.Expander) Option {
	return func(o *options) { o.externalExpander = e }
}

func (o options) apiLimited(scope string, handlers ...gin.HandlerFunc) []gin.HandlerFunc {
	if o.apiLimiter == nil {
		return handlers
	}
	out := make([]gin.HandlerFunc, 0, len(handlers)+1)
	out = append(out, rateLimitMiddleware(o.apiLimiter, scope))
	out = append(out, handlers...)
	return out
}

func rateLimitMiddleware(l *ratelimit.Limiter, scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			ip = "unknown"
		}
		retryAfter, err := l.Allow(c.Request.Context(), scope+":"+ip)
		if err != nil {
			if errors.Is(err, ratelimit.ErrLimitExceeded) {
				secs := max(int(retryAfter.Seconds()), 1)
				c.Header("Retry-After", strconv.Itoa(secs))
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error":   "rate_limited",
					"message": "too many requests from this IP, try again later",
				})
				return
			}
			// Fail-open on backend errors; do not block legitimate traffic.
		}
		c.Next()
	}
}

func requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if auth.FromGin(c) == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthenticated",
				"message": "login required",
			})
			return
		}
		c.Next()
	}
}

type googleLoginRequest struct {
	IDToken string `json:"id_token"`
}

func handleGoogleLogin(o options) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body googleLoginRequest
		if err := c.ShouldBindJSON(&body); err != nil || body.IDToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "id_token is required"})
			return
		}
		user, err := o.verifier.Verify(c.Request.Context(), body.IDToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token", "message": err.Error()})
			return
		}
		token, err := o.sessions.Issue(user, o.cookieTTL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": err.Error()})
			return
		}
		setSessionCookie(c, o, token, int(o.cookieTTL.Seconds()))
		c.JSON(http.StatusOK, gin.H{"user": user})
	}
}

func handleLogout(o options) gin.HandlerFunc {
	return func(c *gin.Context) {
		setSessionCookie(c, o, "", -1)
		c.Status(http.StatusNoContent)
	}
}

func handleMe() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.FromGin(c)
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user": u})
	}
}

func handleAuthDisabled(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error":   "auth_disabled",
		"message": "google login is not configured on this server",
	})
}

func setSessionCookie(c *gin.Context, o options, value string, maxAge int) {
	// SameSite=Lax: API and frontend are expected on the same origin in
	// production (the front Docker image is served behind the same gateway).
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(o.cookieName, value, maxAge, "/", "", o.cookieSecure, true)
}

type shortenRequest struct {
	LongURL    string `json:"long_url"`
	CustomCode string `json:"custom_code,omitempty"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`
	CodeLength int32  `json:"code_length,omitempty"`
}

type expandRequest struct {
	URL string `json:"url"`
}

const maxExpandRequestBody = 16 << 10

func handleExpand(e *expander.Expander) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxExpandRequestBody)
		var body expandRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   codes.InvalidArgument.String(),
				"message": "request body must contain a valid url field",
			})
			return
		}
		if strings.TrimSpace(body.URL) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   codes.InvalidArgument.String(),
				"message": "url is required",
			})
			return
		}

		result, err := e.Expand(c.Request.Context(), body.URL)
		if err != nil {
			writeExpandError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func handleShorten(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body shortenRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_body",
				"message": err.Error(),
			})
			return
		}
		ownerID := ""
		if u := auth.FromGin(c); u != nil {
			ownerID = u.Sub
		}
		resp, err := svc.ShortenWithOwner(c.Request.Context(), &urlov1.ShortenRequest{
			LongUrl:    body.LongURL,
			CustomCode: body.CustomCode,
			TtlSeconds: body.TTLSeconds,
			CodeLength: body.CodeLength,
		}, ownerID)
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.JSON(http.StatusCreated, toShortLinkDTO(resp.GetLink()))
	}
}

func writeExpandError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, expander.ErrInvalidURL):
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   codes.InvalidArgument.String(),
			"message": "url must be an absolute http or https URL",
		})
	case errors.Is(err, expander.ErrInvalidRedirect):
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   codes.InvalidArgument.String(),
			"message": "the target returned an invalid redirect",
		})
	case errors.Is(err, expander.ErrBlockedURL):
		c.JSON(http.StatusForbidden, gin.H{
			"error":   codes.PermissionDenied.String(),
			"message": "the target URL is blocked by the server network policy",
		})
	case errors.Is(err, expander.ErrTooManyRedirects), errors.Is(err, expander.ErrRedirectLoop):
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   codes.InvalidArgument.String(),
			"message": "the URL redirect chain is not supported",
		})
	case errors.Is(err, expander.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		c.JSON(http.StatusGatewayTimeout, gin.H{
			"error":   codes.DeadlineExceeded.String(),
			"message": "timed out while fetching the target URL",
		})
	default:
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   codes.Unavailable.String(),
			"message": "unable to fetch the target URL",
		})
	}
}

func handleListMine(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.FromGin(c)
		links, err := svc.ListByOwner(c.Request.Context(), u.Sub)
		if err != nil {
			writeStatusError(c, err)
			return
		}
		out := make([]shortLinkDTO, 0, len(links))
		for _, l := range links {
			out = append(out, toShortLinkDTO(l))
		}
		c.JSON(http.StatusOK, gin.H{"links": out})
	}
}

func handleResolve(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := svc.Resolve(c.Request.Context(), &urlov1.ResolveRequest{
			Code: c.Param("code"),
		})
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.JSON(http.StatusOK, toShortLinkDTO(resp.GetLink()))
	}
}

func handleLookup(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := svc.Resolve(c.Request.Context(), &urlov1.ResolveRequest{
			Code: c.Param("code"),
		})
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.JSON(http.StatusOK, toShortLinkDTO(resp.GetLink()))
	}
}

func handleGetStats(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerID := ""
		if u := auth.FromGin(c); u != nil {
			ownerID = u.Sub
		}
		link, err := svc.GetStatsAs(c.Request.Context(), c.Param("code"), ownerID)
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.JSON(http.StatusOK, toShortLinkDTO(link))
	}
}

func parseRFC3339Param(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func handleAnalytics(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerID := ""
		if u := auth.FromGin(c); u != nil {
			ownerID = u.Sub
		}
		from, err := parseRFC3339Param(c.Query("from"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "from must be RFC3339"})
			return
		}
		to, err := parseRFC3339Param(c.Query("to"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "to must be RFC3339"})
			return
		}
		limit, _ := strconv.Atoi(c.Query("limit"))
		resp, err := svc.AnalyticsAs(c.Request.Context(), c.Param("code"), ownerID, url.AnalyticsQuery{
			Type:  url.AnalyticsType(c.Query("stats_type")),
			From:  from,
			To:    to,
			Limit: limit,
		})
		if err != nil {
			writeStatusError(c, err)
			return
		}
		items := make([]analyticsItemDTO, 0, len(resp.Items))
		for _, it := range resp.Items {
			items = append(items, analyticsItemDTO{Key: it.Key, Count: it.Count})
		}
		out := analyticsResponseDTO{
			Code:      resp.Code,
			StatsType: string(resp.StatsType),
			Items:     items,
		}
		if !resp.From.IsZero() {
			out.From = resp.From.UTC().Format(time.RFC3339)
		}
		if !resp.To.IsZero() {
			out.To = resp.To.UTC().Format(time.RFC3339)
		}
		c.JSON(http.StatusOK, out)
	}
}

type updateRequest struct {
	LongURL    *string `json:"long_url"`
	TTLSeconds *int64  `json:"ttl_seconds"`
}

func handleUpdate(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body updateRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_body",
				"message": err.Error(),
			})
			return
		}
		ownerID := ""
		if u := auth.FromGin(c); u != nil {
			ownerID = u.Sub
		}
		link, err := svc.UpdateAs(c.Request.Context(), c.Param("code"), ownerID, url.UpdatePatch{
			LongURL:    body.LongURL,
			TTLSeconds: body.TTLSeconds,
		})
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.JSON(http.StatusOK, toShortLinkDTO(link))
	}
}

type statusRequest struct {
	Disabled bool   `json:"disabled"`
	Reason   string `json:"reason,omitempty"`
}

func handleStatus(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body statusRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_body",
				"message": err.Error(),
			})
			return
		}
		ownerID := ""
		if u := auth.FromGin(c); u != nil {
			ownerID = u.Sub
		}
		rec, err := svc.SetDisabledAs(c.Request.Context(), c.Param("code"), ownerID, body.Disabled, body.Reason)
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":     rec.Code,
			"disabled": rec.Disabled,
			"reason":   rec.DisabledReason,
		})
	}
}

func handleGetStatus(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerID := ""
		if u := auth.FromGin(c); u != nil {
			ownerID = u.Sub
		}
		rec, err := svc.GetStatusAs(c.Request.Context(), c.Param("code"), ownerID)
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":     rec.Code,
			"disabled": rec.Disabled,
			"reason":   rec.DisabledReason,
		})
	}
}

func handleAvailability(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		ok, err := svc.Availability(c.Request.Context(), code)
		if err != nil {
			writeStatusError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":      code,
			"available": ok,
		})
	}
}

func handleDelete(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerID := ""
		if u := auth.FromGin(c); u != nil {
			ownerID = u.Sub
		}
		if err := svc.DeleteAs(c.Request.Context(), c.Param("code"), ownerID); err != nil {
			writeStatusError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func handleRedirect(svc *url.Service, ipHashSalt string) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Param("code")
		resp, err := svc.Resolve(c.Request.Context(), &urlov1.ResolveRequest{Code: code})
		if err != nil {
			writeStatusError(c, err)
			return
		}
		recordClick(svc.Recorder(), c, code, ipHashSalt)
		c.Redirect(http.StatusFound, resp.GetLink().GetLongUrl())
	}
}

func recordClick(rec clicks.Recorder, c *gin.Context, code, salt string) {
	if rec == nil {
		return
	}
	if _, ok := rec.(clicks.Nop); ok {
		return
	}
	ua := c.GetHeader("User-Agent")
	ref := c.GetHeader("Referer")
	browser, osName, device, isBot := clicks.ParseUA(ua)
	evt := clicks.Event{
		Code:         code,
		Timestamp:    time.Now().UTC(),
		IPHash:       clicks.HashIP(c.ClientIP(), salt),
		Referrer:     ref,
		ReferrerHost: clicks.ReferrerHost(ref),
		UserAgent:    ua,
		Browser:      browser,
		OS:           osName,
		Device:       device,
		Lang:         clicks.FirstLang(c.GetHeader("Accept-Language")),
		IsBot:        isBot,
	}
	// Detach from the request context so cancelation on response doesn't
	// race the recorder enqueue.
	rec.Record(context.Background(), evt)
}

func handleListClicks(svc *url.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		ownerID := ""
		if u := auth.FromGin(c); u != nil {
			ownerID = u.Sub
		}
		// Owner check — reuse GetStatsAs which performs ownership verification.
		if _, err := svc.GetStatsAs(c.Request.Context(), c.Param("code"), ownerID); err != nil {
			writeStatusError(c, err)
			return
		}
		size, _ := strconv.Atoi(c.Query("page_size"))
		resp, err := svc.ListClicks(c.Request.Context(), &urlov1.ListClicksRequest{
			Code:      c.Param("code"),
			PageSize:  int32(size),
			PageToken: c.Query("page_token"),
		})
		if err != nil {
			writeStatusError(c, err)
			return
		}
		events := resp.GetEvents()
		out := make([]clickEventDTO, 0, len(events))
		for _, e := range events {
			out = append(out, toClickEventDTO(e))
		}
		c.JSON(http.StatusOK, gin.H{
			"events":          out,
			"next_page_token": resp.GetNextPageToken(),
		})
	}
}

// writeStatusError translates a gRPC status error into a JSON HTTP response.
func writeStatusError(c *gin.Context, err error) {
	st, ok := status.FromError(err)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": err.Error(),
		})
		return
	}
	httpCode := codeToHTTP(st.Code())
	c.JSON(httpCode, gin.H{
		"error":   st.Code().String(),
		"message": st.Message(),
	})
}

func codeToHTTP(c codes.Code) int {
	switch c {
	case codes.OK:
		return http.StatusOK
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}
