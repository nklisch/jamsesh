---
id: epic-e2e-cnd-coverage-hydration-handoff
kind: feature
stage: drafting
tags: [e2e-test, testing, portal]
parent: epic-e2e-cnd-coverage
depends_on: [epic-e2e-cnd-coverage-lease-fencing, epic-e2e-cnd-coverage-object-storage-sync, epic-e2e-cnd-coverage-routing-layer]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E CND Coverage — Hydration & Handoff (Capstone)

## Brief

`epic-cloud-native-deploy-hydration-handoff` was the capstone of the
production CND epic, composing lease-fencing + object-storage-sync +
routing into a lifecycle manager that handles acquire → hydrate → serve →
evict cleanly. It's how sessions migrate between pods without losing
state.

Coverage today: zero. No tests reference "hydrate", "evict", "handoff",
"lifecycle manager", or session migration.

This feature is the **capstone of CND e2e coverage** — it depends on the
three middle-band features because it tests their composition. A
handoff that loses commits, leaks evicted-but-still-active sessions,
or hydrates from a stale bucket is exactly the bug class the production
epic structure was designed to prevent (which is why the production
epic explicitly forbade splitting hydration from eviction).

## Audit findings addressed

- **F3 (Critical, journey-gap, all four taxonomy layers)** — Hydration-
  handoff has zero coverage. No tests use a clustered fixture; no
  references to lifecycle, hydration, eviction, migration.
- **F13 handoff half (High, missing-taxonomy-layer chaos)** — Pod-kill
  during active session. The lease-fencing feature owns the
  lease-ownership half (lease auto-releases, fencing-token monotonicity);
  this feature owns the handoff half (session state actually serves
  correctly from pod B after pod A dies).

## Scope

### Tests to add

1. **`tests/e2e/golden/session_handoff_clean_drain_test.go`**
   - Pod A holds session S, pushed commits up to ref X. Gracefully drain
     pod A (SIGTERM, wait for clean shutdown). Pod B acquires the lease
     on next request. Hydrate from MinIO.
   - Assert: pod B reports the same draft tip (ref X), the same recent
     events log, the same finalize state. Subsequent push on pod B
     succeeds and shows up in MinIO with monotonic-greater fencing token.
   - **Invariant**: "After clean drain + handoff, no committed state is
     lost or duplicated."

2. **`tests/e2e/golden/session_handoff_idle_eviction_test.go`**
   - Pod A holds session S, agent goes idle past eviction threshold.
     Lease is released (eviction), session's local cache evicted.
     Subsequent request for S routes (via consistent-hash or hint cache)
     to pod B (or even back to pod A, depending on hash) — pod B
     hydrates from MinIO.
   - Assert: hydration succeeds; client sees the correct state; LRU
     metrics (if exposed via `/metrics`) reflect the eviction.

3. **`tests/e2e/chaos/handoff_under_pod_kill_test.go`** (F13 handoff half)
   - Pod A holds session S, agent has open WS, ack'd 5 commits.
     Pumba SIGKILL pod A.
   - Assert within SLO: pod B acquires lease, hydrates from MinIO, agent's
     WS reconnects to pod B (via router failover), draft tip on pod B
     reflects all 5 ack'd commits (zero data loss across the kill).
   - Coordinates with `epic-e2e-cnd-coverage-lease-fencing > F13` —
     the lease-fencing feature asserts on lease-ownership semantics
     (auto-release, monotonic tokens); this test asserts on the user-
     visible handoff outcome (no data loss, agent reconnects cleanly).
     Design pass coordinates to avoid duplicate test bodies; ideally
     each test asserts on its own layer's invariant only.

4. **`tests/e2e/chaos/handoff_under_object_storage_chaos_test.go`**
   - Pod A holds session S with pending writes to MinIO. Apply Toxiproxy
     latency (3-5s) on portal→MinIO. Trigger handoff (drain pod A or
     kill it). Pod B hydrates while object-storage path is slow.
   - Assert: hydration eventually completes (within an extended SLO),
     no commits lost, no acknowledged state missing on pod B.
   - The toxic clears after pod B starts hydrating; hydration succeeds
     even with the transient slowdown.

5. **`tests/e2e/failure/hydration_with_missing_pack_test.go`**
   - Pod A pushes commits, pack object lands in MinIO. Externally
     delete the pack object from MinIO (out-of-band corruption / disaster
     scenario). Drain pod A. Pod B attempts to acquire and hydrate.
   - Assert: pod B refuses to serve the session, surfacing a documented
     error code (e.g. `hydration.corrupt_bucket` or similar). Does NOT
     silently truncate, does NOT serve a partial state.
   - **This is the safety invariant**: a missing pack object must fail
     hydration loudly, not silently produce wrong data.

6. **`tests/e2e/golden/lifecycle_evict_on_lease_release_test.go`**
   - Pod A holds session S. Force lease release (e.g., explicit
     `release` REST endpoint if exposed, or wait past idle eviction).
     Assert: local cache for session S is removed from pod A's
     filesystem (verify via test-only file-inspect endpoint or via
     container-fs introspection through the Testcontainers API). LRU
     eviction respects the configured threshold.

### Helpers

