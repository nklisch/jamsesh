---
id: epic-cloud-native-deploy-routing-layer-hint-cache
kind: story
stage: implementing
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

- [ ] Set then Get within TTL → hit with same podID
- [ ] Get after TTL → miss
- [ ] Set N entries with maxEntries=N then 1 more → oldest evicted
- [ ] Invalidate then Get → miss
- [ ] Concurrent Get/Set race-free under `go test -race`
- [ ] `New(maxEntries int, ttl time.Duration) *Hint` returns ready cache

## Notes

- Implementation: `container/list` + `map[string]*list.Element` under
  `sync.Mutex`. Acceptable for v1 routing rates; revisit if lock
  contention shows up in metrics.
- Default sizing chosen by service binary: `maxEntries=10_000`,
  `ttl=60s`.
