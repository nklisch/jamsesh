---
id: portal-test-clock-advance-endpoint-test-endpoint
kind: story
stage: implementing
tags: [testing, testability]
parent: portal-test-clock-advance-endpoint
depends_on: [portal-test-clock-advance-endpoint-clock-abstraction]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Build-tag-gated /test/clock-advance endpoint

## Scope

Land the `e2etest`-tagged code path that exposes
`POST /test/clock-advance`. Production builds (`-tags ''`) must not
compile any of the new code paths and must not register the route.

## Files

- `internal/portal/testclock/clock.go` (NEW, `//go:build e2etest`)
- `internal/portal/testclock/handler.go` (NEW, `//go:build e2etest`)
- `internal/portal/testclock/clock_test.go` (NEW, `//go:build e2etest`)
- `internal/portal/testclock/handler_test.go` (NEW, `//go:build e2etest`)
- `internal/portal/router/router.go` (modified — add `MountTest`)
- `cmd/portal/test_clock_advance.go` (NEW, `//go:build e2etest`)
- `cmd/portal/test_clock_advance_prod.go` (NEW, `//go:build !e2etest`)
- `cmd/portal/main.go` (modified — call `newTestClockProvider`,
  conditionally pick `NewMagicLinkHandlerWithClock`, pass
  `MountTest`)
- `cmd/portal/test_clock_advance_e2e_test.go` (NEW, `//go:build e2etest`
  — happy/sad path)
- `cmd/portal/test_clock_advance_prod_test.go` (NEW, `//go:build !e2etest`
  — 404 assertion)
- `Makefile` (modified — `test-portal-image` target builds with
  `-tags e2etest`)

## Spec

### `internal/portal/testclock/clock.go`

```go
//go:build e2etest

// Package testclock provides an advanceable clock for e2e tests.
// This package is build-tag-gated to e2etest — production builds
// (go build -tags '') cannot import it. See feature
// portal-test-clock-advance-endpoint.
package testclock

import (
    "sync"
    "time"
)

// AdvanceableClock is a process-global clock that returns
// time.Now().UTC() plus a cumulative offset. Safe for concurrent use.
//
// The clock only advances forward; rewind would create cross-test
// contamination because state is process-global by design.
type AdvanceableClock struct {
    mu     sync.Mutex
    offset time.Duration
}

// New returns a fresh AdvanceableClock with zero offset.
func New() *AdvanceableClock { return &AdvanceableClock{} }

// Now returns the current wall time plus the accumulated offset, in UTC.
func (c *AdvanceableClock) Now() time.Time {
    c.mu.Lock()
    defer c.mu.Unlock()
    return time.Now().UTC().Add(c.offset)
}

// Advance adds d to the cumulative offset and returns the new
// cumulative offset. Returns the previous offset unchanged if d <= 0.
func (c *AdvanceableClock) Advance(d time.Duration) time.Duration {
    c.mu.Lock()
    defer c.mu.Unlock()
    if d > 0 {
        c.offset += d
    }
    return c.offset
}

// Offset returns the current cumulative offset.
func (c *AdvanceableClock) Offset() time.Duration {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.offset
}
```

### `internal/portal/testclock/handler.go`

```go
//go:build e2etest

package testclock

import (
    "encoding/json"
    "net/http"
    "time"
)

// RouteMount returns an http.Handler that serves POST /clock-advance.
// Mount it under /test/ in the portal router so the public surface
// becomes POST /test/clock-advance.
func RouteMount(clock *AdvanceableClock) http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("POST /clock-advance", advanceHandler(clock))
    return mux
}

type advanceRequest struct {
    AdvanceSeconds int64 `json:"advance_seconds"`
}

type advanceResponse struct {
    Now           string `json:"now"`
    OffsetSeconds int64  `json:"offset_seconds"`
}

func advanceHandler(clock *AdvanceableClock) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req advanceRequest
        dec := json.NewDecoder(r.Body)
        dec.DisallowUnknownFields()
        if err := dec.Decode(&req); err != nil {
            http.Error(w, "invalid JSON body", http.StatusBadRequest)
            return
        }
        if req.AdvanceSeconds < 0 {
            http.Error(w, "advance_seconds must be >= 0", http.StatusBadRequest)
            return
        }
        // advance_seconds == 0 is allowed (no-op read of current clock).
        offset := clock.Advance(time.Duration(req.AdvanceSeconds) * time.Second)
        resp := advanceResponse{
            Now:           clock.Now().Format(time.RFC3339Nano),
            OffsetSeconds: int64(offset / time.Second),
        }
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(resp)
    }
}
```

### `internal/portal/router/router.go` (modified)

Add to `Deps`:

```go
type Deps struct {
    ...
    // MountTest is a nilable hook for test-only routes under /test/*.
    // Populated only by the e2etest-tagged binary; production builds
    // leave it nil and the /test subtree is never registered.
    MountTest func(chi.Router)
    ...
}
```

In `New(d Deps)`, after the `MountMCP` block, add:

```go
if d.MountTest != nil {
    r.Route("/test", d.MountTest)
}
```

### `cmd/portal/test_clock_advance.go`

```go
//go:build e2etest

package main

import (
    "github.com/go-chi/chi/v5"

    "jamsesh/internal/portal/auth"
    "jamsesh/internal/portal/testclock"
)

// testClockProvider holds the advanceable clock used by e2etest-tagged
// builds. It satisfies both the auth.Clock interface and (in future
// follow-on work) tokens.Clock — same Now() time.Time shape.
type testClockProvider struct {
    clock *testclock.AdvanceableClock
}

func newTestClockProvider() *testClockProvider {
    return &testClockProvider{clock: testclock.New()}
}

func (p *testClockProvider) magicLinkClock() auth.Clock { return p.clock }

func (p *testClockProvider) mountTestEndpoints(r chi.Router) {
    r.Mount("/", testclock.RouteMount(p.clock))
}
```

### `cmd/portal/test_clock_advance_prod.go`

```go
//go:build !e2etest

package main

import (
    "github.com/go-chi/chi/v5"

    "jamsesh/internal/portal/auth"
)

// testClockProvider is a no-op stub in production builds. The
// presence of two files with mutually-exclusive build tags ensures
// exactly one definition compiles.
type testClockProvider struct{}

func newTestClockProvider() *testClockProvider { return &testClockProvider{} }

// magicLinkClock returns nil — main.go interprets nil as "use the
// real clock" and falls back to auth.NewMagicLinkHandler.
func (p *testClockProvider) magicLinkClock() auth.Clock { return nil }

// mountTestEndpoints is a no-op stub. main.go passes a nil hook to
// router.Deps.MountTest, so the /test/* subtree is never mounted.
func (p *testClockProvider) mountTestEndpoints(_ chi.Router) {}
```

### `cmd/portal/main.go` (modified)

Replace the existing magic-link construction block:

```go
// Build the magic-link handler. In e2etest builds, inject the
// advanceable clock; in production builds, the provider returns nil
// and the real-clock constructor is used.
testClk := newTestClockProvider()

var magicLinkHandler *auth.MagicLinkHandler
if c := testClk.magicLinkClock(); c != nil {
    magicLinkHandler = auth.NewMagicLinkHandlerWithClock(
        dbStore, tokenSvc, emailSender, cfg.PortalURL, c)
} else {
    magicLinkHandler = auth.NewMagicLinkHandler(
        dbStore, tokenSvc, emailSender, cfg.PortalURL)
}
```

Update the `router.New(router.Deps{...})` call to pass:

```go
MountTest: testClk.mountTestEndpoints,
```

Production builds: `testClk.mountTestEndpoints` is the no-op stub. We
COULD pass nil instead, but passing a callable function value lets
the production-build stub remain a function and keeps the type identical
across builds — clearer to readers.

Actually — to keep router behavior identical, the production stub
should NOT be passed. Instead:

```go
deps := router.Deps{ ... }
if mt := testClk.mountTestEndpoints; mt != nil {
    // Both builds compile this branch; in production the function value
    // is the no-op stub. Passing it means the router will call r.Route("/test", stub),
    // which mounts an empty subtree. That's a behavior change.
    // Prefer to leave MountTest nil in production.
}
```

**Resolution**: have the e2etest-tagged provider expose
`mountTestEndpoints` and the production-tagged provider expose nil
explicitly via a method:

```go
// e2etest build
func (p *testClockProvider) mountTestEndpointsHook() func(chi.Router) {
    return p.mountTestEndpoints
}

// !e2etest build
func (p *testClockProvider) mountTestEndpointsHook() func(chi.Router) {
    return nil
}
```

And in main.go:

```go
MountTest: testClk.mountTestEndpointsHook(),
```

In production builds, `MountTest` is nil and `router.New` skips the
`/test` `r.Route` call entirely. This is the cleanest separation.

### `Makefile` (modified)

Change the `test-portal-image` target:

```makefile
test-portal-image: frontend-build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags e2etest -o portal-linux-amd64 ./cmd/portal
	docker build -f Dockerfile.e2e --build-arg BINARY=portal --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -t jamsesh/portal:e2e .
	@rm -f portal-linux-amd64
```

The release build (which uses the root `Dockerfile`, not `Dockerfile.e2e`,
and whose `go build` line lives in `.github/workflows/release.yml`)
is NOT touched; it stays on `-tags ''`.

### Tests

