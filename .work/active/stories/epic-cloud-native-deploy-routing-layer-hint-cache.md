---
id: epic-cloud-native-deploy-routing-layer-hint-cache
kind: story
stage: done
tags: [infra]
parent: epic-cloud-native-deploy-routing-layer
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Soft-coordinator hint cache

## Scope

LRU-bounded in-memory cache mapping `session_id → pod_id` with per-entry
TTL. Supports `Get` / `Set` / `Invalidate`. No persistence; in-memory only.

Implements **Unit 4** of `epic-cloud-native-deploy-routing-layer`. See
parent feature body for the interface and rationale.

## Files

New:
- `internal/router/cache/hint.go`
- `internal/router/cache/hint_test.go`

## Acceptance criteria

- [x] Set then Get within TTL → hit with same podID
- [x] Get after TTL → miss
- [x] Set N entries with maxEntries=N then 1 more → oldest evicted
- [x] Invalidate then Get → miss
- [x] Concurrent Get/Set race-free under `go test -race`
- [x] `New(maxEntries int, ttl time.Duration) *Hint` returns ready cache

## Notes

- Implementation: `container/list` + `map[string]*list.Element` under
  `sync.Mutex`. Acceptable for v1 routing rates; revisit if lock
  contention shows up in metrics.
- Default sizing chosen by service binary: `maxEntries=10_000`,
  `ttl=60s`.

## Implementation notes

- `internal/router/cache/hint.go`: `Hint` struct wraps `container/list`
  (doubly-linked) + `map[string]*list.Element` under `sync.Mutex`. Each
  list element value is a pointer to a private `entry` struct holding
  `sessionID`, `podID`, and `expiry time.Time`.
- `Get`: expired entries are deleted from both the list and the map before
  returning a miss, preventing unbounded map growth.
- `Set`: updates the value and TTL in-place when the key already exists,
  then promotes to front; inserts at front otherwise. LRU eviction (back
  of list) fires before insertion when at capacity.
- `Invalidate`: removes from both structures; no-op when absent.
- `removeElement` is the single shared helper that maintains list/map
  consistency — called from Get (expiry), Set (eviction), and Invalidate.
- New panics on invalid arguments (≤0 maxEntries or ≤0 TTL) to catch
  misconfiguration at startup rather than silently misbehaving.
- 12 unit tests cover all acceptance criteria, TTL refresh, value update
  on re-Set, absent-key Invalidate, and map-cleanup after expiry. All pass
  under `go test -race`.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Idiomatic Go LRU implementation. `container/list` (doubly-linked, front=MRU) + `map[string]*list.Element` under `sync.Mutex` is the standard pattern. Expired entries are deleted from both structures on Get, preventing unbounded map growth. `Set` correctly updates-and-promotes for existing keys vs evict-then-insert at capacity. `New` panics on `maxEntries <= 0 || ttl <= 0` — fail-fast at startup is the right call. 12 tests cover all acceptance criteria + edge cases including concurrent Get/Set/Invalidate under `-race`.
