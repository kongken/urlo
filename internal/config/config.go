package config

import "log/slog"

type ServiceConfig struct {
	Environment string          `yaml:"environment"`
	BaseURL     string          `yaml:"base_url"`
	Storage     StorageConfig   `yaml:"storage"`
	RateLimit   RateLimitConfig `yaml:"rate_limit"`
	Auth        AuthConfig      `yaml:"auth"`
}

// AuthConfig configures Google login + session JWT.
//
// When Google.ClientID is empty, auth is disabled: the /api/v1/auth/*
// routes return 503, the frontend sees no logged-in user, and all links
// remain anonymous.
type AuthConfig struct {
	Google  GoogleAuthConfig `yaml:"google"`
	Session SessionConfig    `yaml:"session"`
}

type GoogleAuthConfig struct {
	ClientID string `yaml:"client_id"`
}

type SessionConfig struct {
	JWTSecret  string `yaml:"jwt_secret"`
	CookieName string `yaml:"cookie_name"`
	TTLHours   int    `yaml:"ttl_hours"`
	Secure     bool   `yaml:"secure"`
}

// RateLimitConfig configures per-IP rate limiting on URL creation.
//
// When Enabled is true, the service enforces at most PerHour shorten
// requests per client IP per hour using the butterfly redis client
// identified by RedisConfigName.
type RateLimitConfig struct {
	Enabled         bool   `yaml:"enabled"`
	RedisConfigName string `yaml:"redis_config_name"`
	PerHour         int    `yaml:"per_hour"`
}

// StorageConfig selects the backing store for short links.
//
// Driver values:
//   - "memory" (default): in-process map; resets on restart.
//   - "s3": uses the butterfly S3 client identified by S3.ConfigName,
//     storing each link as a JSON object under S3.Prefix.
type StorageConfig struct {
	Driver string          `yaml:"driver"`
	S3     S3StorageConfig `yaml:"s3"`
}

type S3StorageConfig struct {
	// ConfigName matches a key under butterfly's `store.s3.<name>` config.
	ConfigName string `yaml:"config_name"`
	// Prefix is the object-key prefix (default "links").
	Prefix string `yaml:"prefix"`
}

func (c *ServiceConfig) Print() {
	slog.Info("service config loaded",
		"environment", c.Environment,
		"base_url", c.BaseURL,
		"storage_driver", c.Storage.Driver,
	)
}
