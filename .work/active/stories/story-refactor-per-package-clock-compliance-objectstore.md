---
id: story-refactor-per-package-clock-compliance-objectstore
kind: story
stage: review
tags: [portal, refactor, testing]
parent: feature-refactor-per-package-clock-compliance
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# storage/objectstore: introduce package-level Clock for LifecycleManager + Manifest.Save

## Brief

`internal/portal/storage/objectstore/` has four direct `time.Now()` calls
that defeat the per-package-clock-interface pattern:

- `manifest.go:178` — `m.UpdatedAt = time.Now().UTC()` inside `Manifest.Save`.
- `lifecycle.go:99` — `now := time.Now()` inside `sessionEntry.touchLastActive()`.
- `lifecycle.go:180` — `now := time.Now()` inside `LifecycleManager.acquireNew()`.
- `lifecycle.go:337` — `now := time.Now()` inside `LifecycleManager.evictIdleAndOversize()`.

The parent `internal/portal/storage/service.go` already defines `Clock` +
`realClock` (lines 78-88) for the `Service` type. The sub-package
`storage/objectstore` is a sibling — it should define its own
`Clock interface { Now() time.Time }` per the pattern (loose coupling
preferred over cross-package import).

## Current state

```go
// objectstore/lifecycle.go
type LifecycleManager struct {
    // ... existing fields, no clock
}

func (e *sessionEntry) touchLastActive() {
    e.mu.Lock()
    defer e.mu.Unlock()
    now := time.Now()
    // ...
}

func (m *LifecycleManager) acquireNew(...) {
    // ...
    now := time.Now()
    // ...
}

func (m *LifecycleManager) evictIdleAndOversize(...) {
    // ...
    now := time.Now()
    // ...
}

// objectstore/manifest.go
func (m *Manifest) Save(ctx context.Context, store objectstore.Store, sessionID string) error {
    // ...
    m.UpdatedAt = time.Now().UTC()
    // ...
}
```

## Target state

```go
// objectstore/clock.go (new file)
package objectstore

import "time"

// Clock is an injectable time source. Mirrors the shape used across the
// portal so *testclock.AdvanceableClock satisfies this interface too.
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }
```

```go
// objectstore/lifecycle.go
type LifecycleManager struct {
    // ... existing fields
    clock Clock // nil → realClock{} via the now() accessor
}

func (m *LifecycleManager) now() time.Time {
    if m.clock == nil {
        return realClock{}.Now()
    }
    return m.clock.Now()
}

// All three call sites: time.Now() → m.now()
// sessionEntry.touchLastActive() needs the clock too — store a *LifecycleManager
// back-reference OR have callers pass `now` in. Simpler: pass `now` as a parameter
// to touchLastActive(now time.Time).
```

```go
// objectstore/manifest.go
// Option A (preferred): parameter-passing form, matches FindOrProvisionAt.
func (m *Manifest) Save(ctx context.Context, store ..., sessionID string, now time.Time) error {
    // ...
    m.UpdatedAt = now
    // ...
}

// Callers (LifecycleManager, Syncer, ...) pass their clock's Now() in.
```

## Implementation notes

- **Two patterns in play for the same sub-package**: `LifecycleManager` is
  long-lived (one per pod), so the struct-field-Clock form fits. `Manifest`
  is a per-call value type, so the parameter-passing form fits — pattern
  reference `auth.FindOrProvisionAt(ctx, s, id, now)`. Mixing both is fine
  and intentional.
- For `sessionEntry.touchLastActive()`: rather than back-reference the
  manager, pass `now` as a parameter — callers (always the manager) already
  know their clock.
- Find every `Manifest.Save` call site (`grep -rn "\.Save(" internal/portal/storage/objectstore/`) and update them to pass `clock.Now()`.
- Verify `Syncer.SyncPushPath` (in `sync.go`) also routes through the
  parameter — it's the primary `Manifest.Save` caller.
- Add a `LifecycleManagerOption` constructor-style or a `NewLifecycleManager`
  factory if the current zero-value init in `cmd/portal/main.go` is awkward.
  Look at current call sites before deciding.
- Tests in `internal/portal/storage/objectstore/lifecycle_test.go` and
  `manifest_test.go` can now drive idle-eviction and manifest-update tests
  with a fake clock — at least one new test per surface to lock in the
  contract.

