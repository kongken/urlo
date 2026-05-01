package main

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"butterfly.orx.me/core/app"
	butterflyredis "butterfly.orx.me/core/store/redis"
	butterflys3 "butterfly.orx.me/core/store/s3"
	"github.com/gin-gonic/gin"
	"github.com/kongken/urlo/internal/auth"
	"github.com/kongken/urlo/internal/clicks"
	clicksredis "github.com/kongken/urlo/internal/clicks/redisstream"
	"github.com/kongken/urlo/internal/config"
	apihttp "github.com/kongken/urlo/internal/http"
	"github.com/kongken/urlo/internal/ratelimit"
	"github.com/kongken/urlo/internal/url"
	"github.com/kongken/urlo/internal/url/s3store"
	urlov1 "github.com/kongken/urlo/pkg/proto/urlo/v1"
	"google.golang.org/grpc"
)

func main() {
	svcConfig := &config.ServiceConfig{}
	urlSvc := url.NewService(url.Options{})
	var (
		shortenLimiter *ratelimit.Limiter
		verifier       auth.Verifier
		sessions       auth.Sessions
		cookieName     string
		cookieSecure   bool
		cookieTTL      time.Duration
		clickIPSalt    string
	)

	appConfig := &app.Config{
		Service:   "urlo",
		Namespace: "kongken",
		Config:    svcConfig,
		Router: func(r *gin.Engine) {
			r.GET("/ping", func(c *gin.Context) {
				c.JSON(200, gin.H{"message": "pong"})
			})
			apihttp.RegisterRoutes(r, urlSvc,
				apihttp.WithShortenLimiter(shortenLimiter),
				apihttp.WithAuth(verifier, sessions, cookieName, cookieSecure, cookieTTL),
				apihttp.WithIPHashSalt(clickIPSalt),
			)
		},
		GRPCRegister: func(s *grpc.Server) {
			urlov1.RegisterUrlServiceServer(s, urlSvc)
		},
		InitFunc: []func() error{
			func() error {
				urlSvc.SetBaseURL(svcConfig.BaseURL)
				store, err := buildStore(svcConfig.Storage)
				if err != nil {
					return fmt.Errorf("build store: %w", err)
				}
				urlSvc.SetStore(store)
				slog.Info("url service ready", "store", svcConfig.Storage.Driver)

				l, err := buildShortenLimiter(svcConfig.RateLimit)
				if err != nil {
					return fmt.Errorf("build rate limiter: %w", err)
				}
				if l != nil {
					shortenLimiter = l
					slog.Info("shorten rate limit enabled",
						"per_hour", svcConfig.RateLimit.PerHour,
						"redis", svcConfig.RateLimit.RedisConfigName)
				}

				if v, s, name, secure, ttl, err := buildAuth(svcConfig.Auth); err != nil {
					return fmt.Errorf("build auth: %w", err)
				} else if v != nil {
					verifier, sessions, cookieName, cookieSecure, cookieTTL = v, s, name, secure, ttl
					slog.Info("google login enabled", "cookie", name, "ttl", ttl)
				} else {
					slog.Info("google login disabled (auth.google.client_id not set)")
				}

				rec, err := buildClickRecorder(svcConfig.Clicks)
				if err != nil {
					return fmt.Errorf("build click recorder: %w", err)
				}
				if rec != nil {
					urlSvc.SetRecorder(rec)
					clickIPSalt = svcConfig.Clicks.IPHashSalt
					slog.Info("click recorder enabled",
						"driver", svcConfig.Clicks.Driver,
						"max_len", svcConfig.Clicks.MaxLen)
				}
				return nil
			},
		},
	}

	app.New(appConfig).Run()
}

func buildAuth(c config.AuthConfig) (auth.Verifier, auth.Sessions, string, bool, time.Duration, error) {
	if c.Google.ClientID == "" {
		return nil, nil, "", false, 0, nil
	}
	if c.Session.JWTSecret == "" {
		return nil, nil, "", false, 0, errors.New("auth.session.jwt_secret is required when auth.google.client_id is set")
	}
	sess, err := auth.NewHMACSessions(c.Session.JWTSecret)
	if err != nil {
		return nil, nil, "", false, 0, err
	}
	cookieName := c.Session.CookieName
	if cookieName == "" {
		cookieName = "urlo_session"
	}
	ttlHours := c.Session.TTLHours
	if ttlHours <= 0 {
		ttlHours = 168
	}
	return auth.NewGoogleVerifier(c.Google.ClientID), sess, cookieName, c.Session.Secure, time.Duration(ttlHours) * time.Hour, nil
}

func buildShortenLimiter(c config.RateLimitConfig) (*ratelimit.Limiter, error) {
	if !c.Enabled {
		return nil, nil
	}
	if c.PerHour <= 0 {
		return nil, errors.New("rate_limit.per_hour must be > 0 when enabled")
	}
	if c.RedisConfigName == "" {
		return nil, errors.New("rate_limit.redis_config_name is required when enabled")
	}
	client := butterflyredis.GetClient(c.RedisConfigName)
	if client == nil {
		return nil, fmt.Errorf("redis client %q not found in butterfly store config", c.RedisConfigName)
	}
	return ratelimit.New(client, "urlo:ratelimit:shorten", c.PerHour), nil
}

func buildClickRecorder(c config.ClicksConfig) (clicks.Recorder, error) {
	switch c.Driver {
	case "", "none":
		return nil, nil
	case "redis_stream":
		if c.RedisConfigName == "" {
			return nil, errors.New("clicks.redis_config_name is required for redis_stream driver")
		}
		client := butterflyredis.GetClient(c.RedisConfigName)
		if client == nil {
			return nil, fmt.Errorf("redis client %q not found in butterfly store config", c.RedisConfigName)
		}
		prefix := c.KeyPrefix
		if prefix == "" {
			prefix = "clicks"
		}
		return clicksredis.New(clicksredis.Options{
			Client:     client,
			KeyPrefix:  prefix,
			MaxLen:     c.MaxLen,
			BufferSize: c.BufferSize,
		})
	default:
		return nil, fmt.Errorf("unknown clicks driver %q", c.Driver)
	}
}

func buildStore(c config.StorageConfig) (url.Store, error) {
	switch c.Driver {
	case "", "memory":
		return url.NewMemoryStore(), nil
	case "s3":
		if c.S3.ConfigName == "" {
			return nil, errors.New("storage.s3.config_name is required")
		}
		client := butterflys3.GetClient(c.S3.ConfigName)
		if client == nil {
			return nil, fmt.Errorf("s3 client %q not found in butterfly store config", c.S3.ConfigName)
		}
		bucket := butterflys3.GetBucket(c.S3.ConfigName)
		if bucket == "" {
			return nil, fmt.Errorf("s3 bucket for %q is empty", c.S3.ConfigName)
		}
		return s3store.New(s3store.Options{
			Client: client,
			Bucket: bucket,
			Prefix: c.S3.Prefix,
		})
	default:
		return nil, fmt.Errorf("unknown storage driver %q", c.Driver)
	}
}
