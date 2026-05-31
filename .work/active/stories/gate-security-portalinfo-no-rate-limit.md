---
id: gate-security-portalinfo-no-rate-limit
kind: story
stage: implementing
tags: [security, portal, ratelimit]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: security
created: 2026-05-25
updated: 2026-05-31
---

# Public /api/portal/info has no rate-limit

## Severity
Low

## Domain
API Security

## Location
`cmd/portal/main.go:968-971`

## Evidence
```go
// Portal info — fully public, no auth or rate-limiting needed.
// Returns deploy-time config (playground_enabled, landing_variant) for
// anonymous SPA bootstrap before the auth flow completes.
r.Get("/portal/info", apiWrapper.GetPortalInfo)
```

## Remediation direction
The handler is trivial (in-memory struct read, no DB) so flooding it is
mostly a CPU/bandwidth concern, but it's the only public unauthenticated
`/api/*` route without a rate-limiter — `/api/auth/*` all carry per-IP
limits. Add a generous limiter (e.g. 60/min/IP) to close a small
amplification/DoS surface without affecting legitimate SPA bootstrap (one
request per page load). Acceptable to defer permanently if deployed behind
a CDN that caches public GETs.

## Autopilot scope/design (2026-05-31)

Promoted by the gate-drain autopilot run. Implement the remediation direction above as a focused single-stride story, keep edits limited to the named surface, and verify with the targeted test or check that covers the changed file. For older backlog gate items, this run binds the work to `v0.5.0` because the user explicitly requested all gate-related work be scoped, designed, and implemented before release.
