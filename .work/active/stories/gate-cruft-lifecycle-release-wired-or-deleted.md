---
id: gate-cruft-lifecycle-release-wired-or-deleted
kind: story
stage: review
tags: [cleanup, portal, infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# `LifecycleManager.Release` exported wrapper has no production callers

## Confidence
Medium

## Category
passthrough wrapper / test-only API surface

## Location
`internal/portal/storage/objectstore/lifecycle.go:231-233`

## Evidence
```go
// Release is a thin wrapper around releaseWithReason with reason "explicit".
func (m *LifecycleManager) Release(ctx context.Context, sessionID string) error {
    return m.releaseWithReason(ctx, sessionID, "explicit")
}
```

## Removal
Production code never calls `LifecycleManager.Release` (only
`releaseWithReason("lost")` and `releaseWithReason("session_complete")`
etc. are invoked internally). The "explicit" reason is therefore never
recorded. Either delete `Release` (and its three test call sites) or
wire it into the planned external-release entry point. If the intent is
for an upcoming caller, file as a tracking gap.

## Implementation notes

Took option (a): deleted `LifecycleManager.Release` from `lifecycle.go`.
The three test call sites (`TestLifecycle_DrainBeforeRelease`,
`TestLifecycle_ReleaseEvictsRepo`, `TestLifecycle_ConcurrentReleaseAcquire`)
were each using `Release` as a convenience to invoke `releaseWithReason`
internally; replaced each with a direct call to
`mgr.releaseWithReason(ctx, sessionID, "explicit")`.
`go test ./internal/portal/storage/objectstore/...` passes cleanly.
