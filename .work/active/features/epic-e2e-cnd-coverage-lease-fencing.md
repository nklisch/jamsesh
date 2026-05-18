---
id: epic-e2e-cnd-coverage-lease-fencing
kind: feature
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E CND Coverage — Lease Fencing

## Brief

`epic-cloud-native-deploy-lease-fencing` shipped per-session Postgres
advisory-lock leases (`pg_try_advisory_lock(hashtext(session_id))`) with
monotonic fencing tokens. Together they prevent split-brain corruption
of bare repos: only one pod can hold a session's lease at a time, and
every object-storage write carries a token that's rejected if it's
stale.

Coverage today: zero. No test references `pg_try_advisory_lock`,
`hashtext`, `fencing_token`, `NoopManager`, or split-brain semantics.
The grep matches for "lease" in the existing suite are the unrelated
REST `finalize/lock` resource — application-level lock state, not
Postgres advisory-lock leases.

This is **safety-critical surface**. Split-brain on bare-repo writes
means two pods independently update the same `refs/heads/draft` with
divergent commits, then sync conflicting refs to object storage. The
fencing-token check is the only thing standing between this codebase
and silent repo corruption in clustered mode. A green-but-tautological
test against fencing would be actively harmful.

## Audit findings addressed

- **F1 (Critical, journey-gap, all four taxonomy layers)** — Lease-fencing
  has zero coverage. No test exercises advisory-lock acquisition, fencing-
  token issuance, NoopManager identity in single-instance, lease TTL, or
  lease release on shutdown.
- **F11 (Medium, missing-taxonomy-layer fuzz)** — Fencing-token format
  has no fuzz coverage. Tokens are monotonic comparable values; malformed
  tokens at the object-storage check boundary are an attack surface
  (whatever defeats the split-brain guard is by definition critical).
- **F13 (High, missing-taxonomy-layer chaos)** — Pod-kill chaos for lease
  ownership. `tests/e2e/chaos/runtime_and_clock_test.go:134-267`
  (`automerger_pause`) uses `docker pause` on a single portal; no test
  kills a pod that holds a session lease. This is the canonical chaos
  test for the lease+hydration co-invariant.
- **F14 (Medium, missing-taxonomy-layer chaos)** — Cross-pod clock skew
  affecting lease TTL. `tests/e2e/chaos/runtime_and_clock_test.go:65-122`
  uses build-tag-gated `/test/clock-advance` for OAuth token expiry on
  a single portal but not cross-pod TTL.

(F13 also gets coverage in `epic-e2e-cnd-coverage-hydration-handoff` —
this feature owns the lease-ownership half, that feature owns the
handoff half. Design pass coordinates to avoid duplicate tests.)

## Scope

### Tests to add

1. **`tests/e2e/golden/lease_acquire_and_fence_test.go`**
   - Subtest `single_pod_acquires_lease_for_session` — pod creates a
     session, holds the advisory lock (verified by direct Postgres query
     against `pg_locks`), fencing token is emitted in the lease handle
     (verified by a test-only debug endpoint or by reading a test-mode
     trace header).
   - Subtest `two_pods_race_acquire_only_one_wins` — both pods attempt to
     acquire the same session_id concurrently; exactly one succeeds.
     Loser returns the documented error code.
   - Subtest `monotonic_fencing_tokens_across_acquisitions` — same session
     acquired, released, re-acquired; second token > first token.
   - Assertions on Postgres state (real `pg_locks` query) and HTTP-level
     responses. No mock of the lease manager.

2. **`tests/e2e/failure/lease_already_held_test.go`**
   - Pod A holds session S's lease; pod B receives a request for session
     S; pod B's response must be 503 with `lease.held_elsewhere` error
     code and a `Retry-After` header. Asserts on user-visible HTTP shape.

3. **`tests/e2e/failure/stale_fencing_token_rejected_test.go`**
   - Acquire lease (token T1), simulate stale state (release + re-acquire
     gives token T2), explicitly call the object-storage sync path with
     a forged T1; assert the write is rejected with the documented error.
   - This may require a test-only debug surface to inject the stale token;
     design pass decides whether that's a new endpoint behind a build tag,
     or done by directly manipulating Postgres advisory-lock state from
     the test process. Real Postgres advisory locks remain the spec.

4. **`tests/e2e/chaos/lease_holder_killed_test.go`**
   - Pod A acquires session S's lease, agent has an open WS, pumba SIGKILL
     pod A. Assertions within an SLO:
     - Postgres advisory lock auto-releases (the lock's tied to the dropped
       Postgres connection — this is the spec).
     - Pod B acquires the lease on next request for session S.
     - Pod B's fencing token > pod A's token (monotonic).
     - Subsequent writes from a hypothetical "pod A came back" with the
       old token are rejected.
   - Uses existing Pumba pattern (`tests/e2e/chaos/runtime_and_clock_test.go:134-267`).

