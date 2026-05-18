---
id: gate-tests-ring-rebalance-cardinality
kind: story
stage: implementing
tags: [testing, infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Router consistent-hash redistribution tested only at one cardinality

## Priority
Medium

## Spec reference
Item: `epic-cloud-native-deploy-routing-layer-core`
Acceptance criterion: adding/removing 1 pod from a 5-pod ring re-routes
≤ 1/5 ± 10% keys.

## Gap type
missing test for boundary. No 1→2 pod transition test (50% movement
expected), no 100-pod test (1% movement). Also no test for
membership-stability across noop SetPods.

## Suggested test
```go
// TestRing_RebalanceBound_ByCardinality
//   table-driven: N in {2,3,5,10,50,100}; add/remove one pod; assert
//   re-routed fraction is within [1/(N+1) - 0.1, 1/(N+1) + 0.1].
// TestRing_KeyAffinity_UnchangedAcrossNoopSetPods
//   SetPods with the same input twice; assert Get for 1000 keys is stable.
```

## Test location (suggested)
`internal/router/ring/ring_test.go`
