---
id: portal-test-clock-advance-endpoint
kind: feature
stage: done
tags: [testing, e2e-test, testability]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Portal: test-only clock-advance endpoint

## Context

The magic-link TTL is 15 minutes (`magicLinkTTL` in
`internal/portal/auth/magic_link.go`). End-to-end testing of the
`auth.expired_token` path (`ExchangeMagicLink` returns 401 when
`now.After(row.ExpiresAt)`) requires either:

- a real 15-minute wait (unacceptable in CI), or
- the ability to advance the portal's clock.

The e2e spec `tests/e2e/failure/interrupted_ops_test.go` has a
`magic_link_ttl_expiry` subtest that is currently skipped with a
reference to this feature.

## Design

### Architectural choice

**Build-tag-gated `e2etest` binary that injects an advanceable clock into
the magic-link handler, exposing a `POST /test/clock-advance` mutator.**

Compared briefly:

1. **(chosen) Interface + build-tag-gated wiring file.** A `Clock`
   interface in `internal/portal/auth`, threaded into `MagicLinkHandler`
   via an existing constructor parameter. A `//go:build e2etest` file in
   `cmd/portal/` swaps the default real-clock construction for an
   `AdvanceableClock` and mounts the `/test/clock-advance` route. The
   production build (`-tags ''`) sees only the production constructor
   path and the empty `MountTest` hook.

2. `func() time.Time` parameter. Same idea, but breaks the established
   in-repo idiom: `internal/portal/tokens` already defines
   `Clock interface { Now() time.Time }`, `realClock`, and `NewWithClock`.
   Mirroring that pattern keeps consistency.

3. Package-level `var nowFn = time.Now` with a build-tag override file.
   Cheapest, but introduces hidden global state — opposite direction
   from how `tokens` already does it, and the build-tag override would
   live next to the production file, making the override less
   visible to reviewers.

The chosen approach reuses the exact `tokens.Clock` shape the codebase
already validates against (`internal/portal/tokens/service_impl.go`),
keeps the test indirection visible in `cmd/portal/`, and guarantees the
production handler constructor is unchanged.

### Production-safety guarantee

The endpoint and the advanceable-clock package are gated by `//go:build
e2etest`. Production builds (`go build -tags ''` and the default
`Dockerfile` at the repo root) never compile that source. Verification
steps baked into the design:

1. `cmd/portal/test_clock_advance.go` carries `//go:build e2etest`.
2. `cmd/portal/test_clock_advance_prod.go` carries `//go:build !e2etest`
   and provides a no-op `mountTestEndpoints` stub. Exactly one of the
   two files compiles per build, by mutual exclusion.
3. A new package `internal/portal/testclock` carries `//go:build
   e2etest` on every file. Production builds cannot import it; doing so
   from non-tagged code is a compile error, which is the desired
   structural guard.
4. `make test-portal-image` (used by Testcontainers fixtures) is
   updated to pass `-tags e2etest` to `go build`. The release Docker
   image (root `Dockerfile`, CI workflow) keeps `-tags ''`.
5. A CI smoke check verifies the symbol absence: `go build ./cmd/portal
   && nm portal | grep -q clock_advance && exit 1 || exit 0` — or
   equivalently, attempt to `go build -tags ''` and then `curl -sf
   $URL/test/clock-advance` (must 404).

The mount-hook plumbing is itself benign — `router.Deps.MountTest` is a
nilable `func(chi.Router)`, identical in shape to the existing
`MountAPI`/`MountGit` hooks. When the test file doesn't compile, the
hook is never populated and the route subtree is never registered.

### Endpoint contract

```
POST /test/clock-advance
Content-Type: application/json

Request body:
  {"advance_seconds": 900}

Response 200 OK:
  Content-Type: application/json
  {
    "now": "2026-05-17T12:15:00.123456789Z",
    "offset_seconds": 900
  }

Response 400 Bad Request:
  - missing or non-integer "advance_seconds"
  - negative "advance_seconds" (only forward advance supported)
  - body is not valid JSON
```

- `now` is the post-advance wall-clock time as the clock would report it.
- `offset_seconds` is the cumulative offset applied since portal start,
  useful when a test wants to chain advances.
- The endpoint is not authenticated. Trust comes exclusively from the
  build-tag gate.

