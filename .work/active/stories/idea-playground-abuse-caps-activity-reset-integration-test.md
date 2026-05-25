---
id: idea-playground-abuse-caps-activity-reset-integration-test
kind: story
stage: implementing
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
