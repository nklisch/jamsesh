---
id: feature-e2e-playground-coverage-chaos
kind: feature
stage: implementing
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

## Mock-boundary plan

Inherited from golden. Toxiproxy fixture already wired into the stack
for the existing `tests/e2e/chaos/network_and_provider_test.go`. No
new fixtures needed for the playground chaos work.

## Taxonomy plan

This feature is the **chaos** layer for the playground. 1 test:
- Destruction-during-in-flight-push graceful degradation

## Anti-tautology guardrails

Inherited from golden — real subprocess + real DB + real filesystem
assertions. Chaos tests additionally must verify that no torn state
results from the injected fault (no orphan repos, no incomplete
tombstones, no stuck sessions).

## Implementation Units

### Unit 1: destruction during in-flight push
**File**: `tests/e2e/chaos/playground_destruction_during_push_test.go`
**Story**: `e2e-audit-playground-destruction-during-push-chaos` (Medium)
**Invariant**: A push that arrives at the portal while the destruction
worker is mid-sweep for the same session either (a) completes cleanly
before destruction OR (b) fails cleanly and destruction completes
without leaving torn state. No half-deleted repo on disk, no orphaned
tombstone, no stuck "ending" session.

Determinism trick: pause the push at the pre-receive hook (or in the
test client between info/refs and git-receive-pack POST), advance the
clock past hard-cap to make the next sweep tick mark the session for
destruction, then release the push and observe the outcome.

## Test integrity

Inherited from golden. Chaos tests are particularly prone to flakes;
mitigate with deterministic orchestration (pause/release rather than
time-based race), generous deadlines, and clear failure messages that
distinguish "test infra flake" from "real torn state".

## Risks (pre-mortem)

- **Orchestration determinism** is the central design challenge.
  Without a way to pause the push deterministically, the race becomes
  timing-dependent and flaky. The implementer should look first for
  an existing pause hook (e.g. a debug knob in pre-receive); if none,
  the simplest mitigation is to inject a known sleep at the test
  client between info/refs and POST git-receive-pack, then advance
  the clock during the sleep window.

## Status

Design complete. Feature advanced to `implementing`. The 1 child story
is also advanced to `implementing`.

## Next

`/agile-workflow:implement-orchestrator feature-e2e-playground-coverage-chaos`
— single-agent run for Unit 1.
