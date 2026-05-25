---
id: gate-security-portalinfo-no-cachecontrol-no-store
kind: story
stage: done
tags: [security, portal, http]
parent: feature-spa-bootstrap-hygiene
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-25
updated: 2026-05-25
---

# /api/portal/info missing Cache-Control: no-store — deploy-mode flip won't propagate

## Severity
Low

## Domain
API Security / Information Disclosure

## Location
`internal/portal/portalinfo/handler.go:27-32`

## Evidence
```go
func (h *Handler) GetPortalInfo(_ context.Context, _ openapi.GetPortalInfoRequestObject) (openapi.GetPortalInfoResponseObject, error) {
    return openapi.GetPortalInfo200JSONResponse{
        PlaygroundEnabled: h.PlaygroundEnabled,
        LandingVariant:    openapi.PortalInfoLandingVariant(h.LandingVariant),
    }, nil
}
```

## Remediation direction
Handler does not emit `Cache-Control: no-store` or equivalent. If a
downstream CDN or browser caches the response (boolean
`playground_enabled`, three-value `landing_variant`), a deploy-time toggle
(operator flips `JAMSESH_PLAYGROUND_ENABLED=false`) would not reach the SPA
for the cache TTL. Mostly an operational correctness concern, but flipping
playground off after abuse is a security operator's lever — caches should
not weaken it. Set `Cache-Control: no-store` (or short max-age with
`must-revalidate`).

## Implementation notes

- `internal/portal/portalinfo/handler.go`: added exported
  `NoCacheMiddleware(next http.Handler) http.Handler` that sets
  `Cache-Control: no-store` before calling next.ServeHTTP. Header is set
  BEFORE the strict-server handler writes status — Go's net/http drops
  headers set after WriteHeader.
- `cmd/portal/main.go`: changed the registration from
  `r.Get("/portal/info", ...)` to
  `r.With(portalinfo.NoCacheMiddleware).Get("/portal/info", ...)` —
  scoped to only this endpoint via chi's `r.With(...)` chain. No other
  route is affected.
- `internal/portal/portalinfo/handler_test.go`: `newTestEnv` now wraps
  the route with `NoCacheMiddleware` so existing tests exercise production
  wiring. New test `TestGetPortalInfo_CacheControlNoStore` asserts
  `resp.Header.Get("Cache-Control") == "no-store"` and that the body is
  still valid JSON.
- "no-store" (not "no-cache") so the response is never stored, even with
  revalidation — needed because deploy-time config flips must reach the
  browser without a stale-cache window.

Verified: `go test ./internal/portal/portalinfo/... -count 1` passes
(all 7+ subtests).

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: `no-store` is correct (stricter than `no-cache`) — appropriate for deploy-time toggle propagation. Middleware sets header before `next.ServeHTTP` to avoid the WriteHeader-already-called silent-drop. Scoped via `r.With(...)` to only the one endpoint. Test harness wires the middleware so production semantics are exercised.