### Scope of `time.Now()` audit (v1)

Only `internal/portal/auth/magic_link.go` is wired through the new
clock in v1. The acceptance criterion is the magic-link TTL test — no
other test is currently blocked on clock advancement.

A full repository audit found 25+ production `time.Now()` sites
(accounts orgs, sessions, comments, finalize, events log, automerger
outcomes, oauth state, mcpendpoint, storage archive, sessions invites,
auth provision). Threading the clock through all of them is a
multi-feature refactor with no immediate acceptance gate. Tracked as
follow-on: `portal-test-clock-broaden-coverage` in `.work/backlog/`.

`internal/portal/tokens` already has `Clock` + `NewWithClock` but is
not wired through the `/test/clock-advance` endpoint in v1. The chaos
`clock_skew_token_expiry` scenario (deferred-skip in
`epic-e2e-tests-chaos-runtime-and-clock`) calls this out and remains
deferred until the broaden-coverage follow-on lands.

## Implementation Units

### Unit 1: `auth.Clock` and `MagicLinkHandler` wiring

**File**: `internal/portal/auth/magic_link.go` (modified)
**Story**: `portal-test-clock-advance-endpoint-clock-abstraction`

```go
// Clock is an injectable time source. The default realClock calls
// time.Now().UTC(); tests inject a fakeClock to simulate expiry.
// Mirrors internal/portal/tokens.Clock by design — same idiom across
// the auth package family.
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type MagicLinkHandler struct {
    store     store.Store
    tokensSvc tokens.Service
    sender    senders.Sender
    portalURL string
    clock     Clock
}

// NewMagicLinkHandler constructs a MagicLinkHandler with the real
// system clock. Production callers use this.
func NewMagicLinkHandler(
    s store.Store,
    tokensSvc tokens.Service,
    sender senders.Sender,
    portalURL string,
) *MagicLinkHandler {
    return NewMagicLinkHandlerWithClock(s, tokensSvc, sender, portalURL, realClock{})
}

// NewMagicLinkHandlerWithClock constructs a MagicLinkHandler with the
// supplied clock. Used by unit tests (fakeClock) and by the
// e2etest-tagged binary (AdvanceableClock from internal/portal/testclock).
func NewMagicLinkHandlerWithClock(
    s store.Store,
    tokensSvc tokens.Service,
    sender senders.Sender,
    portalURL string,
    clock Clock,
) *MagicLinkHandler { ... }
```

Both `RequestMagicLink` and `ExchangeMagicLink` swap their two
`time.Now().UTC()` calls for `h.clock.Now()`. No other behavior change.

**Acceptance Criteria**:
- [ ] `NewMagicLinkHandler` signature unchanged from production callers'
      perspective; `cmd/portal/main.go` and `magic_link_test.go` still
      compile without edits.
- [ ] `NewMagicLinkHandlerWithClock` exposed for test wiring.
- [ ] All four `time.Now()` reads in magic_link.go go through `h.clock`.
- [ ] One new unit test in `magic_link_test.go` proves expiry: inject
      a fakeClock, issue a token, advance past 15 min, exchange returns
      401 `auth.expired_token`.
- [ ] `go build -tags ''` and `go test ./internal/portal/auth/...` green.

---

### Unit 2: Build-tag-gated test endpoint

**Files**:
- `internal/portal/testclock/clock.go` (NEW, `//go:build e2etest`)
- `internal/portal/testclock/handler.go` (NEW, `//go:build e2etest`)
- `cmd/portal/test_clock_advance.go` (NEW, `//go:build e2etest`)
- `cmd/portal/test_clock_advance_prod.go` (NEW, `//go:build !e2etest`)
- `internal/portal/router/router.go` (modified) — add `MountTest func(chi.Router)` hook
- `cmd/portal/main.go` (modified) — call `mountTestEndpoints(...)` and pass result to `router.Deps`

**Story**: `portal-test-clock-advance-endpoint-test-endpoint`

