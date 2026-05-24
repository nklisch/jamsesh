---
id: story-refactor-per-package-clock-compliance-lease
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

# lease.RunRetention: accept clock parameter for retention cutoff

## Brief

`internal/portal/lease/retention.go:29` computes the retention cutoff
via `cutoff := time.Now().Add(-retentionAfter)`. The function is a
free function — there is no struct to attach a Clock field to — so the
parameter-passing form (matching `auth.FindOrProvisionAt`) is the right
fit.

## Current state

```go
// internal/portal/lease/retention.go
func RunRetention(ctx context.Context, store store.LeaseStore, retentionAfter time.Duration) error {
    cutoff := time.Now().Add(-retentionAfter)
    // ... uses cutoff
}
```

## Target state

```go
// internal/portal/lease/retention.go

// RunRetention deletes lease rows whose UpdatedAt is older than
// `now.Add(-retentionAfter)`. Production callers pass the system clock's
// current time; tests pass a fake `now` to drive deterministic deletion
// without real wall-clock waits.
func RunRetention(ctx context.Context, store store.LeaseStore, retentionAfter time.Duration, now time.Time) error {
    cutoff := now.Add(-retentionAfter)
    // ... uses cutoff
}
```

## Implementation notes

- Parameter-passing form (no struct, no package Clock interface) — `lease`
  is a small package with a handful of free functions; introducing a Clock
  interface here is overkill.
- Update every caller of `RunRetention`. Search:
  ```
  grep -rn "lease.RunRetention\b\|RunRetention(" internal/ cmd/
  ```
- For callers that have a clock (handler-style components), pass `h.clock.Now()`.
- For callers that don't (boot-path code in `cmd/portal/main.go`), pass
  `time.Now().UTC()` explicitly at the boundary.
- Add a unit test in `internal/portal/lease/retention_test.go` that drives
  deletion of "old" rows via an explicit `now` — no real wait.

## Acceptance criteria

- [ ] `RunRetention` signature is `(ctx, store, retentionAfter, now)`.
- [ ] All callers updated to pass `now`.
- [ ] A new unit test in `retention_test.go` covers the time-cutoff logic
      with an explicit `now` parameter.
- [ ] `go build ./...` clean.
- [ ] `go test ./internal/portal/lease/...` clean.

## Risk

**Low.** Single function, signature change is mechanical, compiler catches
missed callers.

## Implementation notes

- Changed `RunRetention` signature from
  `(ctx, s, interval, retentionAfter)` to
  `(ctx, s, interval, retentionAfter, now time.Time)`.
- Cutoff line changed from `time.Now().Add(-retentionAfter)` to
  `now.Add(-retentionAfter)`.
- One caller updated: `cmd/portal/main.go:363` — boot-path, no clock field,
  passes `time.Now().UTC()` explicitly at the boundary.
- Added `TestRunRetention_CutoffUsesNow` in `retention_test.go`: uses a
  `retentionStub` (inline stub implementing only
  `DeleteReleasedLeasesOlderThan`) with a synthetic `now` of
  2026-01-15T12:00:00Z and a 30-day retention window; asserts the cutoff
  delivered to the store equals `2025-12-16T12:00:00Z`. No `time.Sleep`.
  Passes in ~0.01s.
- Updated the two existing test calls to pass `time.Now().UTC()`.
- `go build ./...` clean; `go test ./...` fully green (no failures).

## Rollback

`git revert` the implementation commit.

## Out of scope

- `internal/portal/lease/postgres.go:177` also has a direct `time.Now()` call.
  That site is in the lease driver, not the retention function — it shows
  up in the per-feature discovery but is left for a follow-up story to keep
  this one tight.
