---
id: gate-security-rate-limit-auth-endpoints
kind: story
stage: implementing
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
