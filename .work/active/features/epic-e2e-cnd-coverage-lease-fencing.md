---
id: epic-e2e-cnd-coverage-lease-fencing
kind: feature
stage: drafting
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

## Next

`/agile-workflow:e2e-test-design epic-e2e-cnd-coverage-lease-fencing`
