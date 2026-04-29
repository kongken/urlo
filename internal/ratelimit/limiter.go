// Package ratelimit wraps github.com/go-redis/redis_rate as a thin
// per-IP limiter for URL creation.
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
)

// ErrLimitExceeded is returned by Allow when the caller has exhausted
// its quota for the current window.
var ErrLimitExceeded = errors.New("rate limit exceeded")

// Limiter enforces a per-key, per-hour quota using GCRA via redis_rate.
type Limiter struct {
	limiter *redis_rate.Limiter
	prefix  string
	limit   redis_rate.Limit
	max     int
}

// New constructs a Limiter that allows at most perHour events per key
// per hour. Keys are namespaced under prefix.
func New(client *redis.Client, prefix string, perHour int) *Limiter {
	if prefix == "" {
		prefix = "ratelimit"
	}
	return &Limiter{
		limiter: redis_rate.NewLimiter(client),
		prefix:  prefix,
		limit:   redis_rate.PerHour(perHour),
		max:     perHour,
	}
}

// Allow consumes one token for key. It returns the time until the
// caller may retry and an error wrapping ErrLimitExceeded when the
// quota is exhausted.
func (l *Limiter) Allow(ctx context.Context, key string) (retryAfter time.Duration, err error) {
	if l == nil || l.limiter == nil || l.max <= 0 {
		return 0, nil
	}
	res, err := l.limiter.Allow(ctx, fmt.Sprintf("%s:%s", l.prefix, key), l.limit)
	if err != nil {
		return 0, fmt.Errorf("ratelimit: %w", err)
	}
	if res.Allowed <= 0 {
		return res.RetryAfter, ErrLimitExceeded
	}
	return 0, nil
}

// Limit returns the configured per-window limit.
func (l *Limiter) Limit() int {
	if l == nil {
		return 0
	}
	return l.max
}
