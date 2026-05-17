---
id: epic-cloud-native-deploy-routing-layer-service
kind: story
stage: implementing
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
