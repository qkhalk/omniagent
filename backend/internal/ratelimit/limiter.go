// Package ratelimit implements a per-key token-bucket limiter.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a per-key token-bucket limiter.
type Limiter struct {
	mu     sync.Mutex
	rps    float64
	burst  int
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New returns a new limiter.
func New(rps float64, burst int) *Limiter {
	return &Limiter{rps: rps, burst: burst, buckets: make(map[string]*bucket)}
}

// Allow returns true if the key may proceed now.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(l.burst), last: now}
		l.buckets[key] = b
	}
	// refill
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * l.rps
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
