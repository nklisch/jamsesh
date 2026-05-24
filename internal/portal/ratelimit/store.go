// Package ratelimit provides per-key token-bucket rate limiting for the portal's
// authentication endpoints. It uses golang.org/x/time/rate for the core
// token-bucket algorithm and manages per-key limiters in a sync.Map with periodic
// garbage collection to bound memory.
//
// # Limits (default, documented in cmd/portal/main.go and SELF_HOST.md)
//
//   - POST /api/auth/magic-link/request  — 3/min,  10/hour per IP
//   - POST /api/auth/oauth/start          — 5/min,  20/hour per IP
//   - POST /api/auth/magic-link/exchange  — 10/min  per IP
//   - POST /api/auth/oauth/callback       — 10/min  per IP
//   - POST /api/auth/refresh              — 20/min  per IP
//
// The per-min limit is the sustained rate (token-bucket refill rate).
// The per-hour limit (where specified) is implemented as a second, slower
// limiter stacked in the same entry so both must pass.
//
// # Config knob
//
// Set JAMSESH_AUTH_RATE_LIMIT_ENABLED=false to disable all auth rate limiting.
// Default: true. Useful for single-user self-host scenarios where email-bombing
// is not a concern.
package ratelimit

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// entry holds one or two rate.Limiter values for a single key (client IP or
// IP+email pair). When hourlyLimiter is non-nil, both the minute and hour
// limiters must allow the request.
type entry struct {
	minuteLimiter *rate.Limiter
	hourlyLimiter *rate.Limiter // nil when there is no hourly cap
	lastSeen      time.Time
}

// Store manages per-key rate.Limiter instances. Keys are arbitrary strings
// (typically client IP or "ip:email"). Stale entries are removed by periodic
// GC or on every access older than gcInterval.
//
// Store is safe for concurrent use.
type Store struct {
	mu          sync.Mutex
	entries     map[string]*entry
	minuteLimit rate.Limit  // tokens per second (converted from per-minute)
	minuteBurst int         // burst for minute limiter
	hourlyLimit rate.Limit  // tokens per second (converted from per-hour); 0 if no hourly cap
	hourlyBurst int         // burst for hourly limiter (= per-hour count)
	gcInterval  time.Duration
	lastGC      time.Time
	ttl         time.Duration // idle TTL before an entry is GC'd
	clock       Clock
}

// Config describes the limits for a Store.
type Config struct {
	// PerMinute is the sustained request rate allowed per key per minute.
	// The token bucket refills at PerMinute/60 tokens per second with a
	// burst of PerMinute.
	PerMinute int

	// PerHour, if > 0, adds a second slower limiter that caps requests at this
	// rate per hour. When 0, no hourly cap is applied.
	PerHour int
}

// NewStore creates a Store with the given per-key limits using the real system clock.
func NewStore(cfg Config) *Store {
	return NewStoreWithClock(cfg, realClock{})
}

// NewStoreWithClock creates a Store with the given per-key limits and an injectable
// clock. Used by unit tests to exercise GC and token-bucket refill without
// real wall-clock waits.
func NewStoreWithClock(cfg Config, clock Clock) *Store {
	s := &Store{
		entries:    make(map[string]*entry),
		gcInterval: 5 * time.Minute,
		ttl:        1 * time.Hour,
		lastGC:     clock.Now(),
		clock:      clock,
	}

	// Convert per-minute → rate.Limit (tokens/second).
	s.minuteLimit = rate.Limit(float64(cfg.PerMinute) / 60.0)
	s.minuteBurst = cfg.PerMinute

	if cfg.PerHour > 0 {
		s.hourlyLimit = rate.Limit(float64(cfg.PerHour) / 3600.0)
		s.hourlyBurst = cfg.PerHour
	}

	return s
}

// Allow reports whether a request from key should be allowed. If not allowed
// it also returns the recommended Retry-After duration.
//
// Allow is the hot path: it acquires the store mutex only long enough to
// look up or allocate an entry, then calls Allow on the limiter(s).
func (s *Store) Allow(key string) (allowed bool, retryAfter time.Duration) {
	e := s.getOrCreate(key)

	now := s.clock.Now()

	// Check minute limiter.
	r := e.minuteLimiter.ReserveN(now, 1)
	if !r.OK() {
		// Burst exceeded — limiter doesn't allow even with infinite wait.
		return false, 60 * time.Second
	}
	if d := r.DelayFrom(now); d > 0 {
		r.Cancel() // return the token
		return false, d
	}

	// Check hourly limiter when present.
	if e.hourlyLimiter != nil {
		rh := e.hourlyLimiter.ReserveN(now, 1)
		if !rh.OK() {
			r.Cancel()
			return false, 3600 * time.Second
		}
		if d := rh.DelayFrom(now); d > 0 {
			rh.Cancel()
			r.Cancel()
			return false, d
		}
	}

	return true, 0
}

// getOrCreate retrieves the entry for key, creating it if absent. Also
// triggers GC when gcInterval has elapsed.
func (s *Store) getOrCreate(key string) *entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Lazy GC.
	now := s.clock.Now()
	if now.Sub(s.lastGC) >= s.gcInterval {
		s.gc(now)
		s.lastGC = now
	}

	e, ok := s.entries[key]
	if !ok {
		e = &entry{
			minuteLimiter: rate.NewLimiter(s.minuteLimit, s.minuteBurst),
		}
		if s.hourlyLimit > 0 {
			e.hourlyLimiter = rate.NewLimiter(s.hourlyLimit, s.hourlyBurst)
		}
		s.entries[key] = e
	}
	e.lastSeen = now
	return e
}

// gc removes entries that haven't been seen in ttl. Must be called with s.mu held.
func (s *Store) gc(now time.Time) {
	for k, e := range s.entries {
		if now.Sub(e.lastSeen) > s.ttl {
			delete(s.entries, k)
		}
	}
}

// Size returns the current number of tracked keys. Useful for monitoring.
func (s *Store) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// Middleware returns a chi-compatible http.Handler middleware that enforces
// this store's limits keyed by client IP (r.RemoteAddr host part, or the
// value left by chi's RealIP middleware).
//
// When the limit is exceeded the middleware writes a 429 JSON response with a
// Retry-After header and does NOT call next. When enabled is false, the
// middleware is a no-op pass-through (honoring JAMSESH_AUTH_RATE_LIMIT_ENABLED).
func (s *Store) Middleware(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientIP(r)
			allowed, retryAfter := s.Allow(key)
			if !allowed {
				writeTooManyRequests(w, retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
