---
id: gate-tests-lifecycle-hydration-failure-unskip
kind: story
stage: done
tags: [testing, portal, infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Lifecycle `TestLifecycle_HydrationFailure` skips its lease-cleanup assertion conditionally

## Priority
Medium

## Spec reference
Item: `epic-cloud-native-deploy-hydration-handoff-lifecycle`
Acceptance criterion: `AcquireForRequest` on hydration failure → releases
lease + returns error.

## Gap type
test-integrity. `lifecycle_test.go:387` skips with "lease was not even
acquired — no handle to check"; line 506 skips with "repo directory was
never created — eviction test not meaningful". Both hide a flaky fixture
instead of failing loudly.

## Suggested test
Refactor so the precondition (lease acquired; repo created) is enforced
via `t.Helper`+ `t.Fatalf` on failure — or restructure to a separate
"lease acquired then hydration fails" scenario where the precondition is
guaranteed.

## Test location (suggested)
`internal/portal/storage/objectstore/lifecycle_test.go`

## Implementation notes

Applied Option A (convert to `t.Fatalf`) at both locations:

- `lifecycle_test.go:387`: `t.Skip` → `t.Fatalf("precondition failed: lease was not even acquired ...")`
- `lifecycle_test.go:506`: `t.Skip` → `t.Fatalf("precondition failed: repo directory was never created ...")`

Both `TestLifecycle_AcquireForRequest_HydrationFailure` and
`TestLifecycle_Release_EvictsLocalCache` pass after the change — the fixture
was fine all along and the skips were over-defensive. No production bug
surfaced; no backlog item needed.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Both t.Skip calls at lifecycle_test.go:387 and :506 converted to t.Fatalf. Both tests now run their assertions on every invocation. PASS — the fixtures were correct all along; the skips were purely over-defensive. No production bug surfaced. Future fixture flake will now fail loud.
