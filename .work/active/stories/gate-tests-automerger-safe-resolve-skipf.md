---
id: gate-tests-automerger-safe-resolve-skipf
kind: story
stage: done
tags: [testing, portal]
parent: null
depends_on: []
release_binding: null
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

## Implementation notes

Changed `internal/portal/automerger/outcomes_test.go` line 279:
- Before: `t.Skipf("did not get SafeAutoResolve (got %s); this test requires a whitespace conflict", result.Kind)`
- After: `t.Fatalf("expected SafeAutoResolve for whitespace-only conflict, got %s", result.Kind)`

The whitespace-conflict subtest is deliberately constructed to produce a `SafeAutoResolve` classification. If the classifier ever drifts, the test now fails rather than silently skipping.

Build: `go build ./...` — clean, no errors.
Tests: `go test ./internal/portal/automerger/...` — ok (0.873s), all passing. The whitespace conflict continues to classify as `SafeAutoResolve` on main.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: Replacement string is clearer than the original — the new fatal explicitly states the expectation.

**Notes**: Change matches the story design exactly: one-line `t.Skipf` → `t.Fatalf` swap. The replacement assertion message ("expected SafeAutoResolve for whitespace-only conflict, got %s") is more informative than the original skip wording. Verification re-confirmed locally: whitespace-only conflicts continue to classify as SafeAutoResolve on main, so the now-stricter assertion does not regress.
