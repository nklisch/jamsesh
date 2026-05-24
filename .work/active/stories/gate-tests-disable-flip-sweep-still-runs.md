---
id: gate-tests-disable-flip-sweep-still-runs
kind: story
stage: implementing
tags: [testing, portal, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# SELF_HOST.md disable-flip behavior — sweep runs when create is disabled — has no test

## Priority
High

## Spec reference
Item: `feature-epic-ephemeral-playground-reserved-org`

Acceptance criterion: Design decision: "Existing in-flight sessions keep running through their normal idle / hard-cap lifecycles — the destruction sweep continues to fire even when the create endpoint is off."

## Gap type
missing test for valid partition

## Suggested test
```go
func TestWorker_RunsEvenWhenCreateDisabled(t *testing.T) {
    // Worker with Cfg.Enabled=false. Seed an expired session.
    // Run sweep. Assert session destroyed.
}
```
All existing worker tests use default-enabled config.

## Test location (suggested)
`internal/portal/playground/worker_test.go`
