---
id: gate-cruft-finalize-lockrelease-store-keeper
kind: story
stage: implementing
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: cruft
created: 2026-05-31
updated: 2026-05-31
---

# Finalize lock-release test has obsolete store import keeper

## Confidence
Medium

## Category
unused import

## Location
`internal/portal/finalize/lock_release_test.go:114`

## Evidence
```go
// Build-time check that storage stub satisfies storage.Service.
var _ store.FinalizeLock // keeps store import live in this file
```

The file uses `store` throughout the failing-store wrapper below this point.

## Removal
Delete the misleading comment and dummy `var _ store.FinalizeLock` line.


## Autopilot scope/design (2026-05-31)

Promoted by the gate-drain autopilot run. Implement the remediation direction above as a focused single-stride story, keep edits limited to the named surface, and verify with the targeted test or check that covers the changed file. For older backlog gate items, this run binds the work to `v0.5.0` because the user explicitly requested all gate-related work be scoped, designed, and implemented before release.
