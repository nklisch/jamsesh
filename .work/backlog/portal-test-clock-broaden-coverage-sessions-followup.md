---
id: portal-test-clock-broaden-coverage-sessions-followup
kind: story
stage: drafting
tags: [testing, testability, portal]
parent: null
depends_on: []
release_binding: null
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
