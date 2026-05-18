---
id: gate-security-rate-limit-auth-endpoints
kind: story
stage: review
tags: [security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# No rate limiting on authentication or invite-creation endpoints

## Severity
High

## Domain
API Security

## Location
`cmd/portal/main.go:689-697`, `internal/portal/router/router.go` (no
Throttle middleware anywhere)

## Evidence
```go
r.Group(func(r chi.Router) {
    r.Post("/auth/refresh", apiWrapper.RefreshToken)
    r.Post("/auth/magic-link/request", apiWrapper.RequestMagicLink)
    r.Post("/auth/magic-link/exchange", apiWrapper.ExchangeMagicLink)
    r.Post("/auth/oauth/start", apiWrapper.StartOAuth)
    r.Post("/auth/oauth/callback", apiWrapper.OauthCallback)
})
```

`grep -rn 'ratelim\|RateLimit\|throttle\|Throttle'` returns zero hits
under `internal/portal/`. `RequestMagicLink` and `CreateOrgInvite` send
unbounded transactional email triggered solely by request body — both
can be weaponised for email-bombing and provider-quota exhaustion
(which then degrades all real users). `ExchangeMagicLink` and
`RefreshToken` can also be brute-forced without any backoff.

## Remediation direction
Add `chimw.Throttle` or a token-bucket limiter scoped per-IP for
`/auth/*` and per-account (after Bearer) for invite/comment/event
endpoints. At minimum cap `/auth/magic-link/request` and
`/auth/oauth/start` at single-digit RPM per IP+email pair, and add
Retry-After 429 responses.

## Implementation notes

### Package
`internal/portal/ratelimit/` — new package. Two files:
- `store.go` — `Store` (per-key token-bucket manager) + `Middleware(enabled bool)` factory
- `response.go` — `writeTooManyRequests` (429 envelope) + `clientIP` helper

### Algorithm
Uses `golang.org/x/time/rate` (already in go.mod as indirect; no new deps added).
Each `Store` holds a `sync.Mutex`-protected `map[string]*entry` where each entry has
a per-minute `rate.Limiter` and an optional per-hour `rate.Limiter`. Both must pass
for the request to be allowed. Stale entries (idle > 1h) are GC'd every 5 minutes.

### Limits (per IP)
| Endpoint                          | Per-minute | Per-hour |
|-----------------------------------|-----------|---------|
| `POST /auth/magic-link/request`   | 3         | 10      |
| `POST /auth/oauth/start`          | 5         | 20      |
| `POST /auth/magic-link/exchange`  | 10        | —       |
| `POST /auth/oauth/callback`       | 10        | —       |
| `POST /auth/refresh`              | 20        | —       |

### Keying strategy
Per-IP only (using `r.RemoteAddr` host after chi's `RealIP` middleware has
already replaced it when `TrustProxyHeaders=true`). Per-IP+email keying was
considered but requires body re-reading inside middleware which adds fragility
to the strict-handler pipeline. Per-IP is sufficient for the declared threat
model (email-bombing, brute-force).

### Wiring
Each endpoint gets its own `Store` instantiated in `MountAPI` in `cmd/portal/main.go`,
mounted via `r.With(limiter).Post(...)` on the chi `/api` route group — NOT in the
global `r.Use(...)` chain, preserving separation from `SecurityHeaders` middleware.

### 429 response shape
```json
{"error": "rate_limited", "message": "Too many requests. Retry in N seconds."}
```
With `Retry-After: N` header. Matches the httperr envelope convention (same `error`/`message` keys).

### Config knob
`JAMSESH_AUTH_RATE_LIMIT_ENABLED=false` (default: `true`) via `config.Config.AuthRateLimitEnabled`.
When false, `Middleware(false)` returns a pass-through no-op. Useful for single-user
self-host where email-bombing is not a concern. Also set this in integration test
environments that fire many auth requests in quick succession
(e.g. `JAMSESH_AUTH_RATE_LIMIT_ENABLED=false` in test environment setup).
