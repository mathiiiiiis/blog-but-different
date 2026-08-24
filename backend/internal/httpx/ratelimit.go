package httpx

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type bucket struct {
	limiter *rate.Limiter
	seen    time.Time
}

// Limiter applies a per-client token bucket. Idle entries are evicted so a
// stream of distinct source addresses cannot grow the map without bound.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket

	rate       rate.Limit
	burst      int
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

// NewLimiter allows `perMinute` requests per client per minute, letting a
// client spend up to that many in one burst.
func NewLimiter(perMinute int) *Limiter {
	if perMinute < 1 {
		perMinute = 1
	}
	return &Limiter{
		buckets:    make(map[string]*bucket),
		rate:       rate.Limit(float64(perMinute) / 60.0),
		burst:      perMinute,
		ttl:        10 * time.Minute,
		maxEntries: 50_000,
		now:        time.Now,
	}
}

func (l *Limiter) Allow(key string) bool {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxEntries {
			l.evictLocked(now)
		}
		b = &bucket{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.buckets[key] = b
	}
	b.seen = now
	return b.limiter.AllowN(now, 1)
}

func (l *Limiter) evictLocked(now time.Time) {
	for key, b := range l.buckets {
		if now.Sub(b.seen) > l.ttl {
			delete(l.buckets, key)
		}
	}
	if len(l.buckets) < l.maxEntries {
		return
	}
	// Still full of active clients: drop the coldest half so the map stays bounded.
	oldest := now.Add(-l.ttl / 2)
	for key, b := range l.buckets {
		if b.seen.Before(oldest) {
			delete(l.buckets, key)
		}
	}
}

// Sweep evicts stale buckets until the context's channel closes.
func (l *Limiter) Sweep(done <-chan struct{}) {
	ticker := time.NewTicker(l.ttl)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case now := <-ticker.C:
			l.mu.Lock()
			for key, b := range l.buckets {
				if now.Sub(b.seen) > l.ttl {
					delete(l.buckets, key)
				}
			}
			l.mu.Unlock()
		}
	}
}

// Limit wraps a handler, rejecting callers that exceed their budget.
func (l *Limiter) Limit(trustProxy bool, next Handler) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if !l.Allow(ClientIP(r, trustProxy)) {
			w.Header().Set("Retry-After", "60")
			return Errorf(http.StatusTooManyRequests, "Rate limit exceeded")
		}
		return next(w, r)
	}
}
