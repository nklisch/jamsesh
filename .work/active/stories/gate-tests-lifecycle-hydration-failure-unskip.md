---
id: gate-tests-lifecycle-hydration-failure-unskip
kind: story
stage: drafting
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
