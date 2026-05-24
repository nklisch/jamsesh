---
id: feature-e2e-playground-coverage-chaos
kind: feature
stage: drafting
tags: [testing, e2e-test, playground, portal, chaos]
parent: epic-e2e-playground-coverage
depends_on: [feature-e2e-playground-coverage-golden]
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Chaos e2e tests for the playground subsystem

## Brief

Chaos coverage for the v0.4.0 playground subsystem — specifically,
verifying that the destruction worker degrades gracefully when its sweep
fires during in-flight pushes. The e2e-test-design skill's rule is that
chaos tests verify graceful degradation of already-tested paths, so this
feature depends on `feature-e2e-playground-coverage-golden` for the
solo-create-push baseline.

Existing chaos tests at `tests/e2e/chaos/` (8 tests covering
pod-kill / clock-skew / network-partition / object-storage chaos for
authenticated-org sessions) supply the pattern. Toxiproxy is already
wired into the stack; no new fixture needed.

## Child stories

This feature has 1 child story, carried over from the
`e2e-test-design --audit` run:

1. `e2e-audit-playground-destruction-during-push-chaos` (Medium) —
   start a long-running push to a playground session that is about to
   hit hard-cap; advance the clock past hard-cap mid-push; assert the
   push either completes or fails cleanly (no torn state) and that
   destruction eventually fires without corrupting the bare-repo
   directory or leaving orphan tombstones

## Possible follow-up chaos scenarios (not in this epic)

The audit was scoped narrowly; richer chaos coverage is fair game for
later cycles but explicitly out of scope here:

- Portal restart mid-anonymous-session (does the bearer survive across
  restarts? destruction worker resumes its sweep cadence?)
- Postgres pod kill while a `playground` session is mid-flight
- Toxiproxy-injected latency between binary and portal during `jamsesh
  jam --playground`

Park these as backlog items if they surface during implementation.

## Design status

Audit-supplied sketch is the seed. e2e-test-design's job is to lock the
clock-advance + Toxiproxy interaction (how does the test orchestrate
"clock past hard-cap" and "push still in flight" deterministically?
candidate: pause the push at the pre-receive hook via a hook-side
sleep injected through a portal-config knob, advance clock, release
the push).

## Next

`/agile-workflow:e2e-test-design feature-e2e-playground-coverage-chaos`
once golden is at `stage: implementing` or beyond.