## Acceptance criteria

- [ ] `internal/portal/storage/objectstore/clock.go` exports `Clock` interface
      and unexported `realClock`.
- [ ] `LifecycleManager` carries a `clock Clock` field; all three `time.Now()`
      call sites read it (directly or via the `now()` accessor).
- [ ] `Manifest.Save` takes a `now time.Time` parameter; every caller passes
      their clock's `Now()`.
- [ ] At least one new unit test per surface drives time-dependent logic via
      a fake clock (idle-eviction in lifecycle, manifest UpdatedAt in
      manifest).
- [ ] `go build ./...` clean.
- [ ] `go test ./internal/portal/storage/objectstore/...` clean.
- [ ] Clustered-mode integration test still passes (the objectstore is part of
      the clustered-mode hot path).

## Risk

**Medium.** `LifecycleManager` and `Syncer` are on the clustered-mode hot
path. The refactor is mechanical but the surface is wider than a single
file — every `Manifest.Save` caller must be updated. Mitigation: rely on
the compiler to find call sites (Go's type system will refuse to build
otherwise).

## Rollback

`git revert` the implementation commit. The function-signature change for
`Manifest.Save` is the load-bearing risk — verify no out-of-tree callers
exist (`grep` shows objectstore is only imported by `cmd/portal/main.go`
and other `internal/portal/` packages).

## Atomic acknowledgment

`Manifest.Save`'s new `now` parameter is a signature change — once landed,
every caller must update. This is a single-commit operation by construction
(the build won't pass partway), so revert is the only rollback option.

## Implementation notes

**Files created:**
- `internal/portal/storage/objectstore/clock.go` — new file, exports `Clock` interface and unexported `realClock{}`.

**Files modified:**
- `internal/portal/storage/objectstore/lifecycle.go` — added `clock Clock` field to `LifecycleManager`, `now()` nil-safe accessor, updated `touchLastActive` to accept `now time.Time`, replaced all three `time.Now()` call sites with `m.now()`.
- `internal/portal/storage/objectstore/manifest.go` — `ManifestStore.Save` signature changed to `Save(ctx, m, ifMatch, now time.Time)`, `UpdatedAt` stamped from parameter.
- `internal/portal/storage/objectstore/sync.go` — `doSyncAt` passes `start.UTC()` as `now` to `Manifests.Save`.
- `internal/portal/storage/objectstore/lifecycle_test.go` — added `fakeClock` struct + `TestLifecycle_IdleEviction_WithFakeClock` test.
- `internal/portal/storage/objectstore/manifest_test.go` — all `Save` calls updated to pass `now`; `TestManifestStore_Save_SetsUpdatedAt` converted to deterministic fixed-time assertion; added `TestManifestStore_Save_UpdatedAt_PassedThrough`.
- `internal/portal/storage/objectstore/sync_test.go` — three `Save` seeding calls updated.
- `internal/portal/storage/objectstore/hydrate_test.go` — one `Save` seeding call updated.

**Pattern notes:**
- `LifecycleManager` uses struct-field form (`clock Clock` + nil-safe `now()` accessor) — matches `events.Log` pattern.
- `ManifestStore.Save` uses parameter-passing form — matches `auth.FindOrProvisionAt` pattern. Syncer passes `start.UTC()` (the sync start time) as the boundary value; no clock field needed on `Syncer`.
- `sessionEntry.touchLastActive` updated to accept `now time.Time` (caller always has `m.now()`); no back-reference to manager required.

**Test strategy:**
- `TestLifecycle_IdleEviction_WithFakeClock`: injects `fakeClock`, advances by 5m (no eviction), touches B, advances 10m more (A=15m idle → evicted, B=10m → survives). Zero real sleep.
- `TestManifestStore_Save_SetsUpdatedAt`: now uses fixed deterministic time `2026-05-23T10:00:00Z` instead of `time.Now()` comparison.
- `TestManifestStore_Save_UpdatedAt_PassedThrough`: exercises two successive writes with distinct times, verifies each `UpdatedAt` round-trips exactly.

**Verification:** `go build ./...` and `go test ./...` both clean.
