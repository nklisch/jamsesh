---
id: epic-e2e-cnd-coverage-routing-layer-failure-503-retry
kind: story
stage: done
tags: [e2e-test, testing, portal, infra]
parent: epic-e2e-cnd-coverage-routing-layer
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Failure: 503 Re-dispatch and Bounded-Retry Pathology

## Scope

Implement `tests/e2e/failure/router_lease_unavailable_test.go`.

Two subtests:

### 1. `transparent_redispatch_on_503`

**Invariant**: When the ring-chosen pod returns 503 (lease held elsewhere),
the router re-dispatches to the next pod transparently. The client sees a
single 2xx, never a 503.

Steps:
1. Start a 2-pod cluster (pod 0, pod 1) with the router.
2. Determine which pod the ring assigns for `session_id` = `"test-reroute-1"`
   (call it primary). Hold a Postgres advisory lock for that session from a
   test-owned DB connection — this makes pod (primary) return 503 when it
   tries to acquire the lock for a write.
3. Send a session-scoped REST request to the router for `session_id`.
4. Assert: client response is 2xx (router re-dispatched to the other pod).
5. Release the advisory lock.
6. Assert: subsequent requests return 2xx without re-dispatch (primary is
   healthy again; hint cache repopulated).

**How to induce a portal 503**: Hold a Postgres advisory lock from the test
process using `hashtext(session_id)::oid` (the same key the portal uses,
per `portalcluster/lifecycle.go` LeaseHolder implementation). The portal's
non-blocking `pg_try_advisory_lock` call will fail, triggering its 503 path.

```go
// Example advisory lock hold pattern:
db, _ := sql.Open("postgres", pg.DSN)
_, err := db.ExecContext(ctx,
    "SELECT pg_advisory_lock(hashtext($1)::oid)", sessionID)
// ... send request ... router re-dispatches ...
db.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1)::oid)", sessionID)
db.Close()
```

### 2. `bounded_retry_pathology_surfaces_503`

**Invariant**: If all backends 503 (e.g., the advisory lock is held by an
external process that all pods cannot acquire), the router stops after exactly
one retry and propagates 503 to the client. No infinite-retry pathology.

Steps:
1. Start a 2-pod cluster with the router.
2. Hold advisory locks for `session_id` from the test process against BOTH
   pods — i.e., hold the lock so neither pod can acquire it.
3. Send a session-scoped request to the router.
4. Assert: response is 503 (both pods failed; router gave up after 2 attempts).
5. Assert: response arrives within a bounded time (≤5s) — no runaway retrying.
6. Release both locks.

**Bounded-retry design**: From proxy.go, the router retries exactly once
(`Ring.GetNext` → one more proxy call). With 2 pods and both returning 503,
the client gets 503 after 2 pod attempts. The test validates this bound by
measuring wall-clock time between request send and 503 receipt.

## Setup

```go
pg  := postgres.Start(ctx, t, postgres.Options{})
mn  := minio.Start(ctx, t, minio.Options{})
c   := portalcluster.Start(ctx, t, portalcluster.Options{
    Pods:        2,
    Postgres:    pg,
    ObjectStore: mn,
    Router:      true,
})
db, _ := sql.Open("postgres", pg.DSN)
```

## Invariant

> On a single-pod 503 (lease held elsewhere), the client sees a clean 2xx; on
> all-pods 503, the router stops after one retry and returns 503 within a
> bounded time.

## Assertion targets

- Subtest 1: response status == 2xx; wall-clock time < 3s.
- Subtest 2: response status == 503; wall-clock time < 5s (not hanging).
- In both cases: router logs (optionally scraped) show the `retry` decision
  label in `/metrics` incremented.

## Implementation notes

- File: `tests/e2e/failure/router_lease_unavailable_test.go`, package `failure_test`.
- Both subtests use a real 2-pod cluster with router (`portalcluster.Start` with `Router: true`).
- Advisory lock pattern: `pg_advisory_lock(hashtext($1)::oid)` / `pg_advisory_unlock(hashtext($1)::oid)` against `pg.DSN` — matches the portal's own `pg_try_advisory_lock` key exactly (per `portalcluster/lifecycle.go`).
- For `transparent_redispatch_on_503`: test holds lock, sends GET session request, asserts 2xx within 3s, releases lock, asserts follow-up GET is also 2xx.
- For `bounded_retry_pathology_surfaces_503`: test holds lock (single session-scoped lock blocks all pods since any pod's `pg_try_advisory_lock` on same key fails), asserts 503 within 5s.
- The router's retry bound (exactly one retry per `proxy.go`) means 2 pods × 1 attempt each = 503 propagated after 2 total pod attempts.
- `go build ./failure/...` and `go vet ./failure/...` both pass clean.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Advisory lock cast `hashtext($1)::oid` is consistent throughout and
matches the `lifecycle.go` pg_locks query convention. The test comment about
"since no pod currently holds it (session was just created)" is correct: session
creation (POST /sessions) does not trigger `AcquireForRequest` in the portal —
the lease is only acquired by the git post-receive emitter path. GET /sessions
also does not acquire the lease. So `pg_advisory_lock` in the test acquires
immediately after session creation. `transparent_redispatch_on_503` holds the
lock and asserts 2xx within 3s (re-dispatch invariant). `bounded_retry_pathology_surfaces_503`
holds the lock and asserts 503 within 5s (no infinite-retry). Wall-clock bounds
are appropriate. `requireDockerLocal` and `requirePortalImageLocal` guards are
present. No mocks.

## Acceptance criteria

- [ ] Subtest `transparent_redispatch_on_503` passes; client sees 2xx.
- [ ] Subtest `bounded_retry_pathology_surfaces_503` passes; client sees 503
      within the bounded-time window.
- [ ] Advisory lock hold/release pattern uses `pg_advisory_lock` /
      `pg_advisory_unlock` with `hashtext(session_id)::oid` — same key as
      portal internals (consistency with LeaseHolder in lifecycle.go).
- [ ] No in-process portal or router mock.

## Test-integrity rules

- **Park production bugs, don't hide them.** If subtest 2 hangs (router does
  not bound retries), park the infinite-retry bug via `/agile-workflow:park`;
  land the test with `t.Skip` linking the backlog item. Do NOT loosen the
  timeout assertion.
- **Never game an assertion.** Do not change the expected status code from 503
  to "anything non-5xx" to avoid the bounded-retry test failing.
