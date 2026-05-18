---
id: epic-e2e-cnd-coverage-routing-layer
kind: feature
stage: drafting
tags: [e2e-test, testing, portal, infra]
parent: epic-e2e-cnd-coverage
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E CND Coverage — Routing Layer

## Brief

`epic-cloud-native-deploy-routing-layer` shipped `cmd/jamsesh-router/`,
a standalone Go consistent-hashing reverse proxy with a soft-coordinator
hint cache (which pod recently acquired session X). It supports k8s and
static pod discovery, dispatches MCP requests by `Mcp-Session-Id`
header, and re-dispatches on 503/Retry-After from a backend.

Coverage today: zero. No router binary fixture; no consistent-hash test;
no `Mcp-Session-Id`-based routing test. The 503/Retry-After matches in
the existing failure suite (`tests/e2e/failure/config_and_deps_test.go:
517,662,824`) cover dep failures (SMTP, DB, OAuth) — completely unrelated
to lease-acquisition routing.

This feature's externally-visible failure mode matters because a router
that drops 503s rather than re-dispatching produces silent corruption
from the user's perspective: the client thinks the session is permanently
unavailable, while a different pod would have served it cleanly.

## Audit findings addressed

- **F4 (Critical, journey-gap, all four taxonomy layers)** — Routing-
  layer has zero coverage. No router binary fixture, no consistent-hash
  test, no 503/Retry-After re-dispatch test, no MCP-header routing test.

(The router fixture itself — image build + Testcontainers wrapper —
is in `epic-e2e-cnd-coverage-cluster-fixture`. This feature owns the
test bodies and any router-specific helper utilities.)

## Scope

### Tests to add

1. **`tests/e2e/golden/router_consistent_hash_test.go`**
   - Spin up cluster + router. Issue many requests for the same
     session_id; assert all land on the same backend pod (verified via
     a test header or per-pod response signature).
   - Issue requests for many different session_ids; assert reasonable
     distribution across pods (not perfect — consistent hashing has bias
     — but no pod left idle, no single pod taking everything).
   - **Invariant**: "Same session_id consistently routes to the same
     pod absent re-ring events."

2. **`tests/e2e/golden/router_mcp_session_header_test.go`**
   - MCP handshake on the router URL gets a session ID. Subsequent MCP
     tool calls with that `Mcp-Session-Id` header pin to the same backend
     as the handshake. Asserts on backend identity per request.
   - Reuse existing `mcpclient` fixture.

3. **`tests/e2e/golden/router_hint_cache_test.go`**
   - The soft-coordinator hint cache: after pod B acquires a session
     that the consistent-hash ring would have routed to pod A, the
     router learns this and routes subsequent requests to pod B
     directly. Test:
     - Pre-create lease on pod B for a session that hashes to pod A.
     - Request via router. Assert it reaches pod B without bouncing
       through pod A.
     - Vary session IDs to confirm hint cache distinguishes per session,
       not blanket-replacing the ring.

4. **`tests/e2e/failure/router_lease_unavailable_test.go`**
   - Backend pod returns 503 with `Retry-After` (because lease is held
     elsewhere). Router re-dispatches transparently to the holder.
     Client sees a single eventual 2xx, never a 503.
   - Subtest `repeated_503_eventually_surfaces` — if all backends 503
     persistently (e.g., Postgres unreachable for all pods), router
     stops re-dispatching after bounded attempts and surfaces an error
     to the client. No infinite-retry pathology.

5. **`tests/e2e/failure/router_backend_dead_test.go`**
   - One backend pod is killed (Pumba). The router's discovery should
     remove it (in static mode after health-check failure; in k8s
     mode the discovery callback should trigger). New requests for
     sessions that hash to the dead pod re-shard to a surviving pod
     within an SLO.

6. **`tests/e2e/chaos/router_pod_disappears_test.go`**
   - Toxiproxy disconnect between router and one backend pod
     mid-request. Router fails over to another pod within SLO.
     Client perceives a clean response, possibly after a brief retry.
   - Subtest variant: Toxiproxy latency (3-5s) on router→backend;
     router uses configured backend-timeout to fail over rather than
     hanging the client.