```go
// internal/portal/testclock/clock.go  (//go:build e2etest)
package testclock

import (
    "sync"
    "time"
)

// AdvanceableClock satisfies auth.Clock (and tokens.Clock — same shape).
// Now() returns the real wall clock plus an accumulated offset.
type AdvanceableClock struct {
    mu     sync.Mutex
    offset time.Duration
}

func New() *AdvanceableClock { return &AdvanceableClock{} }

func (c *AdvanceableClock) Now() time.Time {
    c.mu.Lock()
    defer c.mu.Unlock()
    return time.Now().UTC().Add(c.offset)
}

func (c *AdvanceableClock) Advance(d time.Duration) time.Duration {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.offset += d
    return c.offset
}

func (c *AdvanceableClock) Offset() time.Duration { ... }
```

```go
// internal/portal/testclock/handler.go  (//go:build e2etest)
package testclock

// Handler returns an http.Handler implementing POST /clock-advance.
// Decodes {"advance_seconds": <int>}, advances the clock, responds
// with the new wall clock and the cumulative offset.
func Handler(clock *AdvanceableClock) http.Handler { ... }
```

```go
// cmd/portal/test_clock_advance.go  (//go:build e2etest)
package main

import (
    "github.com/go-chi/chi/v5"
    "jamsesh/internal/portal/auth"
    "jamsesh/internal/portal/testclock"
)

// testClockProvider holds the singleton AdvanceableClock used by
// e2etest-tagged builds. Constructed once at server start; injected
// into every handler that needs time.Now indirection (v1: magic-link
// only).
type testClockProvider struct {
    clock *testclock.AdvanceableClock
}

func newTestClockProvider() *testClockProvider {
    return &testClockProvider{clock: testclock.New()}
}

// magicLinkClock returns the clock to inject into the magic-link
// handler. Implements auth.Clock.
func (p *testClockProvider) magicLinkClock() auth.Clock { return p.clock }

// mountTestEndpoints registers POST /test/clock-advance.
func (p *testClockProvider) mountTestEndpoints(r chi.Router) {
    r.Mount("/test", testclock.RouteMount(p.clock))
}
```

```go
// cmd/portal/test_clock_advance_prod.go  (//go:build !e2etest)
package main

import (
    "github.com/go-chi/chi/v5"
    "jamsesh/internal/portal/auth"
)

// testClockProvider is a no-op stub in production builds.
type testClockProvider struct{}

func newTestClockProvider() *testClockProvider { return &testClockProvider{} }

// magicLinkClock returns nil. main.go interprets nil as "use the
// real clock" and calls auth.NewMagicLinkHandler (not the WithClock
// variant).
func (p *testClockProvider) magicLinkClock() auth.Clock { return nil }

// mountTestEndpoints is intentionally nil in production builds; the
// router never registers /test/*.
func (p *testClockProvider) mountTestEndpoints(r chi.Router) {}
```

```go
// cmd/portal/main.go (changes)
testClk := newTestClockProvider()

var magicLinkHandler *auth.MagicLinkHandler
if c := testClk.magicLinkClock(); c != nil {
    magicLinkHandler = auth.NewMagicLinkHandlerWithClock(
        dbStore, tokenSvc, emailSender, cfg.PortalURL, c)
} else {
    magicLinkHandler = auth.NewMagicLinkHandler(
        dbStore, tokenSvc, emailSender, cfg.PortalURL)
}

handler := router.New(router.Deps{
    ...
    MountTest: testClk.mountTestEndpoints,
})
```

```go
// internal/portal/router/router.go (modified)
type Deps struct {
    ...
    MountTest func(chi.Router) // nil in production builds
}

func New(d Deps) http.Handler {
    ...
    if d.MountTest != nil {
        r.Route("/test", d.MountTest)
    }
    ...
}
```

**Implementation Notes**:
- The `testclock` package mirrors `tokens.Clock` (same `Now() time.Time`
  signature), so a single `AdvanceableClock` instance can satisfy both
  the `auth.Clock` interface and the `tokens.Clock` interface when
  follow-on work broadens coverage.
- `Advance` accepts only positive durations; negative values are
  rejected at the HTTP layer (400). Allowing rewind would create
  cross-test contamination since the clock is process-global.
- The endpoint is intentionally non-idempotent (each call adds to the
  cumulative offset). Tests that need a known offset can pre-record
  the response's `offset_seconds` and reason about deltas.
- Concurrency: `sync.Mutex` around offset reads and writes. Magic-link
  exchange and `/test/clock-advance` can race; the mutex serializes
  them safely.

