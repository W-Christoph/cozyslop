// Package ratelimit provides keyed token buckets (per IP, per identity).
package ratelimit

import (
	"sync"
	"time"
)

// Limiter allows bursts of up to Burst events per key, refilling one token
// every Every.
type Limiter struct {
	burst float64
	every time.Duration
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	sweep   time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func New(burst int, every time.Duration) *Limiter {
	return &Limiter{
		burst:   float64(burst),
		every:   every,
		now:     time.Now,
		buckets: make(map[string]*bucket),
	}
}

// Allow takes a token for key and reports whether one was available.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.maybeSweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens += float64(now.Sub(b.last)) / float64(l.every)
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// maybeSweep drops buckets that have refilled completely, so the map does
// not grow with every IP ever seen.
func (l *Limiter) maybeSweep(now time.Time) {
	full := time.Duration(l.burst) * l.every
	if now.Sub(l.sweep) < full {
		return
	}
	l.sweep = now
	for k, b := range l.buckets {
		if now.Sub(b.last) >= full {
			delete(l.buckets, k)
		}
	}
}