- A "compare session state across two pods" helper — reads draft tip,
  recent events, finalize state from each, asserts deep-equal.
  Lives in `tests/e2e/fixtures/portalcluster/state_compare.go`.
- A "wait for hydration to complete" helper — polls a debug endpoint or
  log line until pod reports hydration done, with bounded retry.
- A "delete bucket object out-of-band" helper for F5 (corruption
  scenario). Lives in `tests/e2e/fixtures/minio/inspect.go` extension.

## Mock-boundary plan

| External dep                | Service-level mock                | Notes |
|-----------------------------|-----------------------------------|-------|
| Multi-pod portal cluster    | From cluster-fixture              | Reuse |
| MinIO                       | From cluster-fixture              | Reuse |
| Router                      | From cluster-fixture              | Reuse |
| Postgres advisory locks     | From cluster-fixture              | Reuse |
| Object-storage chaos        | Toxiproxy (existing pattern)      | Reuse from object-storage-sync feature |
| Pod kill                    | Pumba (existing chaos pattern)    | Reuse |
| Out-of-band bucket mutation | Direct S3 SDK call in test process | Test is doing the corruption — not mocking anything |

No in-process mocks. The hydration lifecycle is exactly the path that
must run real-end-to-end to verify the safety properties.

## Open questions for design

- **Coordination with `epic-e2e-cnd-coverage-lease-fencing > F13`.** Both
  features want a pod-kill chaos test. The cleanest split:
  - lease-fencing F13 owns: lease-ownership invariants (auto-release,
    monotonic-token, stale-token-rejected).
  - hydration-handoff F3 chaos test owns: user-visible state preservation
    (draft tip, events, no data loss, WS reconnect).
  Same docker setup, different assertion focus, potentially shared
  test-helper for spinning the scenario. Design pass formalizes.
- **Test-only state-inspect endpoint vs. file-system probe.** The
  evict-on-release test needs to verify the local cache is gone.
  Options: (a) build-tag-gated `/test/cache-state` endpoint listing
  what's resident; (b) docker-exec `ls /var/jamsesh/cache/...` via
  Testcontainers; (c) verify the cache is empty by indirect observation
  (next request to that pod has to re-hydrate, measurably slower).
  Resolve in design.
- **What's the documented hydration SLO?** Tests assert "within N seconds"
  for handoff; N has to come from `docs/SELF_HOST.md`, `docs/SPEC.md`,
  or a probe-call against a real clustered portal. Don't read source.
- **LRU vs. idle eviction interaction.** The lifecycle has both: LRU
  (memory-pressure-driven) and idle (time-driven). Tests probably focus
  on idle (more deterministic); LRU may need a memory-pressure helper
  that's hard to make reliable in containers. Design pass picks scope.
- **Test for "pod A came back" after kill, with stale state cached.** If
  pod A is restarted post-kill and somehow tries to serve session S
  (which now lives on pod B), the stale-token rejection from
  lease-fencing F1 should kick in. Worth a dedicated test here that
  asserts on user-visible outcome (pod A's response to a session-S
  request after restart). Cross-references lease-fencing.

## Acceptance criteria

- [ ] `session_handoff_clean_drain_test.go` green; state preserved
      across SIGTERM handoff
- [ ] `session_handoff_idle_eviction_test.go` green; eviction +
      re-hydration round-trip preserves state
- [ ] `handoff_under_pod_kill_test.go` green; SIGKILL + handoff loses
      zero ack'd commits
- [ ] `handoff_under_object_storage_chaos_test.go` green; handoff
      tolerates transient object-storage slowdown
- [ ] `hydration_with_missing_pack_test.go` green; corrupted bucket
      refused loudly (not silently)
- [ ] `lifecycle_evict_on_lease_release_test.go` green; local cache
      cleared, LRU/idle thresholds honored
- [ ] State-compare + wait-for-hydration helpers landed in fixtures
- [ ] No in-process mocks introduced
- [ ] Coordination doc with lease-fencing F13 prevents duplicate test
      bodies

## Test integrity (from parent epic)

This feature carries the heaviest test-integrity weight in CND coverage.
The handoff tests can quietly "succeed" if the assertion target is
weak. Three rules:

- **Inspect actual data, not just response status.** "Pod B returned 200"
  does not prove pod B has the right state. Every handoff test must
  compare the draft tip, the events log, and the finalize state across
  pods using the state-compare helper.
- **Loud failure on the corruption test.** F5
  (`hydration_with_missing_pack`) MUST fail if the system silently
  serves partial state. If the implementation does silently truncate,
  that's a Critical production bug — park via `/agile-workflow:park`,
  land the failing test with `t.Skip` + backlog id + a clear note in the
  test source naming the safety property that's violated.
- **Pod-kill chaos coordination.** Don't duplicate assertions across
  lease-fencing F13 and this feature's F3 chaos test. Each layer
  asserts its own invariant; both run, but neither replicates the
  other.

## Non-goals

- Performance characterization of hydration time under load
- Multi-region handoff (parent CND defers)
- Cross-version handoff (pod A on vN, pod B on vN+1) — separate concern,
  rolling-deploy testing

## Next

`/agile-workflow:e2e-test-design epic-e2e-cnd-coverage-hydration-handoff`
