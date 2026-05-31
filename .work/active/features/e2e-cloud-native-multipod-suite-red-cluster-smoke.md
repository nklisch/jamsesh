---
id: e2e-cloud-native-multipod-suite-red-cluster-smoke
kind: feature
stage: done
tags: [portal, infra, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: [e2e-cloud-native-multipod-suite-red-objectstore-sync, e2e-cloud-native-multipod-suite-red-lease-migration, e2e-cloud-native-multipod-suite-red-router-redispatch]
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
---

# Clustered smoke / git-clone-over-router integration gate

## Brief
`scaffolding/cluster_smoke_test.go` fails at the prerequisite
`git clone <router>/git/...` (exit 128), so the clustered smoke — push → RPO=0
object-storage sync → graceful drain → cross-pod handoff → lease migration —
never runs to completion. First check the recently heavily-modified githttp
paths for a regression (the bug-squash work on receive-pack-truncated,
git-auth-client-abort, looksLikeReportStatus) and fix clone-over-router; then
confirm the full clustered smoke goes green.

Because the smoke test exercises object-storage sync, lease migration, and
router handoff end-to-end, this is the cross-cutting integration gate for the
epic. It depends on the three subsystem fixes landing first so that "smoke
green" is a true signal rather than masking an upstream subsystem bug. The
clone-exit-128 fix itself (githttp/router routing) is independent and can be
diagnosed first, but the suite is not declared green until the subsystem fixes
are in.

## Epic context
- Parent epic: `e2e-cloud-native-multipod-suite-red`
- Position in epic: integration gate — depends on objectstore-sync,
  lease-migration, and router-redispatch. Last to land; its green is the
  scaffolding suite's green.

## Foundation references
- `docs/ARCHITECTURE.md` — git smart-HTTP serving + router routing/auth
- Primary package: `internal/portal/githttp/` (+ router routing, cluster fixture)
- Representative red tests: scaffolding `cluster_smoke_test.go`
- Related recently-changed code: githttp receive-pack / auth-abort / report-status
  handling (see archived `bug-squash-*` git stories)

## Resolution (2026-05-31) — GREEN via upstream fixes, no separate code

The `git clone <router>/git/... exits 128` failure was the dead-pod **502**
(router routed to a killed pod), not a githttp-clone regression. It is resolved
by the router transport-error failover fix (`e2e-router-dead-pod-502-eviction`,
commit `75c4d23f`) plus the githttp thin-pack delta-base fix
(`e2e-handoff-githttp-push-disconnect-to-hydrated-survivor`, `1cc1369e`) and the
lease-takeover + objectstore-ref fixes. With all of these in, the full clustered
smoke (clone-over-router → push → RPO=0 sync → graceful drain → cross-pod
handoff → lease migration) passes end to end.

Verified: `go test -p1 ./scaffolding/ -run TestClusteredSmoke` → **PASS (22.29s)**.
No dedicated implementation was required for this gate — it was a true
integration signal that the subsystem fixes composed correctly.
