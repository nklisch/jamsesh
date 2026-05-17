---
id: epic-portal-foundation-http-skeleton-config-tls-and-entry
kind: story
stage: done
tags: [portal]
parent: epic-portal-foundation-http-skeleton
depends_on: [epic-portal-foundation-http-skeleton-router-and-middleware]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# HTTP Skeleton — Config, TLS, and Portal Entry

## Scope

Add the configuration loader, server lifecycle helper, and the portal
binary entry point. After this story, `go build ./cmd/portal && ./portal`
brings up a TLS-terminating (or proxied) HTTP server with healthcheck
and graceful shutdown.

## Units delivered

- **Unit 9**: `internal/portal/config/config.go` — `Config`,
  `TLSConfig`, `LogConfig`, `Load(path)`, defaults, env overlay,
  validation
- **Unit 10**: `internal/portal/server/server.go` — `Run(ctx, cfg,
  handler) error` handling both TLS modes and graceful shutdown
- **Unit 11**: `cmd/portal/main.go` — flag-parses `--config`, loads
  config, wires logging, builds router with nil mount hooks, runs
  server until SIGINT/SIGTERM

## go.mod additions

This story adds `gopkg.in/yaml.v3` and consumes the chi-router
package added by the router story (already in go.mod).

## Acceptance Criteria

- [ ] `config.Load("")` returns defaults matching the parent feature's
      Unit 9 design
- [ ] `config.Load(path)` reads YAML; env vars override file values
- [ ] `tls.mode = native` without cert/key paths fails validation
- [ ] `tls.mode = behind_proxy` runs without TLS material
- [ ] `db_driver` must be `"sqlite"` or `"postgres"`; anything else
      fails validation
- [ ] `cmd/portal` builds without errors
- [ ] Smoke test: start with `JAMSESH_BIND=127.0.0.1:0 JAMSESH_TLS_MODE=behind_proxy`,
      send `GET /healthz`, send SIGTERM, exit cleanly within 25s
- [ ] All unit tests green

## Notes

- Uses `signal.NotifyContext` for SIGINT/SIGTERM handling — the
  cleanest stdlib pattern (Go 1.16+).
- Server lifecycle returns nil on graceful shutdown
  (`http.ErrServerClosed`), error otherwise — callers check the
  return value to decide exit code.
- The portal binary intentionally serves only `/healthz` until
  sibling features wire `router.Deps.MountAPI` etc. This is the
  late-binding shape that lets features ship independently.

## Implementation notes

### Landed files

- `internal/portal/config/config.go` — `Config`, `TLSConfig`,
  `LogConfig`, `Load(path)`, `defaults()`, `validate()`, `applyEnv()`
- `internal/portal/config/config_test.go` — 13 tests covering defaults,
  YAML load, env overlay, all validation paths
- `internal/portal/server/server.go` — `Run(ctx, cfg, handler) error`
  handling both TLS modes and graceful shutdown (25 s drain budget)
- `internal/portal/server/server_test.go` — in-process smoke test
  (ephemeral port, /healthz poll, context cancel, clean return);
  listen-error propagation test
- `cmd/portal/main.go` — `--config` flag, loads config, wires slog
  via `logging.Setup`, builds router with nil mount hooks, runs server
  until SIGINT/SIGTERM

### go.mod

`gopkg.in/yaml.v3` promoted from indirect to direct dependency.
Version remains v3.0.1 (already present; `go get` confirmed no newer
stable version changed the pin).

### Config defaults vs docs/SELF_HOST.md

All defaults in `config.defaults()` match the Configuration table:

| Field      | Default          |
|------------|-----------------|
| bind       | `:8443`          |
| db_driver  | `sqlite`         |
| db_dsn     | `./jamsesh.db`   |
| tls.mode   | `behind_proxy`   |
| log.format | `json`           |
| log.level  | `0` (INFO)       |
| storage    | `./storage`      |

### slog.Level YAML handling

`slog.Level` is an int alias; yaml.v3's default decode path routes
through `slog.Level.UnmarshalText` which expects level name strings
("DEBUG", "INFO", etc.) — numeric strings like `"-4"` produce an error.

Fix: `LogConfig.UnmarshalYAML` is added. It tries `strconv.Atoi` first
(numeric integer → slog.Level), then falls back to
`slog.Level.UnmarshalText` for name strings. This supports both:

  ```yaml
  log.level: -4       # integer → DEBUG
  log.level: "DEBUG"  # name string → DEBUG
  ```

Env-var path (`JAMSESH_LOG_LEVEL`) already used `strconv.Atoi`
(integer-only, per design). Documented in package doc.

### Smoke test approach

`server_test.go:TestGracefulShutdown` (in-process, no binary needed):
1. Find ephemeral port via `net.Listen("tcp","127.0.0.1:0")`.
2. Start `server.Run` in a goroutine with cancellable context.
3. Poll `GET /healthz` with 50 ms backoff until 200 or 5 s deadline.
4. Verify response body `{"status":"ok"}`.
5. Cancel context → `server.Run` calls `srv.Shutdown`.
6. Assert `Run` returns nil within 1 s.

### Deviations

None from the feature spec. The `LogConfig.UnmarshalYAML` addition is an
improvement over the int-only YAML approach in the sketch — it handles
both numeric and name-string levels without breaking the env-var path.

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Config loading is clean: defaults match SELF_HOST.md table exactly, env overlay precedence matches the design, validation rejects bad TLS modes and unknown DB drivers. The LogConfig.UnmarshalYAML addition that accepts both integer and string slog levels is a real ergonomic improvement over the int-only design. Server lifecycle exits nil on clean shutdown and propagates errors otherwise — exactly the contract callers need. Smoke test exercises the full startup→healthz→SIGTERM cycle in-process.