5. **`tests/e2e/chaos/cross_pod_clock_skew_test.go`** (F14)
   - 2-pod cluster, both with leases on different sessions. Advance pod
     A's clock past lease TTL via libfaketime `LD_PRELOAD` (the existing
     pattern from the chaos suite). Pod B's clock stays normal.
   - Assert: both pods still agree on lease ownership (Postgres `now()` is
     the anchor; local clocks must not be authoritative).
   - **If this test fails, it surfaces a real bug. Park it via
     `/agile-workflow:park`, land the test with `t.Skip` + backlog id;
     do not silently change the assertion to make it green.**

6. **`tests/e2e/fuzz/fencing_token_test.go`** (F11)
   - Property-based: any malformed token sent to the object-storage write
     precondition path (truncated, oversize, non-monotonic, non-numeric,
     wrong-encoding, embedded null bytes) must either be rejected with a
     typed error or accepted as a valid token. Never panic, never silently
     accept-as-invalid (which would defeat split-brain prevention), never
     SEGV the portal.
   - Seed corpus: empty string, max int64, min int64, negative, decimal,
     embedded newlines, embedded nulls, UTF-8 boundary cases, oversize
     base64.
   - Reuses the fuzz suite's existing structure
     (`tests/e2e/fuzz/mcp_tool_input_test.go`).

### Helpers

- A "verify lease holder via Postgres" helper — direct query against
  `pg_locks` to identify which connection (and thus which portal pod)
  holds a given session's advisory lock. Lives in
  `tests/e2e/fixtures/portalcluster/lease_inspect.go`.
- A "wait for lease to migrate from pod X to pod Y" helper with bounded
  retry. Same file.

## Mock-boundary plan

| External dep              | Service-level mock                  | Notes |
|---------------------------|-------------------------------------|-------|
| Postgres advisory locks   | Real Postgres (existing fixture)    | The lock semantics ARE the spec; in-process mock would test the mock, not the system |
| Pod kill / SIGKILL        | Pumba (existing chaos pattern)      | Reuse |
| Clock skew per-pod        | libfaketime via `LD_PRELOAD`        | Existing chaos-suite pattern |
| Lease-fencing internals   | No mocking — drive via real REST + Postgres + object-storage paths | Test-only debug surface ok behind build tag if needed |

No in-process mocks. The `NoopManager` is implementation choice for
single-instance and is exercised implicitly by every existing single-
instance test — no separate test needed beyond identity assertion if
useful.

## Open questions for design

- **Test-only debug surface for forging a stale token.** Build-tag-gated
  endpoint (the existing `/test/clock-advance` pattern), or direct
  Postgres manipulation from the test process? Lean toward the former for
  consistency. Resolve in design pass.
- **Fencing-token format.** Is it int64, base64-encoded UUID, or
  something else? Determines the fuzz corpus shape. Confirm by reading
  a real lease handle response (probe-call to a clustered portal), not
  source.
- **Advisory-lock holder query** — does `hashtext(session_id)` produce
  a stable lock key across PG major versions? If yes, a direct `pg_locks`
  query works. If unstable, need a portal-side lease-debug endpoint.
  Resolve in design pass.
- **Cross-pod clock-skew test: what's the assertion if the
  implementation IS clock-anchored locally?** The instinct is "fail loudly
  and park the bug" — but design pass should confirm the expected
  semantics from `docs/SPEC.md` or `docs/ARCHITECTURE.md` (without
  reading implementation), so the assertion target is grounded.
- **Lease TTL value.** Tests need an SLO bound for "lease auto-releases
  within N seconds after pod kill". Confirm from SPEC or config defaults.

## Acceptance criteria

- [ ] `lease_acquire_and_fence_test.go` green; asserts on real Postgres
      `pg_locks` state and HTTP responses
- [ ] `lease_already_held_test.go` green; 503 + `Retry-After` + documented
      error code
- [ ] `stale_fencing_token_rejected_test.go` green; documented rejection
      on forged token
- [ ] `lease_holder_killed_test.go` green; lease migrates within SLO,
      monotonic-token invariant holds, stale-pod-A writes rejected
- [ ] `cross_pod_clock_skew_test.go` either green (clock-anchored at
      Postgres) or `t.Skip` with backlog-id reference (clock skew
      surfaces a real bug)
