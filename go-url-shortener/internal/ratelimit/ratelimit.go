// Package ratelimit provides a small per-key token-bucket limiter.
package ratelimit

import (
	"math"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter allows `burst` requests at once per key, refilled at `rate` per second.
type Limiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*bucket
	now     func() time.Time
	calls   int
}

// New creates a limiter. now may be nil (time.Now).
func New(ratePerSec float64, burst int, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{rate: ratePerSec, burst: float64(burst), buckets: map[string]*bucket{}, now: now}
}

// Allow consumes one token for key. When denied, retryAfter says how long until a token is available.
func (l *Limiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.calls++
	if l.calls%1024 == 0 {
		l.sweep(now)
	}
	b, found := l.buckets[key]
	if !found {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	need := (1 - b.tokens) / l.rate
	return false, time.Duration(need * float64(time.Second))
}

// sweep drops buckets that have fully refilled (they are equivalent to a new one).
func (l *Limiter) sweep(now time.Time) {
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.rate >= l.burst {
			delete(l.buckets, k)
		}
	}
}
