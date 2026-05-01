package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kongken/urlo/internal/auth"
	"github.com/kongken/urlo/internal/clicks"
	"github.com/kongken/urlo/internal/ratelimit"
	"github.com/kongken/urlo/internal/url"
	urlov1 "github.com/kongken/urlo/pkg/proto/urlo/v1"
)

// RegisterRoutes wires the URL shortener HTTP API onto r, backed by svc.
//
// Routes:
//
//	POST   /api/v1/auth/google         -> exchange Google ID token for session cookie
//	POST   /api/v1/auth/logout         -> clear session cookie
//	GET    /api/v1/auth/me             -> current user (200 / 401)
//	GET    /api/v1/urls                -> list current user's links (auth required)
//	POST   /api/v1/urls                -> Shorten (anonymous OK; tags owner if logged in)
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
		api.POST("/auth/google", handleGoogleLogin(o))
		api.POST("/auth/logout", handleLogout(o))
		api.GET("/auth/me", handleMe())
	} else {
		api.POST("/auth/google", handleAuthDisabled)
		api.POST("/auth/logout", handleAuthDisabled)
		api.GET("/auth/me", handleAuthDisabled)
	}

	// User links (requires auth)
	api.GET("/urls", requireAuth(), handleListMine(svc))

	// Shorten
	shorten := []gin.HandlerFunc{}
	if o.shortenLimiter != nil {
		shorten = append(shorten, rateLimitMiddleware(o.shortenLimiter, "shorten"))
	}
	shorten = append(shorten, handleShorten(svc))
	api.POST("/urls", shorten...)

	api.GET("/urls/:code", handleResolve(svc))
	api.GET("/urls/:code/stats", handleGetStats(svc))
	api.GET("/urls/:code/clicks", handleListClicks(svc))
	api.DELETE("/urls/:code", handleDelete(svc))

	r.GET("/:code", handleRedirect(svc, o.ipHashSalt))
}

// Option customises route registration.
type Option func(*options)

type options struct {
	shortenLimiter *ratelimit.Limiter
	verifier       auth.Verifier
	sessions       auth.Sessions
	cookieName     string
	cookieSecure   bool
	cookieTTL      time.Duration
	ipHashSalt     string
}

// WithIPHashSalt sets the salt mixed into hashed client IPs in click
// records. Empty disables IP hashing entirely.
func WithIPHashSalt(salt string) Option {
	return func(o *options) { o.ipHashSalt = salt }
}

// WithShortenLimiter applies a per-IP rate limiter to POST /api/v1/urls.
func WithShortenLimiter(l *ratelimit.Limiter) Option {
	return func(o *options) { o.shortenLimiter = l }
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
		c.JSON(http.StatusCreated, resp.GetLink())
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
		c.JSON(http.StatusOK, gin.H{"links": links})
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
		c.JSON(http.StatusOK, resp.GetLink())
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
		c.JSON(http.StatusOK, link)
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
		c.JSON(http.StatusOK, gin.H{
			"events":          resp.GetEvents(),
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