7. **`tests/e2e/golden/router_k8s_discovery_test.go`** (optional /
   nice-to-have)
   - If the router supports k8s discovery against a mock k8s API
     (envtest, kube-apiserver in a container, or a WireMock stubbing
     just the endpoints/pods APIs the router uses), prove a new pod
     coming into the discovery API gets picked up and routed to.
   - This may be deferred to a follow-up if envtest setup is heavy
     for one test — design pass decides.

### Helpers

- A "which pod served this" inspection helper. Options:
  - Custom test-only HTTP response header from the portal naming itself
    (build-tag-gated).
  - Direct introspection of portal container logs to correlate request
    IDs with served pod.
  Design pass picks based on what's cleanest given existing log/trace
  infrastructure.
- A backend pod-health-fault injector (have a pod return 503 to
  `/healthz` so the router's static-mode health check fails it).
  Lives in `tests/e2e/fixtures/portalcluster/`.

## Mock-boundary plan

| External dep                  | Service-level mock                | Notes |
|-------------------------------|-----------------------------------|-------|
| Router binary                 | Real `cmd/jamsesh-router/`        | From cluster-fixture |
| Multiple portals              | Real portals from cluster-fixture | Reuse |
| Network partition             | Toxiproxy                         | Existing chaos pattern |
| Pod kill                      | Pumba                             | Existing chaos pattern |
| K8s API (if covered)          | envtest OR Testcontainers kube-apiserver OR WireMock | Off-the-shelf; design picks |

No in-process router mock. The router IS the system under test for this
feature.

## Open questions for design

- **"Which backend served this request" mechanism.** Build-tag-gated
  response header from the portal? Request-ID correlation through logs?
  Or a router-side debug endpoint? Resolve in design pass; lean toward
  the response-header path (simplest, least flaky).
- **Hint-cache invalidation semantics.** Does the router invalidate the
  hint on backend 503 (the backend is telling it "I don't hold this
  lease anymore"), or only on time-based expiry, or both? Test design
  depends on this. Resolve in design pass by reading the documented
  semantics from `docs/ARCHITECTURE.md` or `docs/SELF_HOST.md` (without
  reading source).
- **K8s discovery test scope.** Full envtest, lightweight kube-apiserver
  container, or WireMock stubbing the specific endpoints?
  Engineering-effort decision; design pass picks based on how much value
  k8s-mode-specific coverage delivers vs. static-mode coverage.
- **Bounded-retry pathology test.** What's the documented max-retry value
  for the router? Test needs to know to assert "stops re-dispatching
  after N attempts".

## Acceptance criteria

- [ ] `router_consistent_hash_test.go` green; same session pins to same
      backend, different sessions distribute
- [ ] `router_mcp_session_header_test.go` green; MCP-header routing
      identity preserved across handshake + tool calls
- [ ] `router_hint_cache_test.go` green; soft-coordinator hint overrides
      consistent-hash placement when lease is elsewhere
- [ ] `router_lease_unavailable_test.go` green; transparent re-dispatch +
      bounded-retry pathology guard
- [ ] `router_backend_dead_test.go` green; dead backend evicted from
      routing pool within SLO
- [ ] `router_pod_disappears_test.go` green; chaos disconnect + chaos
      latency both produce clean failover
- [ ] (optional) `router_k8s_discovery_test.go` green if not deferred
- [ ] "Which backend served this" inspection mechanism landed in fixtures
- [ ] No in-process router or k8s-API mocks introduced

## Test integrity (from parent epic)

- A router test that asserts "request returned 2xx" without verifying
  which backend served it is a tautology — the consistent-hash invariant
  is about routing, not response status. **Every routing-identity test
  must inspect backend identity.**
- If the bounded-retry pathology test surfaces an infinite-retry bug
  (router never gives up), park via `/agile-workflow:park`; don't
  loosen the assertion to "eventually succeeds".
- The hint-cache test may surface a real bug if the cache invalidation
  is broken (router keeps routing to a pod that's released the lease).
  Park if surfaced.

## Non-goals

- TLS termination at the router (separate operational concern; out of
  CND scope)
- Authentication at the router (router is a topology component, not an
  auth boundary)
- Long-haul connection-pool perf characterization
- Multi-region router federation

## Next

`/agile-workflow:e2e-test-design epic-e2e-cnd-coverage-routing-layer`
