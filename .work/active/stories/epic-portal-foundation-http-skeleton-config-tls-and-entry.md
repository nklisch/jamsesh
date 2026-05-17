---
id: epic-portal-foundation-http-skeleton-config-tls-and-entry
kind: story
stage: implementing
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
