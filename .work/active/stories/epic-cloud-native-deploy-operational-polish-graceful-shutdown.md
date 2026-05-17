---
id: epic-cloud-native-deploy-operational-polish-graceful-shutdown
kind: story
stage: implementing
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
- [ ] `server.Run` no longer hardcodes 25; `cmd/portal/main.go`
  no longer hardcodes 10.
