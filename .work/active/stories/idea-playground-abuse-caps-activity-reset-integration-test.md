---
id: idea-playground-abuse-caps-activity-reset-integration-test
kind: story
stage: done
tags: [portal, playground, testing]
parent: feature-playground-hardening
depends_on: [gate-security-githttp-receivepack-wallclock-not-injected]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-25
---

# Activity-reset integration test (push → wait → push → wait past original timeout)

## Context

Surfaced during review of
`story-epic-ephemeral-playground-session-lifecycle-abuse-caps`.

The parent feature's Risks section explicitly flagged "Activity-reset miss =
sessions die early" as the primary correctness risk for this story and
recommended:

> integration test that pushes a commit, waits 25 min (simulated via injected
> clock), pushes another commit, then waits past the original 30m mark —
> verifies session still alive because second push reset the timer.

The shipped story does NOT include this test. The three activity-reset
call-sites (`githttp/receive_pack.go`, `comments/service.go`,
`sessions/handler.go`) are only exercised by tests that don't pass through
the playground `org_id` branch, so the reset code paths are effectively
untested. A regression here is silent: sessions die early but the symptom
is "user complains the session vanished" — no test failure.

## Scope

Add an integration test (likely under `tests/e2e/` or a per-package
integration test that uses the existing `stores(t)` harness) that:

1. Creates a playground session via the REST API
2. Pushes a commit to the session
3. Advances the test clock by 25 minutes (within idle timeout)
4. Verifies `last_substantive_activity_at` and `idle_timeout_at` were
   updated by the push
5. Performs a substantive action (push, comment, or finalize-attempt)
6. Advances the clock past the original idle threshold (e.g. 35 min from
   start)
7. Verifies the session is still alive (destruction worker has NOT picked
   it up)
8. Repeats for each of the three substantive event types — push, comment,
   finalize — so all three call-sites have at least one positive coverage.

## Acceptance criteria

- All three substantive-event types (push / comment / finalize) have a
  test asserting they bump `last_substantive_activity_at` and
  `idle_timeout_at`.
- At least one test exercises the full destruction-worker interaction:
  substantive event resets the timer, then sweep doesn't destroy.
- Tests run under both SQLite and Postgres if using the per-package
  harness.

## Notes

Lightweight alternative: unit tests in each package that invoke the
substantive-event entry point with a mock store and assert
`ResetSessionIdleTimer` was called with the expected params. Less
end-to-end coverage but catches "we forgot to wire the reset" regressions
cheaply.

## Implementation notes

- **Push path**: Added `TestPostReceive_PlaygroundActivityReset_SecondPushExtendsBeyondOriginalDeadline`
  in `internal/portal/githttp/receive_pack_test.go`. Sequence:
  1. Session created at T0 with idle_timeout = 30m.
  2. Inject clock at T0+25m; push #1 → idle_timeout_at = T0+55m.
  3. Bump clock to T0+35m; push #2 → idle_timeout_at = T0+65m.
  4. Assert `ListExpiredPlaygroundSessions(Now: T0+35m)` does NOT include
     the session — the activity-reset extended the deadline past the
     original T0+30m boundary. This is the precise abuse-cap invariant.
  Uses the `fixedGitHTTPClock` introduced in B1.
- **Comment path**: Pre-existing coverage in
  `internal/portal/comments/service_test.go` already pins `ResetSessionIdleTimer`
  is called on `Service.Create` for playground sessions (see `TestService_Create_PlaygroundSession_ResetsIdleTimer`
  family of tests around line 934). No new test needed.
- **Finalize path**: Pre-existing coverage in
  `internal/portal/sessions/handler_test.go` —
  `TestFinalizeSession_PlaygroundSession_ResetsIdleTimer` already asserts
  both timer fields advance after a finalize call. No new test needed.

All three substantive-activity surfaces (push / comment / finalize) now
have at least one test pinning the activity-reset behaviour; the push path
gets the full integration sequence per the original story scope.

Verified: `go test ./internal/portal/githttp/... -count 1 -run SecondPushExtendsBeyondOriginalDeadline` passes.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Test exercises the full abuse-cap invariant — push within window extends deadline past original threshold; sweep at original threshold doesn't pick it up. Uses real bare repo + real git subprocess + injected clock. Comment and finalize paths reuse pre-existing coverage (correct — don't duplicate). The (idle_timeout, hard_cap) separation is preserved by checking only the idle expiration list, which is the correct abuse-cap target.
