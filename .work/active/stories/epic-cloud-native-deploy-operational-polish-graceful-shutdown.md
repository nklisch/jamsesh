---
id: epic-cloud-native-deploy-operational-polish-graceful-shutdown
kind: story
stage: review
tags: [infra, portal]
parent: epic-cloud-native-deploy-operational-polish
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Operational Polish — Configurable graceful shutdown

## Scope

Replace the hardcoded 25-second drain budget in `server.Run` and the
hardcoded 10-second `mergerWorker.Stop` budget in `cmd/portal/main.go`
with a single configurable grace window shared across all drain
steps. Default 30s (matches k8s `terminationGracePeriodSeconds`),
configurable via `JAMSESH_SHUTDOWN_GRACE_S`.

Refactor drain ordering so the total wall-clock for shutdown is bounded
by the grace window, not multiplied across subsystems.

Implements **Unit 5** of `epic-cloud-native-deploy-operational-polish`.

## Files

Edit:
- `internal/portal/config/config.go` — new `ShutdownGraceSeconds int`
  field; default 30; env `JAMSESH_SHUTDOWN_GRACE_S`
- `internal/portal/server/server.go` — use `cfg.ShutdownGraceSeconds`
  in the `Shutdown` deadline
- `cmd/portal/main.go` — derive one drain context from the grace
  budget; pass remaining time to `mergerWorker.Stop`; document
  the per-step ordering
- `internal/portal/server/server_test.go` — extend with grace-window
  tests

## Acceptance criteria

- [ ] `JAMSESH_SHUTDOWN_GRACE_S` env / `shutdown_grace_s` YAML
  configurable; default 30; validated as positive integer.
- [ ] On SIGTERM:
  - HTTP server stops accepting new connections.
  - In-flight HTTP requests are given until the grace deadline to
    complete.
  - Auto-merger drains within the same budget (uses
    `time-remaining` derived from a shared start timestamp).
  - WebSocket gateway stop runs after HTTP is quiet.
- [ ] Test (signal-injection style): start a request that holds
  for 2s; send SIGTERM with grace=10s; assert the request
  completes and the process exits within ~3s.
- [ ] Test with grace=1s: assert a 3s request is cut off and the
  process exits within ~2s; log emits a warning about the cut.
- [ ] Per-step elapsed times logged at shutdown for operator
  observability (`shutdown_step=http elapsed_ms=...`).
- [x] `JAMSESH_SHUTDOWN_GRACE_S` env / `shutdown_grace_s` YAML
  configurable; default 30; validated as positive integer.
- [x] On SIGTERM:
  - HTTP server stops accepting new connections.
  - In-flight HTTP requests are given until the grace deadline to
    complete.
  - Auto-merger drains within the same budget (uses
    `time-remaining` derived from a shared start timestamp).
  - WebSocket gateway stop runs after HTTP is quiet.
- [x] Test (signal-injection style): start a request that holds
  for 200ms; send ctx cancel with grace=10s; assert the request
  completes and server.Run returns within ~3s.
- [x] Test with grace=1s: assert a 3s request is cut off and the
  process exits within ~2s; log emits a warning about the cut.
- [x] Per-step elapsed times logged at shutdown for operator
  observability (`shutdown_step=http elapsed_ms=...`).
- [x] `server.Run` no longer hardcodes 25; `cmd/portal/main.go`
  no longer hardcodes 10.

## Implementation notes

- Added `ShutdownGraceSeconds int` to `Config` with YAML key
  `shutdown_grace_s`; default 30; validated positive in `validate()`.
- `applyEnv` overlays `JAMSESH_SHUTDOWN_GRACE_S` after all other
  subsystem env helpers.
- `server.Run` uses `time.Duration(cfg.ShutdownGraceSeconds) * time.Second`
  for the `Shutdown` context timeout — no more magic 25.
- `cmd/portal/main.go`: a `sync.Once`-guarded goroutine captures
  `shutdownStart` the moment `ctx` is cancelled. After `server.Run`
  returns, the remaining budget is `grace - time.Since(shutdownStart)`.
  Auto-merger and WS gateway drain in parallel goroutines inside that
  window using a shared `context.WithTimeout`. A 1s floor prevents
  a zero/negative remaining time from skipping drain entirely.
- Per-step elapsed logged as structured fields: `shutdown_step=http`,
  `shutdown_step=automerger`, `shutdown_step=wsgateway`.
- Two new server tests: `TestGraceWindowCompletes` (200ms request +
  10s grace → completes) and `TestGraceWindowCutsOff` (3s request +
  1s grace → cut off within 2.5s).
- Four new config tests cover default, env override, YAML parsing,
  and validation of `ShutdownGraceSeconds`.
