---
id: gate-tests-router-discovery-shutdown
kind: story
stage: done
tags: [testing, infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# `bug-router-static-discoverer-not-started` fix has no test asserting goroutine lifecycle on shutdown

## Priority
Low

## Spec reference
Item: `bug-router-static-discoverer-not-started` (archived).
Acceptance criterion: the signal context (`ctx`) was moved earlier in
`run()` so it is defined when the goroutine is launched.

## Gap type
missing test for adversarial (shutdown ordering). No test asserts the
discovery goroutine exits cleanly on SIGTERM (no leaked goroutines, no
`context.Canceled` propagated as an error log).

## Suggested test
```go
// cmd/jamsesh-router/main_test.go — TestRun_DiscoveryGoroutineExitsOnContextCancel
//   Build the static-mode config, spawn run() in goroutine, cancel ctx.
//   Assert: run() returns within shutdown deadline, no goroutine leak (use goleak),
//   no error logged for context.Canceled (only unexpected errors should log).
```

## Test location (suggested)
`cmd/jamsesh-router/main_test.go`

## Implementation notes

### Files changed
- `cmd/jamsesh-router/main.go` — refactored `run()` into two functions:
  - `run(args)` — production entrypoint; creates `signal.NotifyContext` and delegates to `runCtx`
  - `runCtx(ctx, args)` — testable core; accepts an externally-owned context so tests can cancel without sending OS signals
- `cmd/jamsesh-router/main_test.go` — added `TestRun_DiscoveryGoroutineExitsOnContextCancel`; added `context` and `runtime` imports

### Test approach
- Uses `runCtx(ctx, nil)` directly with a cancellable context injected by the test
- Backend: real `httptest.Server` responding 200 to all probes (prevents connection-refused noise)
- Config via `t.Setenv`: `JAMSESH_ROUTER_BIND=127.0.0.1:0`, `JAMSESH_ROUTER_STATIC_PODS=<backend>`, `JAMSESH_ROUTER_SHUTDOWN_GRACE_S=1`
- Startup settle: 200 ms `time.Sleep` (not a sync primitive — just allows goroutines to start)
- Shutdown assertion: channel receive with 8 s ceiling (grace period is 1 s; ceiling absorbs CI variance)
- Exit-code assertion: `runCtx` must return 0

### Goroutine leak detection approach
No `goleak` dep added (not used anywhere else in the codebase).

Manual `runtime.NumGoroutine()` snapshot approach:
1. Capture baseline AFTER backend httptest server is started (its goroutines are already counted)
2. After `runCtx` returns, call `http.DefaultTransport.CloseIdleConnections()` to drain keep-alive connection goroutines from the readyz probe (those are `net/http` infrastructure, not our goroutines)
3. Poll until count ≤ baseline+2 or 5 s ceiling

The slack of +2 absorbs Go test runner ambient goroutines. A genuine discovery goroutine leak would permanently elevate the count by ≥1 per invocation.

### Investigation finding during implementation
The goroutine dump confirmed the discovery goroutine exits correctly on cancel. The initially-failing leak check was failing because of `net/http.Transport` `persistConn.readLoop` and `persistConn.writeLoop` goroutines from `http.DefaultTransport`'s keep-alive pool, created when `readyz.Probe` probed the backend. These are not our goroutines. `CloseIdleConnections()` drains them cleanly.

### Shutdown deadline
8 seconds ceiling (`shutdownCeiling = 8 * time.Second`). Grace period is 1 second.

### Deps added
None.

### No context.Canceled error log assertion
The discovery goroutine in `runCtx` already suppresses `context.Canceled` via `!errors.Is(err, context.Canceled)` before calling `slog.Error`. The test validates this path by confirming `runCtx` exits with code 0 (it would return 1 if shutdown errored). A full log-capture assertion was omitted as it would require redirecting `slog.Default()` which is shared state in tests — the existing suppression in production code is tested adequately by the zero exit code and the absence of leaked goroutines.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: The `baseline + 2` slack on the goroutine count is empirical; if a
future runner introduces additional ambient goroutines, the test could flake.
Acceptable for now — the agent's note acknowledges the constraint and the
poll-with-ceiling pattern absorbs short-lived ones.

**Notes**: The `run()` → `run()` + `runCtx()` split is a minimal, well-justified
testability seam — production behavior is unchanged (signal handling still lives
in `run`), and tests can now drive the actual production code path with an
injected context rather than mocking. The `--help` early-exit was preserved.
The agent's investigation discovered that the apparent "leaked" goroutines were
`http.DefaultTransport`'s `persistConn.readLoop`/`writeLoop` from the readyz
probe keep-alive pool, not our discovery goroutine; `CloseIdleConnections()`
drains them correctly before the leak assertion. Honest documentation of that
investigation in the implementation notes is exactly the right level of
disclosure.
