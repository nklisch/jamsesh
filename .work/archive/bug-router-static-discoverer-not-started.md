---
id: bug-router-static-discoverer-not-started
kind: story
stage: done
tags: [bug, router, discovery, Important]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-18
---

# Bug: Static Discoverer Run Loop Not Started in Router Main

## Summary

`cmd/jamsesh-router/main.go` constructs the static discoverer and the
readiness probe but never starts the discoverer's `Run` goroutine. The
ring is seeded once at startup from the static pod list and is never
updated thereafter.

**Impact**: When a backend pod dies (SIGKILL, OOM-kill, crash), the router
continues routing to the dead pod's address indefinitely. Clients receive
`502 Bad Gateway` for all sessions that hash to the dead pod. No re-sharding
occurs. The session is permanently unavailable until the router is restarted.

This defeats the availability goal of the routing layer for clustered
deployments.

## Root cause

`main.go` (lines 97–129) explicitly notes "the discovery story / Unit 3 will
overlay this" and leaves two placeholder suppressions:

```go
_ = publishWithMetrics // used below when discovery is wired; silences unused warning
_ = probe              // used by discovery; probe is constructed here for metrics wiring
```

The wiring was deferred during the cloud-native-deploy epic but was never
completed. The discoverer implementation itself is correct and tested
(`internal/router/discovery/static.go`, `static_test.go`).

## Fix

Wire the static discoverer `Run` loop in `main.go` after the ring is seeded:

```go
// Start the static discovery loop to evict dead pods from the ring.
// The loop probes all configured backends every cfg.ProbeInterval and
// calls ring.SetPods atomically when the healthy set changes.
disc := discovery.Static(cfg.StaticPods, probe, cfg.ProbeInterval)
go func() {
    if err := disc.Run(ctx, publishWithMetrics(r.SetPods)); err != nil && !errors.Is(err, context.Canceled) {
        slog.Error("jamsesh-router: discovery loop exited unexpectedly", "err", err)
    }
}()
```

This matches the `Run` signature in `internal/router/discovery/discovery.go`
and the `publishWithMetrics` wrapper already constructed in `main.go`.

## Acceptance criteria

- [ ] `cmd/jamsesh-router/main.go` starts the static discoverer goroutine when
      `cfg.DiscoveryMode == "static"`.
- [ ] `tests/e2e/failure/router_backend_dead_test.go` subtest
      `dead_pod_removed_from_routing_pool` passes within the 15s SLO (remove
      the `t.Skip` call once the wiring is added and the test is green).
- [ ] The router image (`make test-router-image`) is rebuilt after the fix so
      the e2e test uses the updated binary.

## Surfaced by

`tests/e2e/failure/router_backend_dead_test.go` (story
`epic-e2e-cnd-coverage-routing-layer-failure-backend-dead`) — the test is
skipped with a reference to this backlog item until the fix lands.

## Severity

**Important** — a dead backend pod causes a permanent 502 for all sessions
hashing to it. In a 3-pod cluster, ~33% of sessions are affected per pod
failure. The fix is small (one `go func()` call + import), but the test
must be green before the backlog item is closed.

## Implementation notes

### What was done

Wired `discovery.Static(...).Run(ctx, publishWithMetrics(r.SetPods))` into
`cmd/jamsesh-router/main.go` inside the existing `cfg.DiscoveryMode == "static"`
block. The goroutine is started after the ring is seeded from static config.

**Structural change**: The signal context (`ctx`) was moved earlier in `run()` —
before the static-mode block — so it is defined when the goroutine is launched.
This is a safe ordering change: signal notification registration is now slightly
earlier, but SIGTERM handling was already deferred to the `select` at the bottom.

**Placeholder suppressions removed**: The two `_ = publishWithMetrics` and
`_ = probe` assignments that silenced unused-variable warnings are gone; both
variables are now consumed by the discovery goroutine.

**Import added**: `"jamsesh/internal/router/discovery"` added to the import block.

### Config field

`cfg.ProbeInterval` exists in `internal/router/config/config.go` with a default
of 5s — matches the story sketch exactly. No adaptation needed.

### e2e test un-skipped

`tests/e2e/failure/router_backend_dead_test.go` — the `t.Skip(...)` call in
`testDeadPodRemovedFromRoutingPool` was removed. The test is ready to run.
The acceptance criterion noted this was optional if simple; it was one line.

### Verification

- `go build ./cmd/jamsesh-router/...` — clean
- `go vet ./cmd/jamsesh-router/...` — clean
- `go test ./internal/router/... -timeout 60s` — all 7 packages pass
- `make test-router-image` — rebuilt `jamsesh/router:e2e` successfully

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Fix is exactly as designed — ~5 LoC plus one import in `main.go`.
The signal context (`ctx`) is correctly moved before the static-mode block so
the discovery goroutine receives cancellation on shutdown. Placeholder
suppressions (`_ = publishWithMetrics`, `_ = probe`) removed cleanly. The
goroutine error path correctly ignores `context.Canceled` and logs everything
else. The `t.Skip` removal in `router_backend_dead_test.go` is surgical (7
lines, the skip call and its explaining comment, nothing else changed). Router
image rebuild step confirmed in implementation notes. No foundation-doc drift.