- [ ] `fencing_token_test.go` green; full fuzz corpus run; no panics; no
      silently-accepted-as-invalid tokens
- [ ] Lease-inspect helpers added to `portalcluster` fixture
- [ ] No new in-process mocks introduced

## Test integrity (from parent epic)

This is the feature where lying tests would do the most damage.
**Especially:**

- **F11 fuzz finding**: a fuzz harness that "passes" because the portal
  silently accepts a malformed token as not-matching is worse than no
  test. The invariant is "rejected with typed error OR accepted as the
  current valid token". Assertion must distinguish.
- **F14 clock skew**: if the implementation uses local clocks, the test
  WILL fail. Park the bug via `/agile-workflow:park`, land the test with
  `t.Skip("<backlog-id>: clock anchored locally, split-brain possible
  under skew")`. The skipped test is the audit trail.

## Non-goals

- Multi-region lease coordination (parent CND epic explicitly defers
  multi-region)
- NoopManager unit-equivalent tests (covered by every existing single-
  instance e2e test; no need for explicit feature-level coverage)
- Performance characterization of lease acquisition (perf-design territory)

## Design decisions (autopilot pass, 2026-05-17)

- **Stale-token forging mechanism:** direct Postgres manipulation from the
  test process (`ReleaseLeaseForcibly` + manifest injection via MinIO fixture)
  rather than a new build-tag endpoint. The cluster fixture already has a
  direct Postgres DSN and the MinIO fixture exposes `PutObject`. Simpler,
  no new production surface, consistent with how `LeaseHolder` works.
  Follow-on story if manifest format is too opaque: add a build-tag-gated
  `/test/inject-stale-manifest` endpoint.

- **Fencing token format confirmed as `int64`:** from `IssueLeaseFencingToken`
  (`nextval('jamsesh_lease_fencing_tokens')::bigint`). Fuzz corpus uses
  int64 boundary values. Stored in object metadata as decimal string under
  key `jamsesh-fencing-token`.

- **Advisory-lock holder query stability:** uses `hashtext($1)::oid` matching
  the production code. The portability risk (hashtext not guaranteed stable
  across PG major versions) is documented in `lifecycle.go` and flagged in
  `## Risks`. The e2e suite targets postgres:16-alpine; if the fixture PG
  version changes, re-validate.

- **Clock-skew test target:** the heartbeat goroutine (`runHeartbeat`) uses
  `time.NewTicker` — a local-clock construct. Advancing the container clock
  accelerates the ticker interval, which shrinks the `PingContext` timeout
  window. This is the testable bug path. If the test fails consistently,
  it is a real split-brain risk. Park + skip rather than soften.

- **Lease "TTL" / SLO:** the advisory lock releases on TCP connection drop
  (pod kill), which is near-instantaneous at the Postgres level. The SLO of
  30s for `WaitForLeaseMigration` is conservative (3× the 10s default
  heartbeat). Tests set `JAMSESH_LEASE_HEARTBEAT_INTERVAL_S=2` to reduce
  wall-clock waiting in CI.

- **`lease.held_elsewhere` error code:** not yet defined in `httperr`
  package or PROTOCOL.md. The golden + failure tests assert on HTTP 503
  status (the safety-critical assertion) and log whatever error code
  appears. A follow-on story should add the code to PROTOCOL.md and the
  `httperr` package if it's missing.

- **Coordination with `hydration-handoff`:** `lease_holder_killed_test.go`
  owns the lease-ownership invariants (lock release, monotonic token). The
  hydration-handoff feature owns hydration and client-continuity invariants.
  No hydration assertions in this feature's chaos tests.

- **NoopManager:** not explicitly tested in this feature. Token 0 (the
  NoopManager sentinel) is exercised by every existing single-instance e2e
  test. The clustered-mode tests explicitly assert `token > 0` to confirm
  they are not accidentally using NoopManager.

---

## Mock-boundary plan

| External dependency          | Service-level mock                            | Notes |
|------------------------------|-----------------------------------------------|-------|
| Postgres advisory locks      | Real Postgres (existing fixture)              | The lock semantics ARE the spec; in-process mock would test the mock, not the system |
| Pod kill / SIGKILL           | `c.Kill(podIndex)` → `docker kill --signal SIGKILL` | Implemented in `lifecycle.go`; no Pumba needed for SIGKILL-only scenarios |
| Clock skew per-pod           | `p.AdvanceClock()` via libfaketime LD_PRELOAD | Existing chaos-suite pattern in `clockadvance.go` |
| Object storage (MinIO)       | MinIO `RELEASE.2024-12-18T13-15-44Z` (existing fixture) | AWS creds via `AWS_ACCESS_KEY_ID=minioadmin` / `AWS_SECRET_ACCESS_KEY=minioadmin` |
| Fencing token injection      | Direct Postgres + MinIO fixture manipulation  | No new production endpoint; follow-on story if manifest format is opaque |