`internal/portal/testclock/clock_test.go` and `handler_test.go`:
unit-test `Advance`, concurrency safety, the 400 paths, and the 200
response shape. Standard `httptest` patterns.

`cmd/portal/test_clock_advance_e2e_test.go` (build-tag `e2etest`):
spin up the full router via `router.New`, POST
`/test/clock-advance`, assert 200 and JSON shape.

`cmd/portal/test_clock_advance_prod_test.go` (build-tag `!e2etest`):
spin up the same router but with the production-build `testClk`
plumbing, POST `/test/clock-advance`, assert 404.

## Acceptance criteria

- [ ] `go build -tags '' ./cmd/portal` succeeds. The resulting binary
      responds 404 to `POST /test/clock-advance` (verified by
      `cmd/portal/test_clock_advance_prod_test.go`).
- [ ] `go build -tags e2etest ./cmd/portal` succeeds. The resulting
      binary responds 200 with `{"now": "...", "offset_seconds": <n>}`
      to a well-formed `POST /test/clock-advance` (verified by
      `cmd/portal/test_clock_advance_e2e_test.go`).
- [ ] `internal/portal/testclock/*.go` all carry `//go:build e2etest`.
      A spot-check `grep -L '//go:build e2etest'
      internal/portal/testclock/*.go` returns empty.
- [ ] Attempting to import `jamsesh/internal/portal/testclock` from
      non-tagged code fails to compile (verified by running
      `go vet ./...` — production-tagged code never references the
      package).
- [ ] Negative `advance_seconds` → 400. Missing field defaults to 0 and
      the call is treated as a no-op read (returns current offset).
      Non-integer or unknown JSON field → 400 (via
      `DisallowUnknownFields`).
- [ ] Concurrent `Advance` + `Now` are race-free under
      `go test -race ./internal/portal/testclock/...`.
- [ ] `make test-portal-image` builds an image whose `POST
      /test/clock-advance` works (smoke-checkable via
      `docker run --rm -p 8443:8443 jamsesh/portal:e2e &` then `curl
      -d '{"advance_seconds":60}' localhost:8443/test/clock-advance`).
- [ ] `router.Deps.MountTest` is documented as test-only in
      `router/router.go` doc comments.

## Production-safety verification

This is the load-bearing story for production safety. Three layers:

1. **Compilation layer**: all `internal/portal/testclock/*.go` and
   `cmd/portal/test_clock_advance.go` carry `//go:build e2etest`. The
   production-mirror file `cmd/portal/test_clock_advance_prod.go`
   carries `//go:build !e2etest`. Exactly one of the two compiles per
   build by mutual exclusion. `go build -tags ''` does not see
   `internal/portal/testclock` at all.
2. **Wiring layer**: in `cmd/portal/main.go`, the test-clock hook is
   accessed via the build-tagged `testClk.mountTestEndpointsHook()`
   method. The production variant returns nil. `router.New` skips the
   `/test` subtree when `MountTest` is nil. No `/test/*` route ever
   exists in the production binary's routing table.
3. **Test layer**: `cmd/portal/test_clock_advance_prod_test.go` runs
   on `-tags ''` and asserts the production binary returns 404 for
   `POST /test/clock-advance`. This test is the CI-level guardrail
   against build-tag drift.

Additionally, the existing release Dockerfile (`Dockerfile`, used by
the GitHub Actions release workflow) builds with no tags. Only
`Dockerfile.e2e` (used by `make test-portal-image` for Testcontainers
fixtures) gets the `-tags e2etest` binary.

## Notes for the implementer

- The `MountTest` hook position in `router.New` matters. Mount it
  AFTER `MountMCP` and BEFORE `MountUI` (the SPA catch-all). The /test
  route must take precedence over the SPA's catch-all.
- The route mux inside `testclock.RouteMount` uses Go 1.22+ method-on-
  pattern syntax (`"POST /clock-advance"`). Confirm Go version
  (`go.mod` should have `go 1.22` or later) — if not, use
  `mux.HandleFunc("/clock-advance", ...)` with a method check inside.
- The `chi` `r.Mount("/", testclock.RouteMount(p.clock))` mounts the
  RouteMount handler at the parent's prefix (`/test`), so the final
  path is `/test/clock-advance`. Verify the prefix arithmetic with
  the integration test.
- Do NOT add `/test/*` to the existing `/api` Bearer-auth middleware
  group. The test endpoint is unauthenticated by design.
- The Makefile `test-portal-image` change is the trigger for
  re-running the e2e suite. CI workflow that runs e2e must invoke
  `make test-portal-image` before `make test-e2e-go` (it already
  does — `make test-e2e-go` is downstream of fixtures, and the
  fixture itself documents the make target as a precondition).
