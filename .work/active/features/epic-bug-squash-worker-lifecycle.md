---
id: epic-bug-squash-worker-lifecycle
kind: feature
stage: drafting
tags: [bug, portal]
parent: epic-bug-squash
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Background-worker lifecycle & shared-state concurrency

## Brief

The portal runs several long-lived background workers — pg advisory-lease
heartbeat/retention, the WS-gateway ticket janitor, the object-store lifecycle
LRU/idle reaper, and the rate limiter — each holding shared mutable state behind
locks, channels, and tickers. The bug-scan found six concurrency/lifecycle
defects across them: a `Release`-vs-heartbeat race on one `*sql.Conn`, a
retention cutoff frozen at startup, a `Stop`-double-close panic, a slow-consumer
close that leaves a dead conn subscribed, an LRU pass that evicts just-active
sessions off a stale snapshot, and an inconsistent rate-limit reservation
cancel.

This feature delivers correct lifecycle and shared-state handling for these
workers: no double-close panics, no frozen time references in ticker loops, no
unsynchronized concurrent use of a pooled connection, and eviction/idle
decisions re-validated at decision time. It covers correctness of the existing
workers only — it does NOT add new workers, change lease/storage semantics, or
alter the rate-limit policy values.

## Epic context
- Parent epic: `epic-bug-squash`
- Position in epic: independent backend feature — parallelizable with the other
  backend features (distinct packages: lease, wsgateway, objectstore, ratelimit).

## Foundation references
- `docs/ARCHITECTURE.md` — Portal § WS gateway, Playground destroyer, storage
- Patterns: `ticker-sweep-loop`, `per-package-clock-interface`

## Child stories (pre-existing, from bug-scan — re-parented here)
- `bug-squash-pghandle-heartbeat-conn-race` — Medium, concurrency — `internal/portal/lease/postgres.go:219`
- `bug-squash-lease-retention-frozen-now` — Medium, time-numbers — `internal/portal/lease/retention.go:25`
- `bug-squash-lru-evicts-hot-sessions` — Medium, concurrency — `internal/portal/storage/objectstore/lifecycle.go:350`
- `bug-squash-ticketstore-stop-double-close` — Medium, concurrency — `internal/portal/wsgateway/tickets.go:92`
- `bug-squash-gateway-slow-consumer-close` — Low, concurrency — `internal/portal/wsgateway/gateway.go:127`
- `bug-squash-ratelimit-reservation-cancel` — Low, concurrency — `internal/portal/ratelimit/store.go:106`

<!-- feature-design fills in the per-worker fix approach and concurrency test
strategy (race-detector runs, fake-clock advancement). -->
