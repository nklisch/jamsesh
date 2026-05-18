---
id: epic-cloud-native-deploy-hydration-handoff-wiring
kind: story
stage: done
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

## Implementation notes

- `Syncer.Lease` field removed; `SyncPushPath` now accepts `handle lease.Handle` as its third parameter. The only caller (postreceive `Emitter`) gets the handle from `LifecycleManager.AcquireForRequest` in clustered mode, or a noop handle in single-instance mode.
- `GetSessionByID` added to `store.Store`, `store.SessionStore`, both adapters (`sqliteAdapter`, `postgresAdapter`) and both TxStore wrappers. Hand-written in `internal/db/sqlitestore/sessions_extra.go` and `internal/db/pgstore/sessions_extra.go` following the existing `*_extra.go` pattern.
- `handlerauth_test.go` `stubStore` updated with no-op `GetSessionByID` method to satisfy the updated interface.
- `LifecycleManager` constructed in `cmd/portal/main.go` only when `cfg.DeployMode == "clustered"`; it is `nil` in single-instance mode. `Emitter` handles `nil` gracefully by falling back to a noop lease.
- ARCHITECTURE.md: removed preview callout; replaced "Hydration handoff (to come)" with shipped description; updated fencing-token paragraph to describe the implementation rather than future plans.
- SELF_HOST.md §14: removed preview callout and preview-limitations subsection; added hydration env vars subsection with the four new knobs.
- `go build ./...` and `go test ./...` both clean.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Final story in the epic. Clean execution across 6 surfaces:
1. Config — 4 new hydration env vars with defaults + validation + tests
2. SyncPushPath refactor — Syncer.Lease field removed; handle is now caller-provided. Mechanical fix as designed; only one caller (postreceive Emitter) needed updating
3. Emitter wiring — nil-safe Lifecycle handoff; single-instance falls back to noop handle for the Syncer call
4. main.go wiring — Hydrator + LifecycleManager constructed in clustered mode; background goroutine started; OrgIDLookup wired to GetSessionByID
5. GetSessionByID — added to store.Store interface + both adapter impls + TxStore wrappers + stubStore patched. Hand-written sql.go files follow the established `*_extra.go` pattern (sqlc not in env)
6. Docs — SELF_HOST §14 preview callout removed; hydration env vars subsection added; ARCHITECTURE preview callout replaced with shipped description; no "previously" prose

Build + tests green. The clustered-mode capability is now COMPLETE in code.