**Acceptance Criteria**:
- [ ] `go build -tags '' ./cmd/portal` produces a binary with no
      `/test/*` route — verified by an integration test that boots the
      production binary, requests `POST /test/clock-advance`, and
      asserts 404.
- [ ] `go build -tags e2etest ./cmd/portal` produces a binary where
      `POST /test/clock-advance` with `{"advance_seconds": 900}` returns
      200 with `now` and `offset_seconds` fields.
- [ ] `internal/portal/testclock/` has `//go:build e2etest` on every
      file. Attempting to import the package from non-tagged code is a
      compile error.
- [ ] `make test-portal-image` builds the e2e Docker image with
      `-tags e2etest`. The release `Dockerfile` and CI release workflow
      keep `-tags ''`.
- [ ] Negative `advance_seconds`, missing field, non-integer field all
      return 400 with a clear message.

---

### Unit 3: Un-skip `magic_link_ttl_expiry`

**Files**:
- `tests/e2e/failure/interrupted_ops_test.go` (modified) — un-skip + body
- `tests/e2e/fixtures/portal/portal.go` (modified, if needed) — wire `-tags e2etest` portal image option
- `tests/e2e/fixtures/portal/clockadvance.go` (NEW) — thin helper that POSTs `/test/clock-advance`

**Story**: `portal-test-clock-advance-endpoint-e2e-unskip`

```go
// tests/e2e/fixtures/portal/clockadvance.go
func (p *Portal) AdvanceClock(ctx context.Context, t *testing.T, d time.Duration) {
    t.Helper()
    body := fmt.Sprintf(`{"advance_seconds": %d}`, int64(d.Seconds()))
    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        p.URL+"/test/clock-advance", strings.NewReader(body))
    ...
    if resp.StatusCode != 200 {
        t.Fatalf("portal.AdvanceClock: status %d (the portal image must be built with -tags e2etest; see Makefile target test-portal-image)", resp.StatusCode)
    }
}
```

```go
// magic_link_ttl_expiry subtest (un-skipped)
t.Run("magic_link_ttl_expiry", func(t *testing.T) {
    email := "ttl-expiry@example.com"
    // Step 1: request a magic-link. (Don't sign in — we want the raw token.)
    authflow.RequestMagicLink(ctx, t, p, email)
    token := authflow.ExtractMagicLinkToken(ctx, t, mh, email)

    // Step 2: advance the portal's clock past the 15-minute TTL.
    p.AdvanceClock(ctx, t, 16*time.Minute)

    // Step 3: attempt exchange — must fail with auth.expired_token.
    url := p.URL + "/api/auth/magic-link/exchange"
    body := []byte(fmt.Sprintf(`{"token":%q}`, token))
    resp := rawPostExpect(ctx, t, url, body, "", http.StatusUnauthorized, "auth.expired_token")
    _ = resp
})
```

**Implementation Notes**:
- If `authflow.RequestMagicLink` and `authflow.ExtractMagicLinkToken`
  helpers don't already exist (the existing `SignInViaMagicLink`
  conflates request + exchange), add them by factoring the existing
  helper. Both helpers should live in `tests/e2e/fixtures/authflow/`.
- The Testcontainers fixture builds the portal image via
  `make test-portal-image`; this story does NOT need to change the
  fixture's container start path. It just needs the image to have been
  built with `-tags e2etest`. The Makefile change for that lives in
  Unit 2.

**Acceptance Criteria**:
- [ ] `magic_link_ttl_expiry` subtest is no longer `t.Skip`'d.
- [ ] Running `cd tests/e2e && go test ./failure/ -run
      TestInterruptedOps/magic_link_ttl_expiry -v` is green.
- [ ] The subtest asserts on the `auth.expired_token` error code (not
      the human-readable message).
- [ ] If `tests/e2e/fixtures/authflow` lacks the split helpers, they
      are added in this story (small change, scope creep accepted to
      keep the story self-contained).

---

## Implementation Order

1. `portal-test-clock-advance-endpoint-clock-abstraction` — pure
   refactor in `internal/portal/auth/`. No build tags introduced. Lands
   first because Unit 2 depends on its constructor surface.
2. `portal-test-clock-advance-endpoint-test-endpoint` — adds the
   build-tag-gated package + wiring. Depends on Unit 1.
