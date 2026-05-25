---
id: gate-tests-portalinfo-method-not-allowed-cors
kind: story
stage: review
tags: [testing, portal, api]
parent: null
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-25
updated: 2026-05-25
---

# /api/portal/info — method-not-allowed and CORS preflight not asserted

## Priority
Medium

## Spec reference
Item: `story-portal-visitor-entry-pages-info-endpoint`
Acceptance: "The endpoint is reachable without an Authorization header."
Adversarial: this endpoint is the SPA's bootstrap surface; a
POST/PUT/DELETE should yield 405 not 200, and CORS preflight handling
(if the portal is hit cross-origin during dev) is unspecified.

## Gap type
Adversarial — boundary / invalid HTTP method.

## Location
`internal/portal/portalinfo/handler_test.go:201-307` covers all
`(playground_enabled, landing_variant)` combinations plus no-auth, but
no test for wrong-method or OPTIONS preflight.

## Suggested test
```go
func TestGetPortalInfo_WrongMethodReturns405(t *testing.T) {
  // POST to /api/portal/info, assert 405 Method Not Allowed.
}
```

## Test location (suggested)
`internal/portal/portalinfo/handler_test.go`. CORS depends on whether
the portal sets CORS headers anywhere — file as `[documentation]` gap
if absent.

## Implementation notes

- `TestGetPortalInfo_WrongMethodReturns405` walks POST/PUT/DELETE/PATCH
  against `/api/portal/info` via the existing `testEnv` server and
  asserts every response is HTTP 405 Method Not Allowed. This relies on
  chi's default behaviour for routes registered via `.Get()` — the
  router auto-emits 405 for other verbs on the same path.
- `TestGetPortalInfo_OptionsPreflight` documents the current
  CORS-preflight behaviour at `/api/portal/info`. The portal binary
  installs no CORS middleware on this route, so the test pins what is
  true today: OPTIONS returns a non-5xx response and
  `Access-Control-Allow-Origin` is empty. The test comment explicitly
  flags that this should be flipped to assert positive CORS headers if
  the portal ever gains a CORS middleware on the public bootstrap
  surface — surfacing the change here prevents silent CORS regressions.
- No production code changed. The 405 contract was already implicitly
  provided by chi's router; the test makes it explicit and regression-proof.
- Verification: `go test ./internal/portal/portalinfo/... -count 1` →
  full file passes including both new tests.

