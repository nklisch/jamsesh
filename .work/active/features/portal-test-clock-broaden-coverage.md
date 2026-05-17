---
id: portal-test-clock-broaden-coverage
kind: feature
stage: drafting
tags: [testing, testability, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Broaden test-clock injection to all production time.Now sites

## Idea

`portal-test-clock-advance-endpoint` (v1) threads an injectable clock
through `internal/portal/auth/magic_link.go` only — the call sites
required to un-skip `magic_link_ttl_expiry`. A full audit of
`internal/portal/` surfaced ~25 other production `time.Now()` reads
that remain on the real wall clock:

- `internal/portal/accounts/orgs.go` (org creation, slug retry)
- `internal/portal/accounts/handlers.go` (org-invite create)
- `internal/portal/auth/oauth.go` (state TTL check)
- `internal/portal/auth/provision.go` (account/org provisioning)
- `internal/portal/auth/slug.go` (rand seed — likely fine to leave)
- `internal/portal/automerger/outcomes.go` (merge timestamps, ×3)
- `internal/portal/comments/service.go` (×3)
- `internal/portal/events/log.go` (×3)
- `internal/portal/finalize/lock_acquire.go`,
  `finalize/lock_release.go`, `finalize/lock_patch.go`,
  `finalize/plan.go`, `finalize/mark_shipped.go`
- `internal/portal/logging/logging.go` (access-log latency — fine to
  leave on real clock)
- `internal/portal/mcpendpoint/handler.go`,
  `mcpendpoint/tools.go`
- `internal/portal/oauth/state.go`
- `internal/portal/sessions/handler.go`, `sessions/invites.go`,
  `sessions/listing.go`
- `internal/portal/storage/archive.go`

`internal/portal/tokens` already has its own injectable `Clock` but is
not yet wired to the runtime `/test/clock-advance` knob.

Unlocked tests:
- `clock_skew_token_expiry` in
  `tests/e2e/chaos/runtime_and_clock_test.go` (currently
  documented-skip; needs the `tokens.Service` to be clock-advanceable
  at runtime).
- 30-minute idle-TTL path on finalize locks (alluded to in
  `tests/e2e/failure/interrupted_ops_test.go >
  finalize_lock_release_and_reacquire` comment).
- Org-invite expiry, OAuth-state expiry, comment max-age window —
  none currently have skipped tests but adding the injectability
  unlocks future failure-mode subtests cheaply.

## Approach sketch

Mirror the v1 pattern:
1. Each package gets its own local `Clock` interface (auth-package
   pattern) OR they share `tokens.Clock` if the import graph allows.
2. Constructors get a `WithClock` variant; the default constructor
   uses `realClock{}`.
3. The e2etest-tagged `cmd/portal/test_clock_advance.go` injects the
   shared `*testclock.AdvanceableClock` into every handler that
   accepts a clock.

Sizing: ~12 handlers / services, ~25 call sites, ~12 constructor
edits in `cmd/portal/main.go`. Probably 2-3 stories rather than one.

## Why deferred

No active test is blocked on these in v1. Acceptance criteria for
`portal-test-clock-advance-endpoint` names only the magic-link TTL
test. Broadening eagerly would multiply the v1 surface area by an
order of magnitude with no immediate gate to justify it.

Promote via `/agile-workflow:scope` when:
- a second clock-sensitive e2e test gets blocked on this, OR
- the chaos `clock_skew_token_expiry` scenario is prioritized to
  un-skip.
