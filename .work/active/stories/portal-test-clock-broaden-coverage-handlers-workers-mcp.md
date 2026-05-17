---
id: portal-test-clock-broaden-coverage-handlers-workers-mcp
kind: story
stage: implementing
tags: [testing, testability, portal]
parent: portal-test-clock-broaden-coverage
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Inject Clock into the remaining portal handlers, background workers, and MCP endpoint

## Scope

Mechanical broadening of the clock-injection pattern to the rest of
the portal's production `time.Now()` sites. Three groups:

1. **Request-path handlers** — `comments.Service`, `sessions` (invite
   + listing endpoints only — handler.go is in-flight conflict),
   `finalize.Handler`, `storage` archive service.
2. **Background workers** — `events.Log`, `automerger.Applier`.
   Wired for consistency even though they don't sit on the HTTP path
   that `/test/clock-advance` invokes; tests that exercise event/merge
   timestamps may need the offset visible to backend writes.
3. **MCP endpoint** — `mcpendpoint.Endpoint` (verifyToken expiration
   stamp + fork tool's `ForkedAt` payload field). MCP tools share
   bearer auth with the REST API; if a future test exercises an
   expired MCP token, the same advanceable clock must drive that
   path's read.

Wires through `cmd/portal/main.go` via new `*Clock()` accessors on
`testClockProvider`.

## Files

### Modified

- `internal/portal/comments/service.go` — add `Clock` interface,
  `Clock` field on `Service` (struct-literal-initialized in main.go),
  replace 3 `time.Now()` reads. Provide a zero-value-safe path: if
  `Service.Clock == nil`, fall back to real clock at read time.
- `internal/portal/sessions/invites.go` — replace 2 `time.Now().UTC()`
  reads with `h.clock.Now()`.
- `internal/portal/sessions/listing.go` — replace 1 `time.Now().Add(...).UTC()`
  read with `h.clock.Now().Add(...)`.
- `internal/portal/sessions/clock.go` (NEW) — define `Clock` interface,
  `realClock`, and `NewWithClock` constructor that returns a Handler
  with the clock field set. The `Handler` struct field is added in
  this same package via a tiny stub method `WithClock(c Clock) *Handler`
  that returns a new Handler with the field set (avoids touching
  `handler.go`, which is in-flight).
- `internal/portal/finalize/handler.go` — add `Clock` interface,
  `realClock`, `clock` field, and `NewWithClock` constructor.
- `internal/portal/finalize/lock_acquire.go`, `lock_release.go`,
  `lock_patch.go`, `plan.go`, `mark_shipped.go` — replace 5
  `time.Now().UTC()` reads with `h.clock.Now()`.
- `internal/portal/storage/archive.go` — replace `time.Now().UTC()`
  with `s.clock.Now()`. Add `Clock` interface + field on the
  `service` struct + `WithClock` option to `storage.New`.
- `internal/portal/events/log.go` — add `Clock` interface, `Clock`
  field on `Log` (or pass via `New`/`NewWithClock` pair), replace 3
  `time.Now().UTC()` reads.
- `internal/portal/automerger/outcomes.go` — add `Clock` field on
  `Applier`, replace 3 reads (`merger signature When`, conflict
  `now`, conflict-resolve `now`). `NewApplier` keeps its signature;
  add `NewApplierWithClock` variant.
- `internal/portal/mcpendpoint/handler.go` — add `Clock` field on
  `Endpoint`. Replace `time.Now().Add(24h)` in `verifyToken` with
  `e.Clock.Now().Add(24h)` (with nil-check fallback to real clock
  since `Endpoint` is struct-literal-initialized).
- `internal/portal/mcpendpoint/tools.go` — replace `time.Now().UTC().Format(...)`
  in the `fork` tool with `e.clock().Now().UTC().Format(...)` via a
  small helper that reads `e.Clock` with nil-safe fallback.

- `cmd/portal/main.go` — for each of `sessionsHandler`, `finalizeHandler`,
  `commentsSvc`, `storageSvc`, `eventLog`, `mergerApplier`,
  `mcpEndpoint`, switch to the conditional clock-injection pattern.
  The pattern is identical to the v1 magic-link wiring; just repeated
  per handler.
- `cmd/portal/test_clock_advance.go` (modified, `//go:build e2etest`)
  — add one accessor per package:
  `sessionsClock()`, `finalizeClock()`, `commentsClock()`,
  `storageClock()`, `eventsClock()`, `automergerClock()`, `mcpClock()`.
  All return `p.clock`. Per-package interface types are imported and
  the underlying `*testclock.AdvanceableClock` satisfies each.
- `cmd/portal/test_clock_advance_prod.go` (modified, `//go:build !e2etest`)
  — add the same accessors returning `nil` of the appropriate
  per-package `Clock` interface type.

### NOT modified (in-flight / deferred)

- `internal/portal/sessions/handler.go` — locked by
  `portal-validate-writable-scope-at-create-time`. The 2 sites
  (`CreateSession` `created_at` stamp on line 62 and `AbandonSession`
  `ended_at` stamp on line 332) stay on the real clock. Rationale:
  these are write timestamps with no TTL semantics — no currently
  skipped test depends on them. A 1-paragraph follow-on story will
  pick them up once the scope-validation feature lands; the wiring
  is a 2-line replacement (`time.Now().UTC()` → `h.clock.Now()`)
  once the field is reachable.

## Spec

### Per-package Clock interface (uniform shape)

Every modified package gets the same interface declaration at the top
of its main file (or a new `clock.go` file when the main file is
locked):

```go
// Clock is an injectable time source. Mirrors auth.Clock and
// tokens.Clock so a single *testclock.AdvanceableClock satisfies all.
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }
```

### Sessions package — workaround for in-flight `handler.go`

Create `internal/portal/sessions/clock.go`:

```go
package sessions

import "time"

// Clock is an injectable time source. See auth.Clock for the shared
// shape across packages.
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// WithClock returns a copy of h with the supplied clock. Callers that
// want a custom clock construct the handler via `New(...)` and then
// chain `.WithClock(c)`.
func (h *Handler) WithClock(c Clock) *Handler {
    cp := *h
    cp.clock = c
    return &cp
}
```

The `clock Clock` field MUST land on the `Handler` struct in `handler.go`
— but that file is in-flight. **Workaround**: this story explicitly
holds the `Handler.clock` field addition until the
`portal-validate-writable-scope-at-create-time` feature merges, OR
introduces the field via a tiny coordinated edit that minimizes diff
overlap with the scope-validation work. Implementer's call. If
coordination is impossible, defer the entire sessions-package wiring
to the same follow-on story that handles the `handler.go` sites.

The `invites.go` and `listing.go` reads (3 sites) require the field.
If the field can't be added safely, those 3 sites stay on
`time.Now().UTC()` — flag this in the implementation notes and the
follow-on story picks them all up together.

### Comments service — struct-literal initialization

`Service` is initialized in `cmd/portal/main.go` via:

```go
commentsSvc := &comments.Service{Store: dbStore, Log: eventLog}
```

This story extends that to:

```go
var commentsClock comments.Clock
if c := testClk.commentsClock(); c != nil {
    commentsClock = c
}
commentsSvc := &comments.Service{Store: dbStore, Log: eventLog, Clock: commentsClock}
```

Inside `service.go`, helpers read via:

```go
func (s *Service) now() time.Time {
    if s.Clock == nil {
        return time.Now().UTC()
    }
    return s.Clock.Now()
}
```

Same pattern for `mcpendpoint.Endpoint` and `automerger.Applier` and
`events.Log` if they're also struct-literal-initialized in `main.go`.
The nil-safe fallback preserves backwards compatibility with any test
that constructs these structs directly without setting `Clock`.

### Finalize — full handler refactor

`finalize.Handler` already has a clean constructor. Mirror the
magic-link pattern exactly:

```go
type Handler struct {
    store     store.Store
    storage   storage.Service
    events    *events.Log
    tokens    tokens.Service
    portalURL string
    clock     Clock // NEW
}

func New(s store.Store, stor storage.Service, log *events.Log, tok tokens.Service, portalURL string) *Handler {
    return NewWithClock(s, stor, log, tok, portalURL, realClock{})
}

func NewWithClock(s store.Store, stor storage.Service, log *events.Log, tok tokens.Service, portalURL string, clock Clock) *Handler {
    return &Handler{store: s, storage: stor, events: log, tokens: tok, portalURL: portalURL, clock: clock}
}
```

Then `lock_acquire.go`, `lock_release.go`, `lock_patch.go`, `plan.go`,
`mark_shipped.go` each replace their `now := time.Now().UTC()` with
`now := h.clock.Now()`. The `IsLockExpired(lastActivity, now)` and
`LockExpiresAt(lastActivity)` helpers in `lock_check.go` keep their
pure-function shape — `now` is passed in.

### Storage — service struct field

`storage.New(cfg, store)` returns `*service` (lowercase). Add:

```go
type Clock interface { Now() time.Time }
type realClock struct{}
func (realClock) Now() time.Time { return time.Now().UTC() }
```

Add `clock Clock` to `service`. Add an option-or-builder pattern (or
a simple `NewWithClock(cfg, store, clock)` variant). `archive.go`'s
read switches to `s.clock.Now()`.

### Events log

`events.New(s store.Store) *Log`. Add `clock Clock` field. Add
`NewWithClock(s store.Store, clock Clock) *Log`. Replace 3 reads.

### Automerger Applier

`NewApplier(s, log)` keeps shape. Add `NewApplierWithClock(s, log, clock)`.
3 reads replaced. The merger-signature `When` stamp goes through
`clock.Now()` — this affects the merge-commit timestamp visible to git
clients but is intentional: tests that advance the clock and inspect
merge timestamps want to see the advanced time.

### MCP endpoint

`Endpoint` struct gains an exported `Clock Clock` field. The two reads
(`verifyToken` Expiration, `fork` ForkedAt) go through `e.clockNow()`
helper with nil-safe fallback.

### `cmd/portal/main.go` accessor wiring

For each of the 7 handler/service slots, add the same conditional
pattern. Total ~30 lines of repetitive but mechanical wiring. Example
for finalize:

```go
var finalizeHandler *finalize.Handler
if c := testClk.finalizeClock(); c != nil {
    finalizeHandler = finalize.NewWithClock(dbStore, storageSvc, eventLog, tokenSvc, cfg.PortalURL, c)
} else {
    finalizeHandler = finalize.New(dbStore, storageSvc, eventLog, tokenSvc, cfg.PortalURL)
}
```

### `cmd/portal/test_clock_advance.go` / `_prod.go`

Each accessor follows the established pattern. e2etest variant returns
`p.clock`; prod stub returns nil (typed as the per-package interface
so the comparison in main.go is well-defined).

## Acceptance criteria

- [ ] All 5 finalize package reads go through the injected clock.
- [ ] 2 sessions/invites + 1 sessions/listing reads go through the
      injected clock (or are flagged for a follow-on if the
      in-flight handler.go conflict can't be resolved cleanly).
- [ ] 3 comments/service reads go through the clock (with nil-safe
      fallback for struct-literal callers).
- [ ] 1 storage/archive read goes through the clock.
- [ ] 3 events/log reads go through the clock.
- [ ] 3 automerger/outcomes reads go through the clock.
- [ ] 2 mcpendpoint reads go through the clock.
- [ ] `cmd/portal/main.go` constructs every handler via the
      e2etest-gated `*Clock()` accessor pattern.
- [ ] Production builds compile clean with `go build ./...`.
- [ ] e2etest builds compile clean with `go build -tags e2etest ./...`.
- [ ] All existing unit and e2e tests stay green (regression).
- [ ] No new test required at this story level — coverage is exercised
      by future TTL / failure-mode tests; this story is the
      enabling-infrastructure phase.

## Test approach

- Run the full `go test ./...` suite at both build-tag settings.
- Run the `tests/e2e/chaos/runtime_and_clock_test.go > clock_skew_token_expiry`
  subtest (un-skipped in the tokens-wiring story) — confirms the
  shared `*testclock.AdvanceableClock` correctly satisfies every
  per-package `Clock` interface in production code.
- Spot-check that `make test-portal-image` still builds the
  e2etest-tagged image without errors and the existing
  `magic_link_ttl_expiry` test still passes (smoke test that nothing
  regressed in the auth path).

## Notes for the implementer

- Keep the per-package `Clock` interfaces — do NOT collapse to a
  shared interface. Reasons: (a) consistency with the v1 reference;
  (b) avoiding `tokens` package being imported by every handler just
  for the type; (c) the structural-typing trick (one
  `AdvanceableClock` satisfies all) gives the convenience of a shared
  type without the import-graph cost.
- Background-worker rationale: events/log + automerger run on
  goroutines started in `main.go`, not inside HTTP request handlers.
  Advancing `/test/clock-advance` only affects readers that consult
  the `*testclock.AdvanceableClock` after the advance — for workers,
  that's any work item started after the advance. This is the
  intended semantic: tests that advance the clock and then trigger
  a merge should see the advanced timestamp in `merge.succeeded`
  events. Confirmed acceptable.
- The nil-safe `s.now()` helper pattern (used for struct-literal
  callers) is preferred over forcing every caller to migrate to
  `NewWithClock` because some test code initializes these structs
  directly with hand-rolled stores. Don't break those tests.
- If you hit a circular-import issue introducing a per-package
  `Clock`, the fix is almost always to keep the interface inside
  the package it serves and rely on structural typing for the
  cross-package compatibility — same trick the v1 work used.
- Total LoC budget: ~250–300 LoC across all files (mostly
  constructor variants and the field reads). The actual `time.Now()`
  → `clock.Now()` swap is ~20 LoC; everything else is the wiring.

## Production-safety verification

Same checks as the v1 and tokens-wiring stories:

1. `git grep -- 'testclock' cmd/portal/ internal/portal/` returns
   only `//go:build e2etest`-tagged files plus the prod stub.
2. `go build -tags '' ./cmd/portal/` produces a binary; running it
   and hitting `POST /test/clock-advance` returns 404.
3. `go build -tags e2etest ./cmd/portal/` produces a binary; running
   it and POSTing to `/test/clock-advance` advances every wired clock
   simultaneously (the same `*AdvanceableClock` is shared across all
   handlers).