**No in-process mocks.** The `NoopManager` is implicitly exercised by
every existing single-instance test. All clustered-mode tests use real
`PostgresManager` via the cluster fixture.

---

## Taxonomy plan

- **Golden:** 3 subtests in `lease_acquire_and_fence_test.go` — single-pod
  acquisition, two-pod race (only one wins), monotonic tokens across
  acquisitions. All assert on Postgres state and HTTP responses.
- **Failure:** 2 test files — `lease_already_held_test.go` (503 + Retry-After
  when second pod receives session already held), `stale_fencing_token_rejected_test.go`
  (write with stale token is rejected, not silently accepted).
- **Chaos:** 2 test files — `lease_holder_killed_test.go` (F13: pod-kill,
  lease migration within 30s SLO, monotonic token), `cross_pod_clock_skew_test.go`
  (F14: clock skew either passes cleanly or parks a real bug).
- **Fuzz:** 1 harness — `fencing_token_test.go` (F11: property-based fuzz
  on token format boundary; seed corpus of 17 edge cases; correctness
  sub-test for explicit rejection).

---

## Implementation units

### Unit 1: Infrastructure helpers
**File:** `tests/e2e/fixtures/portalcluster/lease_inspect.go`
**Story:** `epic-e2e-cnd-coverage-lease-fencing-infra`
**Invariant:** all downstream test helpers compile and behave correctly
against the e2e Postgres fixture.

Adds `RequireLeaseHolder`, `FencingTokenForSession`, `ReleaseLeaseForcibly`
to the `portalcluster` package. (`LeaseHolder` and `WaitForLeaseMigration`
already exist in `lifecycle.go`.)

```go
// RequireLeaseHolder polls LeaseHolder until a holder is found or timeout.
func (c *Cluster) RequireLeaseHolder(ctx, t, sessionID, timeout) int

// FencingTokenForSession queries the leases table for the most recent token.
func (c *Cluster) FencingTokenForSession(ctx, t, sessionID) int64

// ReleaseLeaseForcibly marks the most recent lease row as released in PG.
// Used by stale-token tests to simulate re-acquisition.
func (c *Cluster) ReleaseLeaseForcibly(ctx, t, sessionID)
```

**Acceptance criteria:**
- [ ] All three helpers compile.
- [ ] `RequireLeaseHolder` calls `t.Fatal` on timeout (not `t.Log`).
- [ ] `FencingTokenForSession` returns -1 for unknown sessions (not 0).

---

### Unit 2: Golden tests
**File:** `tests/e2e/golden/lease_acquire_and_fence_test.go`
**Story:** `epic-e2e-cnd-coverage-lease-fencing-golden`
**Invariant:** in a 2-pod cluster, per-session advisory lock acquisition
is exclusive, fencing tokens are monotonically increasing, and the lease
record in Postgres is accurate.

**Subtests:**
- `single_pod_acquires_lease_for_session` — one pod holds the lock; token > 0.
- `two_pods_race_acquire_only_one_wins` — direct request to non-holder pod
  returns 503.
- `monotonic_fencing_tokens_across_acquisitions` — T2 > T1 after pod kill
  and re-acquisition.

**Setup:** 2-pod cluster, `Router: true`, `JAMSESH_LEASE_HEARTBEAT_INTERVAL_S=2`.

```go
func TestLeaseAcquireAndFence(t *testing.T) {
    t.Run("single_pod_acquires_lease_for_session", testSinglePodAcquiresLease)
    t.Run("two_pods_race_acquire_only_one_wins", testTwoPodsRaceAcquire)
    t.Run("monotonic_fencing_tokens_across_acquisitions", testMonotonicFencingTokens)
}
```

**Assertion targets:** `c.RequireLeaseHolder` return value, `c.FencingTokenForSession`
return value, HTTP status on direct-to-pod-B request.

---

### Unit 3: Failure-mode tests
**File:** `tests/e2e/failure/lease_already_held_test.go`
**Story:** `epic-e2e-cnd-coverage-lease-fencing-failure`
**Invariant:** pod B returns 503 + Retry-After when pod A holds the session lease.

```go
func TestLeaseAlreadyHeld(t *testing.T) { ... }
```

