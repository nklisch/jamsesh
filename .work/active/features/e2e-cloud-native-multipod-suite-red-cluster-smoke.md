---
id: e2e-cloud-native-multipod-suite-red-cluster-smoke
kind: feature
stage: drafting
tags: [portal, infra, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: [e2e-cloud-native-multipod-suite-red-objectstore-sync, e2e-cloud-native-multipod-suite-red-lease-migration, e2e-cloud-native-multipod-suite-red-router-redispatch]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
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