3. `portal-test-clock-advance-endpoint-e2e-unskip` — un-skips the e2e
   test. Depends on Unit 2 (the endpoint must exist and the e2e
   Docker image must be built with `-tags e2etest`).

## Testing

- Unit (Unit 1): `internal/portal/auth/magic_link_test.go` gains an
  expiry test driven by `fakeClock` (no build tag — the
  `MagicLinkHandler.clock` field is package-private but
  `NewMagicLinkHandlerWithClock` is exported).
- Unit (Unit 2): `cmd/portal` integration test with `-tags ''` boots
  the binary, POSTs `/test/clock-advance`, asserts 404. With
  `-tags e2etest`, asserts the happy path + bad inputs. Best landed as
  `cmd/portal/test_clock_advance_test.go` carrying its own build tags.
- E2E (Unit 3): the interrupted-ops subtest re-running.

## Risks

- **Risk: tests/e2e portal fixture defaults to a production image.**
  If a developer runs `cd tests/e2e && go test ./failure/...` without
  first running `make test-portal-image` (which now passes
  `-tags e2etest`), the `AdvanceClock` helper will get 404 and the
  subtest fails with a confusing message. **Mitigation**: the helper's
  failure message names the Makefile target explicitly. The Makefile
  target is the canonical source of the build flag.

- **Risk: `nil` `Clock` interface confusion.** If
  `testClk.magicLinkClock()` returns a typed nil rather than an
  untyped nil, the `if c != nil` check would silently pass and the
  handler would panic on the first `c.Now()` call. **Mitigation**:
  both production-build and e2etest-build return concrete-typed values
  (`nil` of `auth.Clock` interface type from the production stub),
  and a `auth.Clock(nil) == nil` test in `cmd/portal` confirms the
  comparison works. Alternative: production stub returns a sentinel
  `realClock` — but that defeats the assertion that production code
  doesn't depend on anything in `internal/portal/testclock`.

- **Risk: process-wide clock bleeds across e2e subtests.** Once
  advanced, the clock can't go back. Each Testcontainers portal
  instance is fresh (terminate-on-cleanup), so process-wide is also
  test-wide. **Mitigation**: documented in `testclock` package
  comments; future tests must reset by booting a fresh portal
  container, not by attempting to subtract from the offset.

- **Risk: build-tag drift across CI.** A future contributor adds a
  new build target without remembering the production-tag invariant.
  **Mitigation**: the smoke check in Unit 2 acceptance criteria runs
  on every release build; the release workflow's `go build` line is
  `-tags ''` and the production Dockerfile invokes that line.

## Child stories

1. `portal-test-clock-advance-endpoint-clock-abstraction` — introduce
   `auth.Clock` interface and `NewMagicLinkHandlerWithClock`. Thread
   through the magic-link handler. Unit-tested with a fakeClock.
2. `portal-test-clock-advance-endpoint-test-endpoint` — build-tag-gated
   `testclock` package, `cmd/portal` wiring (tagged + stub),
   `router.Deps.MountTest`, Makefile/Dockerfile updates so the e2e
   image is `-tags e2etest`. Depends on (1).
3. `portal-test-clock-advance-endpoint-e2e-unskip` — un-skip the
   `magic_link_ttl_expiry` subtest; add the `Portal.AdvanceClock`
   fixture method and the split authflow helpers. Depends on (2).

## Design decisions

- **Build tag: `e2etest`** — generic enough to cover future e2e-only
  endpoints (e.g., seeded test data, in-memory event injection) without
  another tag proliferation. The alternative `testclock` was rejected
  as too narrow; this feature won't be the last e2e-only mutator.
  Verified no existing `//go:build` tag in the repository conflicts
  (only `//go:build tools` in `tools/tools.go`).
- **Clock abstraction: `Clock` interface (auth-local), parallel to
  `tokens.Clock`** — keeps the existing in-repo idiom. The interface
  is locally defined per package (auth has its own, tokens has its
  own) rather than centralized, because Go interfaces are satisfied
  structurally and centralizing would create an awkward import
  pyramid. An `AdvanceableClock` in `internal/portal/testclock` (one
  concrete type) satisfies both.
