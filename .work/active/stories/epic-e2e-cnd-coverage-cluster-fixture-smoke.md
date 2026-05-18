---
id: epic-e2e-cnd-coverage-cluster-fixture-smoke
kind: story
stage: implementing
tags: [e2e-test, testing, infra]
parent: epic-e2e-cnd-coverage-cluster-fixture
depends_on: [epic-e2e-cnd-coverage-cluster-fixture-portalcluster]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Keystone clustered smoke test

## Scope

The acceptance test for the entire `epic-e2e-cnd-coverage-cluster-fixture`
feature. If this passes, every downstream content feature
(lease-fencing, object-storage-sync, routing-layer, hydration-handoff)
is unblocked to start.

Adds `tests/e2e/scaffolding/cluster_smoke_test.go > TestClusteredSmoke`
which brings up the full clustered stack and exercises the end-to-end
lifecycle: session creation → push → object-storage mirror → graceful
drain → handoff → session-state preserved on the new pod.

Updates `tests/e2e/README.md` with a "Clustered mode" section so future
contributors understand the new entry point.

## Files

- `tests/e2e/scaffolding/cluster_smoke_test.go` — `TestClusteredSmoke`
- `tests/e2e/README.md` — add "Clustered mode" section

## Test scaffold

See parent feature body, Unit 4, for the full scaffold. Recap of steps:

1. Start Postgres + MinIO via existing fixtures.
2. Start 3-pod clustered portal cluster with router enabled.
3. Create session via router (assert 201, capture session_id).
4. Push a commit via router git smart-HTTP (assert push success, capture HEAD).
5. List MinIO bucket; assert pack object present for the session (RPO=0
   invariant at smoke level).
6. Query `LeaseHolder(session_id)`; assert a pod holds the lease.
7. `GracefulDrain` the holding pod (30s timeout).
8. Request session HEAD via router; assert response matches the original
   pushed HEAD (handoff preserved state).
9. `WaitForLeaseMigration`; assert a different pod now holds the lease.

## Invariant

"A session created on pod A is visible on pod B after a graceful drain,
with all committed state preserved and the object backed by MinIO."

## Acceptance criteria

- [ ] `TestClusteredSmoke` is green when run via `make test-e2e`
- [ ] Every assertion is against user-visible state (REST response codes,
      response body content, bucket contents, lease-holder identity) —
      no mock-invocation asserts
- [ ] Test skips cleanly when Docker or images are unavailable
- [ ] Total runtime under 3 minutes on a developer laptop (Postgres
      shared-binary cache + parallel pod start should keep this tight)
- [ ] `tests/e2e/README.md` gains a "Clustered mode" subsection naming
      the three new fixtures + the smoke entry point
- [ ] If the smoke test surfaces a real product bug (e.g., handoff
      silently drops state, RPO=0 violated even under happy path), the
      bug is parked via `/agile-workflow:park` with specifics; the test
      is `t.Skip("park-id: short reason")` rather than weakened

## Test integrity (from parent epic)

This is the test where a tautology would do the most damage —
"TestClusteredSmoke passes therefore clustered mode works" is the
strongest claim the suite will make. Be ruthless about it:

- Step 5 (bucket inspection) MUST find the pushed object. If MinIO is
  empty after a 2xx push, that's an RPO=0 violation — park as Critical
  bug, t.Skip with reference.
- Step 8 (state preservation) MUST compare actual content. If the
  handoff lets pod B serve a stale or empty state, park the bug — do
  not change the assertion to "any 200 OK".
- Step 9 (lease migration) MUST verify the holder index changed. Asserting
  only that "some pod holds the lease" misses the migration entirely.

If you encounter implementation surprises (e.g., the router's hint cache
keeps routing to the drained pod past expectation), surface them in the
story body's "Implementation notes" section and ask the parent feature
to absorb fixture changes — don't paper over in this test.

## References

- Parent feature body, Unit 4 — full scaffold + helper notes
- `tests/e2e/golden/session_join_and_push_test.go` — existing patterns
  for creating sessions + pushing commits in e2e (helpers to reuse)
- `tests/e2e/golden/onboarding_test.go` — existing REST-flow patterns

## Dependencies on this story (downstream)

- Unblocks `epic-e2e-cnd-coverage-lease-fencing`,
  `epic-e2e-cnd-coverage-object-storage-sync`,
  `epic-e2e-cnd-coverage-routing-layer` to enter their own design passes
- Unblocks `epic-e2e-cnd-coverage-cluster-fixture` to advance to review

## What's now possible (on completion)

The full clustered shape of CND can be exercised end-to-end in CI. Every
downstream feature's test bodies can spin up clustered stacks in a few
lines of fixture code. The keystone smoke runs on every PR.
