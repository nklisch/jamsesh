---
id: portal-test-clock-broaden-coverage-sessions-followup
kind: story
stage: done
tags: [testing, testability, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Inject Clock into the sessions package (deferred follow-on)

## Context

The parent story
`portal-test-clock-broaden-coverage-handlers-workers-mcp` (commit
`e54ea0b`) wired the e2etest `AdvanceableClock` into the remaining 17
portal `time.Now()` sites across `comments`, `finalize`, `storage`,
`events`, `automerger`, and `mcpendpoint`. The sessions package was
deferred because adding the `Handler.clock` field requires editing
`internal/portal/sessions/handler.go`, which was locked by the
in-flight `portal-validate-writable-scope-at-create-time` story at the
time of design.

That blocking story has since landed (commit `87835cc`,
stage:done), so `handler.go` is now unlocked. This follow-on picks
up the deferred 5 sites + 3 reads.

## Scope

Mirror the v1 clock-injection pattern for the sessions package:

1. Add `internal/portal/sessions/clock.go` with the standard `Clock`
   interface and `realClock` type — same shape as the other packages
   (see `comments/service.go`, `finalize/handler.go`, etc).
2. Add a `clock Clock` field to `Handler` in `handler.go`.
3. Add a `NewWithClock(...)` constructor variant; have `New(...)`
   delegate to it with `realClock{}`.
4. Replace the 5 `time.Now().UTC()` sites:
   - `handler.go:73` — `CreateSession` `created_at` stamp.
   - `handler.go:349` — `AbandonSession` `ended_at` stamp.
   - `invites.go:94` — `InviteToSession` `CreatedAt`/`ExpiresAt`.
   - `invites.go:175` — `AcceptSessionInvite` `AcceptedAt`/`JoinedAt`.
   - `listing.go:68` — pagination `before` cursor.
5. Wire `cmd/portal/main.go` to use the new constructor under the
   established `if c := testClk.sessionsClock(); c != nil` pattern.
6. Add `sessionsClock()` accessor to both
   `cmd/portal/test_clock_advance.go` (returns `p.clock`) and
   `cmd/portal/test_clock_advance_prod.go` (returns `nil`).
7. Add a `clock_test.go` mirroring the pattern from the other
   packages — one positive test that `NewWithClock` reads from the
   injected clock, and a parity smoke test that `New` produces the
   same handler shape as `NewWithClock(realClock{})`.

## Acceptance criteria

- [ ] All 5 sessions package `time.Now().UTC()` reads go through
      `h.clock.Now()`.
- [ ] `sessionsClock()` accessor added to both test-clock files.
- [ ] `cmd/portal/main.go` uses the standard conditional wiring.
- [ ] Production builds compile clean with `go build ./...`.
- [ ] e2etest builds compile clean with `go build -tags e2etest ./...`.
- [ ] All existing unit and e2e tests stay green.
- [ ] `POST /test/clock-advance` advances the sessions handler
      clock alongside every other wired clock.

## Estimated size

~50-80 LoC. Mechanical replication of the v1 pattern. Single stride.

## Implementation notes

Mirrored the v1 clock-injection pattern from `internal/portal/finalize`
into `internal/portal/sessions`.

### Files touched

- `internal/portal/sessions/clock.go` — new. Defines `Clock` interface
  + `realClock` (returns `time.Now().UTC()`). Same shape as
  `finalize/handler.go` / `comments/service.go`.
- `internal/portal/sessions/handler.go` — added `clock Clock` field to
  `Handler`. `New(...)` now delegates to `NewWithClock(..., realClock{})`.
  Dropped the now-unused `time` import.
- `internal/portal/sessions/invites.go` — no constructor change; the 2
  `time.Now().UTC()` sites swapped for `h.clock.Now()`. `time` import
  retained for `sessionInviteTTL = 7 * 24 * time.Hour`.
- `internal/portal/sessions/listing.go` — pagination cursor "before"
  swapped to `h.clock.Now().Add(time.Second)`. `time` import retained.
- `internal/portal/sessions/clock_test.go` — new. Two tests:
  `TestHandler_CreateSessionUsesInjectedClock` proves
  `NewWithClock(fakeClock)` controls the `CreatedAt` stamp, and
  `TestHandler_NewVsNewWithClock_ProductionPathClean` proves the
  default `New(...)` path still produces realClock stamps in
  `[before, after]`.
- `cmd/portal/main.go` — sessions handler construction now branches
  `sessions.NewWithClock(..., c)` vs `sessions.New(...)` on
  `testClk.sessionsClock() != nil`, mirroring the finalize wiring.
  Removed the deferral NOTE comment that referenced this very story.
- `cmd/portal/test_clock_advance.go` — added `sessionsClock()`
  accessor returning `p.clock` (typed as `sessions.Clock`).
- `cmd/portal/test_clock_advance_prod.go` — added `sessionsClock()`
  stub returning typed nil.

### Sites wrapped (post-edit line numbers)

1. `handler.go:80` — `CreateSession` `created_at` / `joined_at` stamp.
2. `handler.go:356` — `AbandonSession` `ended_at` stamp.
3. `invites.go:94` — `InviteToSession` `CreatedAt` + `ExpiresAt`.
4. `invites.go:175` — `AcceptSessionInvite` `AcceptedAt` + `JoinedAt`.
5. `listing.go:68` — `ListSessions` pagination cursor "before".

### Verification

- `go build ./...` — clean.
- `go build -tags e2etest ./...` — clean.
- `go vet ./internal/portal/...` — clean.
- `go test ./internal/portal/sessions/...` — pass.
- `go test ./internal/portal/...` — all pass.
- `go test ./cmd/portal/ -run TestProductionBuild_HasNoTestEndpoint` —
  pass (production binary still rejects `POST /test/clock-advance` with
  404).
- Two new sessions tests pass (`TestHandler_CreateSessionUsesInjectedClock`,
  `TestHandler_NewVsNewWithClock_ProductionPathClean`).

## Review

**Verdict: Approve.** Implementation matches the v1 clock-injection
pattern exactly. All 5 production `time.Now().UTC()` sites are wrapped
through `h.clock.Now()`; test files retain real-time fixtures as
expected. `clock.go` mirrors `finalize`/`comments` shape; `New(...)`
delegates to `NewWithClock(..., realClock{})`. `cmd/portal/main.go`
follows the established `if c := testClk.sessionsClock(); c != nil`
branch with the deferral NOTE comment removed.

### Shared-clock invariant

The new `sessionsClock()` accessor in
`cmd/portal/test_clock_advance.go` returns `p.clock` — same field as
the existing 9 accessors (magicLink, tokens, accounts, comments,
finalize, storage, events, automerger, mcp). Single process-global
`*testclock.AdvanceableClock` — `POST /test/clock-advance` moves all
10 wired clocks atomically. Production stub returns typed `nil
sessions.Clock` — no typed-nil trap at the `c != nil` comparison.

### Verification

- `go build ./...` — clean.
- `go build -tags e2etest ./...` — clean.
- `go vet ./internal/portal/...` — clean.
- `go test ./internal/portal/...` — all 24 packages pass.
- `go test ./internal/portal/sessions/...` — pass (new tests included).
- `TestProductionBuild_HasNoTestEndpoint` — pass.
- Two new tests (`TestHandler_CreateSessionUsesInjectedClock`,
  `TestHandler_NewVsNewWithClock_ProductionPathClean`) — pass.

### Findings

- Blockers: 0
- Important: 0
- Nits: 0