- **Endpoint at `/test/clock-advance`** — flat namespace under
  `/test/*`, room for future test-only endpoints without renaming. The
  router mounts `/test/` only when `MountTest` is non-nil, matching
  the existing nilable-hook pattern in `router.Deps`.
- **Endpoint unauthenticated** — under the build tag, the endpoint
  exists; trust comes from the build tag, not from an in-binary
  credential. Authenticating it would imply some test-side credential
  setup, which buys nothing because the existence of the endpoint
  already implies a test build. The `Dockerfile.e2e` image and the
  Testcontainers fixture are the trust boundary.
- **Scope: magic-link only in v1** — acceptance criteria names only
  the magic-link TTL test as a deliverable. The 25+ other production
  `time.Now()` sites are tracked in a follow-on backlog item
  (`portal-test-clock-broaden-coverage`). The chaos
  `clock_skew_token_expiry` scenario waits on the follow-on.
- **Cumulative offset, forward-only** — `Advance(d)` adds to a
  monotonic offset; the clock can't be rewound. Process-global by
  design, matching the lifetime of a Testcontainers portal instance.
- **Parentless feature** — the natural parent epic
  (`epic-e2e-tests-failure-mode`) is already `stage: done`. We don't
  retroactively bind to a closed epic. This feature stands alone, and
  any later parent (e.g., a `portal-testability` epic) can claim it
  via a future scope action.

## Acceptance criteria

- [x] `POST /test/clock-advance` advances the portal's clock by the
      requested number of seconds (build-tag-gated, never compiled in
      production builds)
- [x] `magic_link_ttl_expiry` subtest in
      `tests/e2e/failure/interrupted_ops_test.go` is un-skipped and
      green
- [x] No clock-injection code appears in production build output
      (`go build -tags ''` must not include the test endpoint)

## Review

**Verdict**: Approve.

**Summary**: All three child stories landed exactly as designed. The
feature ships a build-tag-gated `POST /test/clock-advance` endpoint
backed by a process-global `AdvanceableClock`, with three independent
production-safety layers (compilation gate, wiring gate, CI guardrail
test) all intact and verified end-to-end.

**Production-safety verification**:
- `go build ./...` (no tags) — clean. The production binary contains
  no `testclock` references; the only non-tagged mention is a doc
  comment in `internal/portal/auth/magic_link.go`.
- `go build -tags e2etest ./...` — clean.
- `TestProductionBuild_HasNoTestEndpoint` (no tags) — PASS. The
  production-build router 404s `POST /test/clock-advance`.
- `TestE2EBuild_TestEndpointMounted` (-tags e2etest) — PASS. The
  e2etest-tagged router exposes the endpoint with the documented
  `{"now", "offset_seconds"}` shape.
- All four files in `internal/portal/testclock/` carry
  `//go:build e2etest`. `grep -rn 'testclock' cmd/ internal/` shows
  the only non-tagged reference is a doc comment.
- `cmd/portal/test_clock_advance.go` carries `//go:build e2etest`;
  `cmd/portal/test_clock_advance_prod.go` carries `//go:build !e2etest`.
  Exactly one compiles per build.
- `Makefile` `test-portal-image` target passes `-tags e2etest`; the
  release `Dockerfile` and CI workflow stay on `-tags ''`.

**Test verification**:
- `go test ./internal/portal/auth/...` — green. The new
  `TestExchangeMagicLink_ExpiredToken_Returns401WithExpiredCode`
  exercises the injected-clock path.
- `go test -tags e2etest -race ./internal/portal/testclock/...` —
  green. Cumulative, zero-noop, negative-rejected, and
  concurrent-safe paths all covered.
- e2e: trusting the e2e-unskip story's reported run of
  `TestInterruptedOps/magic_link_ttl_expiry` as PASS after
  `make test-portal-image`.

**Acceptance criteria**: all three boxes checked.

**Design fidelity**: one well-justified deviation in
`portal-test-clock-advance-endpoint-test-endpoint` — `RouteMount`
became path-agnostic (chi doesn't rewrite `r.URL.Path` before
delegating to a stdlib `ServeMux`, so the nested-mux design would
have 404'd). The cleaner fix registers the route at the chi layer
and is documented in the story's implementation notes.

**Findings**: 0 blockers, 0 important, 0 nits. Parked: none.
