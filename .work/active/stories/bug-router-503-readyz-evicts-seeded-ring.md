---
id: bug-router-503-readyz-evicts-seeded-ring
kind: story
stage: implementing
tags: [bug, router, discovery, e2e-chaos]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Bug: Router's first readyz probe evicts pre-seeded ring, causing startup 503

## Brief

Five chaos tests (`TestCrossPodClockSkew`, `TestHandoffUnderObjectStorageChaos`,
`TestHandoffUnderPodKill`, `TestLeaseHolderKilled`, `TestRouterPodDisappears`)
and the scaffolding keystone `TestClusteredSmoke` all fail with the same
symptom: `POST /api/orgs/{orgID}/sessions` through the router URL returns 503
with body "no backends available" (`tests/e2e/chaos/*_test.go` and
`tests/e2e/scaffolding/cluster_smoke_test.go`).

## Root cause

In `cmd/jamsesh-router/main.go` (static discovery path, lines 165–188), the
ring is seeded immediately with all configured pod addresses (lines 168–177),
then the discovery goroutine is started (lines 184–188). The discovery goroutine
calls `staticDiscoverer.Run`, which immediately fires a `doProbe()` pass on
startup (before the first `ticker.C` tick). `Probe.Check` sends HTTP GET
`/readyz` to every pod.

The portal fixture (`tests/e2e/fixtures/portal/portal.go:241–253`) waits for
the portal's `/healthz` endpoint to return 200 before returning from `Start`.
`/healthz` is a no-dependency instant responder (`internal/portal/router/router.go:131`).
`/readyz`, by contrast, runs two checks: a Postgres DB ping and an
`os.Stat(cfg.Storage)` (`cmd/portal/main.go:702–713`). If either check is
slow or fails at the moment the router's first probe fires (which can happen
in the race window between the portal's healthz passing and its readyz being
ready), all pods fail the readyz probe. When all pods fail, the discoverer
calls `publish([])` — clearing the ring that was just seeded. Subsequent
requests receive 503.

The probe fires immediately on goroutine start (`static.go:45`: `doProbe()`
before the ticker loop), and the router fixture (`tests/e2e/fixtures/router/router.go`)
waits only for the router's own `/metrics` endpoint — not for the portal
backends' `/readyz`. There is no retry window and no delay between ring-seed
and first probe.

**This is the primary cause of every chaos-test 503, and of `TestClusteredSmoke`'s
scaffolding failure. Cluster D (`FAIL jamsesh/tests/e2e/scaffolding`) is
downstream of this same bug.**

## Fix site

Two complementary fixes, either is sufficient but both together are most robust:

**Fix A — router startup: delay first probe until `ProbeInterval` elapses**
`internal/router/discovery/static.go:Run` — move `doProbe()` from the
pre-ticker position (line 45) into the ticker loop, so the first probe does
not fire until `ProbeInterval` (default 5s) after startup. The ring is already
seeded from static config before the discovery goroutine starts, so skipping
the immediate probe is safe: the seed is the authoritative initial state.

**Fix B — router fixture: wait for portal backends to be readyz before starting router**
`tests/e2e/fixtures/router/router.go:Start` — before starting the router
container, poll each backend's `/readyz` endpoint (using the mapped host port)
and wait until all return 200. This guarantees that when the router's discovery
loop fires its first probe, the portals are genuinely ready.

Fix A is the production-correctness fix. Fix B is the test-hardening fix. Both
should land together.

## File:line pointers

- `internal/router/discovery/static.go:44-45` — `doProbe()` called immediately
  before the ticker loop (remove or guard this call)
- `cmd/jamsesh-router/main.go:165-181` — static ring seed; the seeded ring is
  correct, the problem is the discovery loop clearing it
- `tests/e2e/fixtures/router/router.go:90-101` — `WaitingFor` waits for the
  router's own `/metrics`, not for backend readyz; add backend-readyz poll here
  or in `portalcluster.Start` before calling `router.Start`
- `tests/e2e/fixtures/portal/portal.go:247` — fixture waits for `/healthz`;
  should either also wait for `/readyz` or callers must delay the router start

## Acceptance criteria

- [ ] The router's static discoverer does NOT call `publish([])` on startup when
      all portals are healthy (i.e. readyz probe wins the race).
- [ ] `TestClusteredSmoke` in `tests/e2e/scaffolding` passes end-to-end.
- [ ] `TestCrossPodClockSkew` no longer fails at `createLeaseSkewSession: want 201; got 503`.
- [ ] All five chaos tests reach their actual chaos scenario (not blocked on 503 at session creation).
- [ ] `go test ./internal/router/... -timeout 60s` passes.