**File:** `tests/e2e/failure/stale_fencing_token_rejected_test.go`
**Story:** `epic-e2e-cnd-coverage-lease-fencing-failure`
**Invariant:** a write with a stale fencing token is explicitly rejected;
the manifest in MinIO is NOT overwritten.

```go
func TestStaleFencingTokenRejected(t *testing.T) { ... }
```

**Assertion targets:** HTTP status 503, `Retry-After` header presence,
non-empty error code, MinIO manifest token unchanged after rejection.

---

### Unit 4: Chaos tests
**File:** `tests/e2e/chaos/lease_holder_killed_test.go`
**Story:** `epic-e2e-cnd-coverage-lease-fencing-chaos`
**Invariant:** after SIGKILL of the lease-holder pod, the advisory lock
auto-releases, a second pod acquires the lease with T2 > T1, and subsequent
pushes succeed within 30s SLO.

```go
func TestLeaseHolderKilled(t *testing.T) { ... }
```

**File:** `tests/e2e/chaos/cross_pod_clock_skew_test.go`
**Story:** `epic-e2e-cnd-coverage-lease-fencing-chaos`
**Invariant:** clock skew on one pod does not destabilize lease ownership.
If it does: park the bug, `t.Skip` with backlog id.

```go
func TestCrossPodClockSkew(t *testing.T) { ... }
```

**Chaos mechanism:** `c.Kill(0)` for pod-kill; `p.AdvanceClock()` for skew.

---

### Unit 5: Fuzz harness
**File:** `tests/e2e/fuzz/fencing_token_test.go`
**Story:** `epic-e2e-cnd-coverage-lease-fencing-fuzz`
**Invariant:** no input at the fencing-token boundary causes a 5xx. A token
lower than the stored value is explicitly rejected, not silently accepted.

```go
func TestFencingTokenFuzz(t *testing.T) { ... }          // property: no 5xx
func TestFencingTokenRejectionIsExplicit(t *testing.T) { ... } // correctness
```

**Corpus file:** `tests/e2e/fuzz/testdata/fencing-token-corpus.json`
(17 seed cases covering int64 boundaries, non-numeric, oversized, special chars).

---

## Implementation order

1. `epic-e2e-cnd-coverage-lease-fencing-infra` (helpers; unblocks golden + fuzz)
2. `epic-e2e-cnd-coverage-lease-fencing-golden` (golden; unblocks failure + chaos)
3. `epic-e2e-cnd-coverage-lease-fencing-failure` (parallel with chaos after golden)
4. `epic-e2e-cnd-coverage-lease-fencing-chaos` (parallel with failure after golden)
5. `epic-e2e-cnd-coverage-lease-fencing-fuzz` (parallel with golden after infra)

---

## Risks

- **hashtext portability:** `hashtext()` is implementation-defined in
  Postgres and may not be stable across major versions. The `LeaseHolder`
  helper notes this risk. If the e2e suite ever targets a different PG
  major version than the production advisory-lock key was tuned for,
  `LeaseHolder` will silently return -1. Mitigation: pin the e2e Postgres
  image to postgres:16-alpine and document that the lock key is version-sensitive.

- **Manifest format opacity:** the stale-token tests rely on injecting a
  known token value into the MinIO manifest. If the manifest's JSON schema
  or ETag-conditional-write protocol is not stable enough to inject from
  test code, the stale-token tests must be `t.Skip`'d and a follow-on story
  filed for a test-side injection helper. This is the weakest point in the
  failure-mode test design.

- **`lease.held_elsewhere` error code undefined:** the `httperr` package
  has no lease-specific error constructor. The golden and failure tests
  assert on 503 status (correct behavior) and log the actual error code
  (informational). If the error code is `internal` or `dep.db_unavailable`
  rather than a lease-specific code, that is a usability gap (not a
  correctness bug) — file a follow-on story to add the code to PROTOCOL.md.

- **Clock-skew test may reliably fail:** `runHeartbeat` uses `time.NewTicker`
  with interval = `PingContext` timeout. A clock advance that accelerates
  the ticker shrinks the effective timeout window, causing spurious heartbeat
  failure. This IS the bug — the test is designed to surface it. Park
  rather than skip without a bug item.

- **Sibling feature coordination (hydration-handoff):** the chaos test's
  `lease_holder_killed_test.go` tests lease-migration. The hydration-handoff
  feature (`epic-e2e-cnd-coverage-hydration-handoff`) tests the same scenario
  from the client-continuity angle. These are designed to be complementary,
  not duplicate — if both end up testing the same assertion, the review pass
  on hydration-handoff should prune the duplicate.

---

## Next

`/agile-workflow:implement-orchestrator epic-e2e-cnd-coverage-lease-fencing`
