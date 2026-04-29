package ratelimit

import (
	"context"
	"testing"
)

func TestNilLimiterAllows(t *testing.T) {
	var l *Limiter
	retry, err := l.Allow(context.Background(), "any")
	if err != nil || retry != 0 {
		t.Fatalf("nil limiter must noop, got retry=%v err=%v", retry, err)
	}
	if got := l.Limit(); got != 0 {
		t.Fatalf("nil limiter Limit() = %d, want 0", got)
	}
}

func TestZeroLimitAllows(t *testing.T) {
	l := New(nil, "", 0)
	retry, err := l.Allow(context.Background(), "any")
	if err != nil || retry != 0 {
		t.Fatalf("zero-limit limiter must noop, got retry=%v err=%v", retry, err)
	}
}
