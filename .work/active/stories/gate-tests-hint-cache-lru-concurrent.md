---
id: gate-tests-hint-cache-lru-concurrent
kind: story
stage: done
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

## Implementation notes

### Production code contract

`Hint` uses a single `sync.Mutex` that wraps the entire body of both `Get` and
`Set`. This means every operation — including the LRU `MoveToFront` inside
`Get` and the eviction-then-insert inside `Set` — is fully serialized. There
is no race window between "decide which entry to evict" and "remove it": by
the time `Set` acquires the lock, the LRU list already reflects every `Get`
that completed before the lock was granted. The cache therefore guarantees
**strict LRU ordering even under concurrent access**, not merely
"eventual/approximate LRU."

### Synchronization strategy

1. Keys k0..k9 are inserted in reverse order (k9 first, k0 last) so that after
   pre-population k0 is at the MRU front and k9 is at the LRU back.
2. Eight goroutines each call `Get("k0")` once, signal via a buffered channel
   (countdown-latch pattern), and then loop calling `Get("k0")` until a done
   channel is closed. The buffered channel ensures no goroutine blocks the
   main goroutine.
3. The main goroutine waits for all eight signals before firing a single
   `Set("k10", "v10")`. This synchronization point guarantees k0's promotion
   has been observed by the LRU list before the eviction decision is made.
4. Exactly one evicting `Set` is fired. Firing many Sets beyond the original
   key set would legitimately make k0 the LRU eventually (once all nine
   competing keys are newer than k0's last promotion), so the test limits to
   one eviction to keep the assertion unambiguous.

### Tolerance and invariant

Because the implementation is strictly serialized, the test asserts the hard
invariant: k0 survives the eviction and k9 (the true LRU) is gone. No
tolerance or flakiness guard is needed. The race detector (`-race`) confirmed
no data races exist across 1.098 s of execution covering all package tests.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: LRU correctness under concurrent Get verified. 8 goroutines run Get(k0) in a tight loop; one Set(k10) fires after a buffered-channel barrier ensures all goroutines have promoted k0. Strict invariant holds: k0 survives, k9 (oldest) evicts, k10 present. Implementation uses a single mutex around all Get/Set bodies → no race window between LRU promotion and eviction. -race clean. Initial test design (500 evicting Sets) correctly failed (k0 can become LRU after enough eviction churn); the corrected single-Set design captures the spec invariant precisely.
