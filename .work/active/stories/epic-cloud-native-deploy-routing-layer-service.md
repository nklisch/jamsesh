---
id: epic-cloud-native-deploy-routing-layer-service
kind: story
stage: review
tags: [infra]
parent: epic-cloud-native-deploy-routing-layer
depends_on: [epic-cloud-native-deploy-routing-layer-core, epic-cloud-native-deploy-routing-layer-hint-cache]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Reverse-proxy service + lifecycle

## Scope

Standalone `cmd/jamsesh-router/` binary with reverse-proxy `Handler`,
router-specific config, lifecycle (SIGTERM + graceful drain).

Implements **Unit 2** of `epic-cloud-native-deploy-routing-layer`.
Depends on the core (`extract` + `ring`) and `hint cache` from sibling
stories.

## Files

New:
- `cmd/jamsesh-router/main.go`
- `cmd/jamsesh-router/main_test.go`
- `internal/router/proxy/proxy.go`
- `internal/router/proxy/proxy_test.go`
- `internal/router/config/config.go`
- `internal/router/config/config_test.go`

## Acceptance criteria (per parent feature Unit 2)

- [ ] `cmd/jamsesh-router/main.go` parses config, builds ring + hint
  cache + proxy, starts HTTP server, handles SIGTERM with configurable
  grace
- [ ] REST request `/api/orgs/o/sessions/s/...` proxies to ring-chosen
  pod
- [ ] WS upgrade `/ws/sessions/s` proxies through (Upgrade echoed)
- [ ] Git request `/git/sessions/s.git/...` proxies through
- [ ] MCP request with `Jam-Session-Id: s` proxies through
- [ ] `/healthz` / `/readyz` / `/metrics` / `/auth/*` fall through to
  round-robin fallback
- [ ] 503 from pod → hint invalidate + single retry → propagate 503
- [ ] Graceful shutdown drains in-flight requests

## Notes

- Reverse proxy: `net/http/httputil.ReverseProxy` (handles WS upgrade
  natively when Director sets the right URL)
- Env-overlay pattern: `JAMSESH_ROUTER_<KEY>` mirroring portal config
  conventions
- Pod set is provided externally — config has `StaticPods` for tests;
  the actual Discoverer wiring is in Unit 3 / discovery story.
  Construct the proxy with an injected `*ring.Ring` whose contents are
  managed by callers.

## Implementation notes

Implementation work completed by the parallel orchestrator agent but the agent did NOT commit or advance the story (likely timed out mid-cleanup). Verified by the orchestrator and committed on the agent's behalf.

### What landed

- `cmd/jamsesh-router/main.go` (5.4KB) — binary entrypoint with config Load + Validate, ring + hint cache + discoverer wiring, http.Server lifecycle with graceful shutdown.
- `cmd/jamsesh-router/main_test.go` (11KB) — integration test with httptest backends; asserts requests reach the right backend based on session id; signal-injection style shutdown test.
- `internal/router/proxy/proxy.go` (8.9KB) — `Handler` with the full routing flow: extract → hint-cache lookup → ring fallback → reverse-proxy → 503-retry-with-skip → propagate.
- `internal/router/proxy/proxy_test.go` (13KB) — table-driven tests for REST / WS / Git / MCP route shapes plus 503-retry behavior.
- `internal/router/config/config.go` (6.3KB) — `Config` with the documented fields + YAML + env overlay (`JAMSESH_ROUTER_<KEY>`) + `Validate()`.
- `internal/router/config/config_test.go` (6.1KB) — covers default load, env override, YAML parsing, validation errors.
- `internal/router/ring/ring.go` — extended with `Ring.Pods()` (snapshot accessor for round-robin fallback) and `Ring.GetNext(key, skipID)` (used by the proxy's 503-retry path to find a different pod for the same key).

### Adjacent fix

Added `/jamsesh-router` to `.gitignore` (mirroring the existing `/portal` entry). The build binary was sitting untracked in the worktree root and would otherwise be staged accidentally.

### Verification

`go build ./...` clean. `go test -race ./internal/router/... ./cmd/jamsesh-router/...` passes all packages.

### Parked bug

The agent surfaced bug `bug-static-discoverer-empty-publish` while testing — `staticDiscoverer.Run` not publishing an initial empty pod set. The sibling `routing-layer-discovery` story (implemented in parallel) independently identified and fixed this with a `neverPublished` sentinel ("\x00"). The parked bug is therefore already resolved; archive on commit.
