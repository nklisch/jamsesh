---
id: lease-collision-check-not-returning-erralreadyheld
kind: story
stage: implementing
tags: [portal, lease, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# PostgresManager.Acquire collision check does not return ErrAlreadyHeld

## Gap type
bug. `TestPostgresCollisionDefensiveCheck` surfaces a real product defect.

## What fails
`TestPostgresCollisionDefensiveCheck` sets up a row where
`pod_id = "pod-original"` with `released_at IS NULL`, releases the advisory
lock, then calls `mgr.Acquire` from `"pod-collision"`. The test expects
`ErrAlreadyHeld` but `Acquire` returns `nil` — the collision check in
`PostgresManager.Acquire` is not detecting the stale row correctly.

## Test location
`internal/portal/lease/postgres_test.go` — `TestPostgresCollisionDefensiveCheck`

## Fix direction
Inspect `internal/portal/lease/postgres.go` `Acquire` method: the collision
guard should query the `leases` row after acquiring the advisory lock and
return `ErrAlreadyHeld` when `pod_id != mgr.PodID AND released_at IS NULL`.
