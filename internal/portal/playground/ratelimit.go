package playground

import (
	"net/http"

	"jamsesh/internal/portal/ratelimit"
)

// NewCreateRateLimiter returns a *ratelimit.Store configured for the
// playground session-create endpoint. The store uses per-IP token-bucket
// limiting based on cfg.CreatePerIPHour.
//
// The existing ratelimit package uses PerMinute as its unit. We convert the
// per-hour cap to per-minute (rounded up, minimum 1) so the token-bucket
// burst equals the per-minute count. A caller that fires all 3 creates in
// the first minute is blocked for the remainder of the hour by the hourly
// limiter; the per-minute limiter prevents sub-minute bursts beyond 1/min.
func NewCreateRateLimiter(cfg Config) *ratelimit.Store {
	perHour := cfg.CreatePerIPHour
	if perHour < 1 {
		perHour = 1
	}
	perMinute := (perHour + 59) / 60
	if perMinute < 1 {
		perMinute = 1
	}
	return ratelimit.NewStore(ratelimit.Config{
		PerMinute: perMinute,
		PerHour:   perHour,
	})
}

// CreateRateLimitMiddleware returns a chi-compatible middleware that enforces
// the store's per-IP limits. When the limit is exceeded it writes a 429
// response with a Retry-After header and does NOT call next.
//
// When enabled is false the middleware is a transparent pass-through — the
// rate-limit store is not consulted. Pass cfg.Enabled so that playground
// routes are not rate-limited when playground is disabled globally (the route
// returns 503 before any quota is consumed anyway, but the pass-through keeps
// the control path clean).
func CreateRateLimitMiddleware(store *ratelimit.Store, enabled bool) func(http.Handler) http.Handler {
	return store.Middleware(enabled)
}
