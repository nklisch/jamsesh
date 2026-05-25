---
id: gate-tests-state-readtoken-per-session-sweep-callsite-coverage
kind: story
stage: done
tags: [testing, plugin, refactor]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# `feature-state-readtoken-per-session-sweep` call-site coverage uncertain

## Priority
Medium

## Spec reference
Item: `feature-state-readtoken-per-session-sweep`

Acceptance criterion: Inferred from feature naming: extract a helper + sweep call-sites. Without confirmation, the existing tests in `cmd/jamsesh/state/{state,migrate}_test.go` cover migration but call-site coverage of the new helper is unverified.

## Gap type
missing test for valid partition (worth confirming the helper + each call-site has a test)

## Suggested test
For each call-site (status, fork, etc.), verify the migrated `ReadToken`
helper is invoked.

## Test location (suggested)
`cmd/jamsesh/state/`, `cmd/jamsesh/sessioncmd/`

## Implementation notes

Added five tests to `cmd/jamsesh/state/state_test.go` covering the per-session
token isolation contract established by `feature-state-readtoken-per-session-sweep`:

- `TestReadCurrentBearer_SessionIsolation` — core invariant: token written for
  session-A is not returned when session-B is requested.
- `TestReadCurrentBearer_BoundCallsite_PrefersPerSession` — models the fork/new
  bound-session callsite pattern (`ReadCurrentBearer(sessID)`): per-session file
  shadows the legacy token.
- `TestReadCurrentBearer_PreBindingCallsite_UsesLegacy` — models the join/new
  pre-binding callsite pattern (`ReadCurrentBearer("")`): always reads legacy token.
- `TestReadSessionToken_StatusCallsite_PerSessionDirect` — models the status
  callsite (`ReadSessionToken(sessID)`): absent before write, present after.
- `TestReadCurrentBearer_MultiSession_EachIsolated` — table-driven sweep over
  three concurrent sessions, verifying no cross-contamination.

All tests are in the same `package state` as the code-under-test, using the
existing `withPluginData(t, dir)` helper. No new test infrastructure was needed.
The session-level callsites in `sessioncmd/` are integration-tested via
`status_test.go` (which already asserts per-session token reads); the unit tests
here directly target the helper function that was swept across those callsites.

## Review notes

Approve. Five tests cleanly partition the callsite contracts (status, bound,
pre-binding, multi-session). Each writes real files via the real helper and
asserts isolation by content equality / inequality. All pass.
