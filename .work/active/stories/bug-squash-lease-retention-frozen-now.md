---
id: bug-squash-lease-retention-frozen-now
kind: story
stage: review
tags: [bug, portal, time-numbers]
parent: epic-bug-squash-worker-lifecycle
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: medium
bug_domain: time-numbers
bug_location: internal/portal/lease/retention.go:25
---

# Lease retention cutoff is frozen at startup — old released-lease rows are never purged

**Location**: `internal/portal/lease/retention.go:25` (call site `cmd/portal/main.go:370`) · **Severity**: medium · **Pattern**: stale captured wall-clock value inside a long-lived ticker loop

`RunRetention` takes a single `now time.Time` captured once when the goroutine launches and recomputes `cutoff := now.Add(-retentionAfter)` on every tick using that frozen value. With the default ~30-day retention, a pod up longer than the retention window stops purging anything new — a row released at `startupTime-1s` is never older than the frozen cutoff, so released-lease rows accumulate indefinitely on long-running pods. Tests pass a synthetic `now` for a single deletion, masking the loop bug. Fix: take a `Clock`/`func() time.Time` and recompute `cutoff := clock.Now().Add(-retentionAfter)` inside the `case <-ticker.C` block each tick, matching the per-package-clock convention used elsewhere.

```go
case <-ticker.C:
    cutoff := now.Add(-retentionAfter)   // `now` captured once at startup, never advances
    if err := s.DeleteReleasedLeasesOlderThan(ctx, cutoff); err != nil { ... }
```

## Implementation notes

Changed `RunRetention` signature: `now time.Time` → `nowFn func() time.Time`. The
cutoff is now recomputed as `nowFn().Add(-retentionAfter)` on every tick so it
advances with wall time across long-running pods. Updated `cmd/portal/main.go` to
pass `func() time.Time { return time.Now().UTC() }`. Updated all test call sites.
Added `TestRunRetention_CutoffAdvancesEachTick` which uses a nowFn that increments
by 24h on each call and asserts that successive cutoffs are strictly increasing.
The existing `TestRunRetention_CutoffUsesNow` was updated to use the new
`func() time.Time` signature. Build/vet/`-race` clean: `go test -race ./internal/portal/lease/...`.
