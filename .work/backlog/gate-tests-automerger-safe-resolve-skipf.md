---
id: gate-tests-automerger-safe-resolve-skipf
kind: story
stage: backlog
tags: [testing, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# `outcomes_test.go` SafeAutoResolve subtest contains conditional skip if heuristic doesn't fire

## Priority
Low

## Spec reference
Item: `epic-auto-merger-outcomes-apply`
Acceptance criterion: safe-auto-resolve adds `Auto-Resolved:<heuristic>`
trailer.

## Gap type
test-integrity.
`internal/portal/automerger/outcomes_test.go:279` reads
`t.Skipf("did not get SafeAutoResolve (got %s); this test requires a whitespace conflict", result.Kind)`.
The skip masks a real test-fixture issue: if upstream `Merge` ever
changes classification, this subtest silently no-ops instead of failing.

## Suggested test
Convert the `t.Skipf` to `t.Fatalf`: if the test author deliberately
constructed a whitespace-only conflict, an unexpected `Kind` is a
regression, not a precondition miss.

## Test location (suggested)
`internal/portal/automerger/outcomes_test.go:279`
