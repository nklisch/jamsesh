---
id: e2e-cloud-native-multipod-suite-red-objectstore-sync
kind: feature
stage: drafting
tags: [portal, infra, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Object-storage cross-pod sync / hydration

## Brief
A ref pushed via one pod must be readable — a non-empty SHA — from another pod
before any chaos is applied. Today the chaos prerequisites fail:
`handoff_under_object_storage_chaos_test.go` and `handoff_under_pod_kill_test.go`
report "pod N returned empty SHA for ref `jam/<sid>/<acct>/main`" at the
PREREQUISITE step, before chaos/kill is even injected. The object-storage sync
provider (RPO=0 push to MinIO) or the receiving pod's repo-cache hydration from
object storage is incomplete (or broken) at the moment the second pod is read.

This feature roots-causes and fixes cross-pod base-ref visibility / hydration
timing in the object-storage sync layer so a pushed ref is durably visible
cluster-wide before downstream steps run. Scope is the cloud-native multi-pod
sync path only.

It does NOT cover lease migration, router redispatch/metrics, the scaffolding
clone gate, or playground-specific tests (those are owned by the playground
epics, already done). Per the parent epic's design decisions this is never-green
stabilization — root-cause forward from the current red state, no bisect.

## Epic context
- Parent epic: `e2e-cloud-native-multipod-suite-red`
- Position in epic: independent subsystem fix — parallel with lease, router,
  and fuzz. The cluster-smoke integration gate depends on this feature.

## Foundation references
- `docs/ARCHITECTURE.md` — object-storage sync / RPO=0 component
- Primary package: `internal/portal/storage/objectstore/`
- Representative red tests (feature-design confirms the exact owned set):
  chaos `handoff_under_object_storage_chaos_test.go`,
  `handoff_under_pod_kill_test.go`, `object_storage_partition_test.go`;
  golden `object_storage_rpo0_test.go`, `object_storage_pack_manifest_test.go`
