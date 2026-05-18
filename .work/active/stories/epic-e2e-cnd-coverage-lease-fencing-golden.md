---
id: epic-e2e-cnd-coverage-lease-fencing-golden
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage-lease-fencing
depends_on: [epic-e2e-cnd-coverage-lease-fencing-infra]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E CND Lease Fencing — Golden Path

## Scope

Implement `tests/e2e/golden/lease_acquire_and_fence_test.go` covering
the three golden-path subtests defined in the feature brief.

## Implementation unit

**File:** `tests/e2e/golden/lease_acquire_and_fence_test.go`

**Stack:** 2-pod cluster (`portalcluster.Start`), shared Postgres, shared
MinIO. `Router: true` for all subtests so pushes go through the router (the
lease path is exercised only under clustered mode).

**Invariants (one per subtest):**

1. `single_pod_acquires_lease_for_session` — after a session push via the
   router to pod A, exactly one pod holds the advisory lock for that session
   (verified by `c.RequireLeaseHolder`) and the lease row in Postgres has a
   fencing token > 0.
2. `two_pods_race_acquire_only_one_wins` — when pod A holds a session lease
   and a request for the same session arrives directly at pod B (bypassing the
   router), pod B returns 503. The `error` field in the JSON body identifies
   the contention (assert on 503 status; log the error code; if the error code
   is not `lease.held_elsewhere` or an equivalent documented code, file a
   backlog item and `t.Skip` the code-assertion portion, but keep the 503
   status assertion).
3. `monotonic_fencing_tokens_across_acquisitions` — after kill of pod A
   (releasing its PG connection and thus the advisory lock), pod B acquires the
   session, and `c.FencingTokenForSession` returns a token strictly greater
   than the token from the first acquisition.

**Scaffold:**

```go
package golden_test

import (
    "context"
    "net/http"
    "testing"
    "time"

    "jamsesh/tests/e2e/fixtures/authflow"
    "jamsesh/tests/e2e/fixtures/mailhog"
    "jamsesh/tests/e2e/fixtures/minio"
    "jamsesh/tests/e2e/fixtures/portalcluster"
    "jamsesh/tests/e2e/fixtures/postgres"
)

func TestLeaseAcquireAndFence(t *testing.T) {
    t.Run("single_pod_acquires_lease_for_session", testSinglePodAcquiresLease)
    t.Run("two_pods_race_acquire_only_one_wins", testTwoPodsRaceAcquire)
    t.Run("monotonic_fencing_tokens_across_acquisitions", testMonotonicFencingTokens)
}

// testSinglePodAcquiresLease:
// Invariant: after a session push via the router, exactly one pod holds
// the advisory lock and the fencing token in the leases row is > 0.
func testSinglePodAcquiresLease(t *testing.T) {
    t.Helper()
    ctx := context.Background()

    pg := postgres.Start(ctx, t, postgres.Options{})
    mn := minio.Start(ctx, t, minio.Options{})
    mh := mailhog.Start(ctx, t)
    c := portalcluster.Start(ctx, t, portalcluster.Options{
        Pods: 2, Postgres: pg, ObjectStore: mn, Router: true,
        PortalExtraEnv: map[string]string{
            // Short heartbeat so lease state settles quickly.
            "JAMSESH_LEASE_HEARTBEAT_INTERVAL_S": "2",
        },
    })

    alice := authflow.SignInViaMagicLink(ctx, t, c.Pods[0], mh,
        leaseFenceEmail(t, "alice-single"))
    orgID := authflow.CreateOrg(ctx, t, c.Pods[0], alice.AccessToken, "Lease Golden Org")
    sessionID := createLeaseFenceSession(ctx, t, c.Pods[0], alice.AccessToken, orgID,
        "lease-golden-single")

    // Push via router to establish the lease on whichever pod handles it.
    pushViaRouter(ctx, t, c.RouterURL, orgID, sessionID, alice)

    // Assert: advisory lock held by exactly one pod.
    holder := c.RequireLeaseHolder(ctx, t, sessionID, 10*time.Second)
    if holder < 0 || holder >= len(c.Pods) {
        t.Fatalf("no valid pod holds the lease after push")
    }

    // Assert: fencing token in leases table > 0.
    token := c.FencingTokenForSession(ctx, t, sessionID)
    if token <= 0 {
        // Token of 0 is the NoopManager sentinel — only valid in single-instance mode.
        // In clustered mode the Postgres sequence must issue token >= 1.
        t.Fatalf("fencing token %d <= 0 in leases row (clustered mode must issue >0)", token)
    }
}

// testTwoPodsRaceAcquire:
// Invariant: when pod A holds a session lease, a direct-pod request to pod B
// for the same session returns 503.
func testTwoPodsRaceAcquire(t *testing.T) {
    // ... (establish session on pod 0, then make direct HTTP request to pod 1
    // and assert 503; document expected error code in comment)
}

// testMonotonicFencingTokens:
// Invariant: after pod A is killed (releasing its PG connection / advisory
// lock), pod B re-acquires the session with a strictly higher fencing token.
func testMonotonicFencingTokens(t *testing.T) {
    // ... (push to pod 0, record token T1; Kill(0); push to pod 1, record
    // token T2; assert T2 > T1)
}
```

**Assertion targets:**
- `c.RequireLeaseHolder` returns a valid pod index (>= 0)
- `c.FencingTokenForSession` returns an `int64` > 0 for first acquisition
- HTTP status 503 for the second-pod direct request
- `T2 > T1` for the monotonicity check (both from `c.FencingTokenForSession`)

**Setup:** 2-pod cluster + Postgres + MinIO + MailHog. Short heartbeat
(`JAMSESH_LEASE_HEARTBEAT_INTERVAL_S=2`) so lease state settles in test time.

**Teardown:** Testcontainers cleanup via `t.Cleanup` (automatic from fixture).

## Acceptance criteria

- [ ] `TestLeaseAcquireAndFence` has all three subtests.
- [ ] Each subtest has a single-line invariant comment above it.
- [ ] Assertions target Postgres state (`pg_locks` via `LeaseHolder`,
      `leases` table via `FencingTokenForSession`) and HTTP response status.
- [ ] No in-process mocks. No assertions on internal call traces.
- [ ] Tests pass green in CI when clustered mode is fully functional.

## Test integrity

**Park production bugs, don't hide them.** If `testTwoPodsRaceAcquire`
finds that pod B returns 200 (not 503) when pod A holds the lease, that is
a split-brain bug in the portal. Do NOT adjust the assertion — park it via
`/agile-workflow:park`, land the test with `t.Skip("<backlog-id>: pod B
does not reject session already held by pod A")`.

**Error code assertion:** if the error code on the 503 from pod B is not the
documented `lease.held_elsewhere` or equivalent, add a `t.Logf` noting the
actual code AND file a follow-on story to align the error code with PROTOCOL.md.
The 503 status assertion is the safety-critical one; the code assertion is
informational until PROTOCOL.md documents the code.

**Never game an assertion.** Do not change `token <= 0` to `token < 0` to
accommodate a token of 0 in clustered mode. Token 0 is the NoopManager
sentinel — receiving it in clustered mode means the Postgres sequence is not
being used, which is a real bug.
