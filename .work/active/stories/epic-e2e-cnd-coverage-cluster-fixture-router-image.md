---
id: epic-e2e-cnd-coverage-cluster-fixture-router-image
kind: story
stage: review
tags: [e2e-test, testing, infra]
parent: epic-e2e-cnd-coverage-cluster-fixture
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Router image + fixture

## Scope

Three deliverables:

1. **`Dockerfile.router`** — alpine:3.21 + ca-certificates, copies the
   pre-built `jamsesh-router-linux-amd64` binary, exposes :8080, runs as
   `nobody`. Mirrors the existing `Dockerfile` for the portal (no git
   needed for the router).

2. **`Makefile` targets** — `test-router-image` (build) and
   `test-router-image-clean` (rmi). Mirrors the existing
   `test-portal-image` / `test-portal-image-clean` shape exactly.

3. **`tests/e2e/fixtures/router/`** — Testcontainers fixture exposing
   `Start(ctx, t, Options{Backends []string, HintCacheTTL time.Duration})
   *Router` returning a `Router{URL, ContainerURL, ...}`. The fixture
   configures the router via env vars (`JAMSESH_ROUTER_BIND=:8080`,
   `JAMSESH_ROUTER_DISCOVERY_MODE=static`, `JAMSESH_ROUTER_STATIC_PODS=...`,
   `JAMSESH_ROUTER_SHUTDOWN_GRACE_S=5`). Waits for `/metrics` 200.

4. **CI workflow update** — `.github/workflows/e2e.yml` gets a `build
   router test image` step (`run: make test-router-image`) inserted
   between the existing portal-image and e2e-suite steps.

5. **Self-test** — uses a small `nginx:alpine` Testcontainer as a stub
   backend; router points at the nginx container's bridge IP; assert the
   stub's static response body is returned through the router URL.

## Files

- `Dockerfile.router` (new)
- `Makefile` (edit; add `test-router-image` and `test-router-image-clean`
  targets to `.PHONY`)
- `.github/workflows/e2e.yml` (edit; add build step)
- `tests/e2e/fixtures/router/router.go`
- `tests/e2e/fixtures/router/router_test.go`

## Acceptance criteria

- [ ] `make test-router-image` produces `jamsesh/router:e2e` tag
- [ ] `make test-router-image-clean` removes it
- [ ] `Dockerfile.router` builds CGO-free static binary; runs as `nobody`
- [ ] Router fixture's `Start` returns within 30s
- [ ] Wait strategy uses `/metrics` 200 (router exposes it unconditionally
      per `cmd/jamsesh-router/main.go:145`)
- [ ] Self-test asserts router proxies a real backend response (nginx
      stub container) — not a mock
- [ ] Test skips cleanly when Docker is unavailable
- [ ] Test skips cleanly with actionable message when `jamsesh/router:e2e`
      is absent (mirror of `requirePortalImage`)
- [ ] CI workflow runs `make test-router-image` before `make test-e2e`
- [ ] `go test ./fixtures/router/...` is green from the `tests/e2e/` module

## Test integrity (from parent epic)

Self-test asserts on a real reverse-proxy round-trip against a real
backend (nginx). Not tautological.

If the router exhibits unexpected behavior under the simple stub setup
(e.g., panics, hangs on shutdown), park as bug and reference the backlog
id in the test. Do not silence.

## References

- `cmd/jamsesh-router/main.go` — env-var surface, default port, signal
  handling
- `Dockerfile` — portal image to mirror (alpine 3.21, USER nobody pattern)
- `Makefile:82-89` — `test-portal-image` shape to mirror
- `.github/workflows/e2e.yml` — existing CI flow

## Dependencies on this story (downstream)

- `epic-e2e-cnd-coverage-cluster-fixture-portalcluster` (uses router when
  `Options.Router: true`)
- Eventually consumed by every test in
  `epic-e2e-cnd-coverage-routing-layer`

## Implementation notes

### Files touched

- `Dockerfile.router` (new) — alpine:3.21, `ca-certificates`, copies
  `jamsesh-router-${TARGETOS}-${TARGETARCH}` binary, exposes 8080, runs as
  `nobody`. No `git` package (router doesn't shell out).
- `Makefile` — added `test-router-image` and `test-router-image-clean`
  targets immediately after the parallel portal targets; added both to the
  relevant `.PHONY` line.
- `.github/workflows/e2e.yml` — inserted `build router test image` step
  (`make test-router-image`) between `build portal test image` and
  `run e2e suite`.
- `tests/e2e/fixtures/router/router.go` — `Start(ctx, t, Options) *Router`
  fixture; `Router{URL, ContainerURL}` type; `requireDocker` /
  `requireRouterImage` skip helpers; cleanup via
  `containerlog.DumpAndTerminate`.
- `tests/e2e/fixtures/router/router_test.go` — `TestRouterProxy`: nginx:alpine
  stub backend (real container), router pointed at bridge IP, assert
  `"Welcome to nginx"` in response body. Ran green in 5.65s.

### Env-var bindings used

| Env var | Value |
|---|---|
| `JAMSESH_ROUTER_BIND` | `:8080` |
| `JAMSESH_ROUTER_DISCOVERY_MODE` | `static` |
| `JAMSESH_ROUTER_STATIC_PODS` | comma-joined `opts.Backends` |
| `JAMSESH_ROUTER_SHUTDOWN_GRACE_S` | `5` |

### HintCacheTTL — not configurable via fixture

`HintCacheTTL` is documented as YAML-only in
`internal/router/config/config.go` (comment: "Remaining knobs —
YAML-only in v1"). `cmd/jamsesh-router/main.go` `printUsage` confirms no
corresponding env var. `Options.HintCacheTTL` was omitted from the fixture
API — the default (60s) applies. If tests need a shorter TTL, a follow-on
story can add config-file mount support to the fixture.

### Wait strategy

`/metrics` HTTP 200 on port `8080/tcp` with 30s startup timeout, per
`cmd/jamsesh-router/main.go:145` which registers Prometheus metrics
unconditionally. The wait resolved immediately after container start
(confirmed in test output).

### Verified

- `go build ./fixtures/router/...` — clean
- `go vet ./fixtures/router/...` — clean
- `make test-router-image` — produced `jamsesh/router:e2e`
- `go test -run TestRouterProxy ./fixtures/router/... -v -timeout 60s` — PASS (5.65s)
