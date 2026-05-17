---
id: epic-cloud-native-deploy-hydration-handoff-wiring
kind: story
stage: implementing
tags: [portal, documentation]
parent: epic-cloud-native-deploy-hydration-handoff
depends_on: [epic-cloud-native-deploy-hydration-handoff-lifecycle]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Hydration + Handoff — Wiring + metrics + docs + SyncPushPath refactor

## Scope

The capstone wiring: config + main.go construction + metrics + foundation
docs + small breaking-change refactor of `SyncPushPath` to accept an
existing handle (LifecycleManager is now the long-held lease owner).

Implements **Unit 3** of `epic-cloud-native-deploy-hydration-handoff`.
This is the final story in epic-cloud-native-deploy.

## Files

Edit:
- `internal/portal/config/config.go` — 4 new hydration env vars
- `internal/portal/config/config_test.go`
- `cmd/portal/main.go` — construct LifecycleManager in clustered mode; start goroutine; OrgIDLookup wiring; pass to postreceive Emitter
- `internal/portal/storage/objectstore/sync.go` — refactor `SyncPushPath` to accept `handle lease.Handle` parameter; drop internal acquire
- `internal/portal/storage/objectstore/sync_test.go` — update test calls
- `internal/portal/postreceive/emitter.go` — call `LifecycleManager.AcquireForRequest` first, pass handle to SyncPushPath
- `internal/portal/metrics/metrics.go` — append hydration + lifecycle handles
- `docs/SELF_HOST.md` — remove hydration from §14 limitations; add hydration env vars
- `docs/ARCHITECTURE.md` — Horizontal Scaling section update
- `docs/SPEC.md` — clustered mode promoted from "preview"

## Config additions

- `JAMSESH_HYDRATION_IDLE_TIMEOUT_S` (default 300, i.e. 5m)
- `JAMSESH_HYDRATION_CACHE_MAX_BYTES` (default 0 = unlimited)
- `JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S` (default 30)
- `JAMSESH_HYDRATION_WORKERS` (default 8)

Validation: positive integers when non-zero (CacheMaxBytes can be 0 = unlimited).

## Metric handles

Append to Registry + register in New():
- `HydrationsTotal *prometheus.CounterVec` — labels {result} ∈ {ok, fresh, error}
- `HydrationDurationSeconds prometheus.Histogram`
- `HydrationBytesTotal prometheus.Counter`
- `LifecycleActiveSessions prometheus.Gauge`
- `LifecycleEvictionsTotal *prometheus.CounterVec` — labels {reason} ∈ {idle, lru, lost, shutdown}

## Acceptance criteria

- [ ] Config validation accepts positive values; rejects negative
- [ ] cmd/portal/main.go constructs LifecycleManager + starts goroutine in clustered mode
- [ ] OrgIDLookup wired to query Store
- [ ] postreceive Emitter routes through `LifecycleManager.AcquireForRequest`; passes returned handle to SyncPushPath
- [ ] SyncPushPath signature accepts handle; internal Lease.Acquire removed
- [ ] sync.go tests updated for new signature
- [ ] Metrics emit on hydration + eviction events
- [ ] SELF_HOST §14 no longer mentions hydration-handoff as a limitation
- [ ] ARCHITECTURE Horizontal Scaling section reflects shipped capability
- [ ] SPEC Deployment shape removes "preview" framing for clustered mode
- [ ] `go build ./...` + `go test ./...` green
- [ ] No "previously" / "originally" prose in docs

## Notes

- This is the final story. After it lands, the clustered-mode architecture is COMPLETE: routing + leases + durability + hydration all shipped. Operators can deploy multi-pod clustered with full session migration.
- SyncPushPath refactor is mechanical — the pipeline story documented this as deferred to hydration-handoff. Update Syncer struct (drop Lease field if no longer used internally) and the test calls.
- Foundation-doc principle: clustered mode is now SHIPPED, not preview. Update framing throughout (SELF_HOST, ARCHITECTURE, SPEC).
