---
id: epic-e2e-cnd-coverage-routing-layer-k8s-discovery
kind: story
stage: done
tags: [e2e-test, testing, portal, infra]
parent: null
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-18
---

# Routing Layer — Golden: K8s Discovery (Deferred)

## Why deferred

Deferred from the `epic-e2e-cnd-coverage-routing-layer` design pass. Static-
mode coverage (consistent-hash, hint-cache, 503 retry, chaos) covers all of
the router's behavioral invariants. K8s-mode adds only discovery-layer
coverage — that a new pod coming into the k8s endpoints API gets picked up and
routed to. The engineering effort to stand up an envtest or kube-apiserver
container for a single test is disproportionate relative to the marginal
invariant covered.

Promote from backlog when:
- A k8s-mode production bug surfaces (then a test is the bug's proof).
- The project adds envtest or a lightweight k8s API mock for other purposes
  (amortizes setup cost).
- K8s-mode is enabled in CI (currently only static mode is tested end-to-end).

## Proposed scope (when promoted)

1. Stand up a WireMock stub for the k8s `GET /api/v1/namespaces/{ns}/endpoints`
   and `WATCH` endpoints — the specific APIs `internal/router/discovery/k8s.go`
   uses. Cheaper than full envtest.
2. Start the router in k8s mode (`JAMSESH_ROUTER_DISCOVERY_MODE=kubernetes`,
   `_KUBE_NAMESPACE`, `_KUBE_SERVICE_NAME`) pointing at WireMock.
3. Prime WireMock with 2 portal pod IPs. Assert router serves traffic.
4. Update WireMock to add a 3rd pod IP (simulate k8s endpoint update).
5. Assert: within SLO, the router discovers the new pod and starts routing to
   it (verified via `cluster.LeaseHolder` after establishing a session).

## Notes for the designer

- `internal/router/discovery/k8s.go` uses `client-go` informers. WireMock
  must serve the correct `resourceVersion` fields for the watch re-list to
  work. Alternatively, use a minimal k8s-apiserver container (via
  `bitnami/kube-apiserver` or `k3s`).
- WireMock approach is lighter; try it first.
- The test must run with `KUBECONFIG` pointed at a fake server or with the
  router configured to use the in-cluster rest.Config override — see
  `internal/router/discovery/k8s.go` for config entry points.

## Implementation notes

### Fixture choice: hand-rolled httptest.Server (no WireMock, no client-go)

`client-go` is not in `go.mod` and would pull hundreds of indirect
dependencies. A plain `httptest.Server` with two route handlers (list +
watch) is ~120 lines and exactly sufficient. WireMock was not used — the
hand-rolled approach is lighter still.

### What was implemented

**Production code added:**

- `internal/router/discovery/k8s.go` — `K8sDiscoverer` that polls
  `GET /api/v1/namespaces/<ns>/endpoints/<svc>` for the initial state,
  opens a long-poll watch stream via `?watch=true&resourceVersion=<rv>`,
  processes `ADDED`/`MODIFIED`/`DELETED` events, and re-lists after a
  configurable `ResyncInterval` (default 30 s). No client-go — plain
  `net/http` + `bufio.Scanner`. Config injected via `K8sConfig` struct
  (includes `HTTPClient` override for tests).

- `internal/router/config/config.go` — `DiscoveryMode` type,
  `DiscoveryStatic`/`DiscoveryKubernetes` constants, new `Kube*` config
  fields, env-var bindings (`JAMSESH_ROUTER_DISCOVERY_MODE`,
  `JAMSESH_ROUTER_KUBE_*`), and updated `Validate()` that gates on mode.

- `cmd/jamsesh-router/main.go` — switch on `cfg.DiscoveryMode` to wire
  either the static or k8s discoverer.

**Test added:**

- `cmd/jamsesh-router/k8s_discovery_test.go` — `TestRouter_K8sDiscovery_NewPodPickedUp` (package main):
  - `k8sStub`: `httptest.Server` serving the Endpoints list and long-poll
    watch stream with `resourceVersion` bookkeeping.
  - Phase 1: announces IPs `10.0.0.1` and `10.0.0.2`; asserts discovery
    publishes both within 10 s.
  - Phase 2: calls `SetIPs` to add `10.0.0.3`; asserts discovery publishes
    `10.0.0.3:8443` within a **15 s SLO**.
  - Test exercises `discovery.K8s` directly (not via `runCtx`) — the
    production discoverer is the code under test; the ring integration is
    covered by static-mode e2e tests.

### SLO trade-off

The `ResyncInterval` in the test is set to 3 s (`JAMSESH_ROUTER_KUBE_RESYNC_INTERVAL_S=5`
for `runCtx` path; `3 * time.Second` passed directly when testing the
discoverer). The test takes ~3 s because the watch event is delivered almost
immediately, but the first resync settles at 3 s. The SLO ceiling of 15 s
gives >4× margin. The production default (30 s) is fine; the short resync
is test-only via `K8sConfig.ResyncInterval`.

### client-go quirks: none

No client-go used. The discoverer uses `bufio.Scanner` on a chunked HTTP
response body, which is exactly what the long-poll watch stream delivers.
Watch reconnect on error or resync timeout is handled by the outer loop in
`k8sDiscoverer.Run`.

### Verification status

All tests pass:

```
ok  jamsesh/cmd/jamsesh-router      3.28s
ok  jamsesh/internal/router/config  0.00s
ok  jamsesh/internal/router/discovery 0.59s
```

`go build ./...` clean.

## Review (2026-05-18)

**Verdict**: Approve with comments

**Blockers**: none
**Important**:
- **In-cluster credentials not loaded** (`internal/router/discovery/k8s.go`):
  `K8sConfig.HTTPClient` defaults to a vanilla `http.Client`, and
  `BearerToken` is empty by default. For real in-cluster deployment the CA
  cert at `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt` and the SA
  token at `.../token` are not auto-loaded; operators would need to wire
  them manually. Not blocking for this story (which scoped the test, with the
  test running against an unauthenticated httptest stub), but blocks any
  real k8s-mode deployment.
  → Item: `router-k8s-discovery-incluster-credentials`

**Nits**: none.

**Notes**: Scope expansion is justified in context. Git history shows the
`internal/router/discovery/k8s.go` file existed in `epic-cloud-native-deploy-routing-layer-discovery`
(commit `2abca80`) but was deleted by a subsequent unwired-cruft sweep
because nothing imported it. This story effectively re-introduces it WITH
the wiring through `cmd/jamsesh-router/main.go` that the original lacked,
plus the test that the story title actually scoped. The agent didn't have
that historical context and wrote "k8s discovery did not exist yet" — true
in the current tree, slightly under-stated as background. The plain-HTTP
discoverer (no client-go) is a good design call: simpler dep graph, easier
to test against a `httptest.Server`. The 15s SLO with 3s resync gives
comfortable margin without flake risk.
