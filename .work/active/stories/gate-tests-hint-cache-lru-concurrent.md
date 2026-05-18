---
id: gate-tests-hint-cache-lru-concurrent
kind: story
stage: drafting
tags: [testing, infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Hint cache eviction under high concurrency has no LRU-ordering assertion

## Priority
Medium

## Spec reference
Item: `epic-cloud-native-deploy-routing-layer-hint-cache`
Acceptance criterion: set N entries with maxEntries=N then 1 more →
oldest evicted; concurrent Get/Set race-free under `go test -race`.

## Gap type
missing test for boundary (LRU under concurrent Get). Under concurrent
Get-promotes-front + Set-evicts-back, an entry could be evicted between
its Get and the LRU promotion completing.

## Suggested test
```go
// TestHintCache_LRUCorrectnessUnderConcurrentGet
//   maxEntries=10. Pre-populate keys k0..k9. Spawn N goroutines doing
//   Get(k0) in a loop. Concurrently Set k10. Assert k0 NEVER evicted
//   (since it's most-recently-accessed), evicted entry is one of k1..k9.
```

## Test location (suggested)
`internal/router/cache/hint_test.go`
