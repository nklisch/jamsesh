---
id: gate-tests-hard-cap-idle-timeout-boundary-equality
kind: story
stage: drafting
tags: [testing, portal, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Exact-boundary tests for `hard_cap_at` and `idle_timeout_at` are missing

## Priority
Medium

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-destruction`

Acceptance criterion: Worker sweeps "sessions where `(now > hard_cap_at OR now > idle_timeout_at)`" — boundary at `now == hard_cap_at` is unspecified but must be deterministic.

## Gap type
missing test for boundary

## Suggested test
```go
func TestWorker_SessionExpiresWhenNowEqualsHardCapAt(t *testing.T) { ... }
func TestWorker_SessionExpiresWhenNowEqualsIdleTimeoutAt(t *testing.T) { ... }
```
Document the chosen behavior (matches SQL strict `>` — boundary excluded) so
future refactors can't silently change it.

## Test location (suggested)
`internal/portal/playground/worker_test.go`
