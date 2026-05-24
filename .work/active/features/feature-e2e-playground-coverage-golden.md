---
id: feature-e2e-playground-coverage-golden
kind: feature
stage: drafting
tags: [testing, e2e-test, playground, portal, plugin]
parent: epic-e2e-playground-coverage
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Golden-path e2e tests for the playground subsystem

## Brief

Happy-path end-to-end coverage for the v0.4.0 playground subsystem and
the `jamsesh jam --playground` CLI flow. Each test runs against the real
portal binary in a Testcontainers stack (postgres + portal + binary +
gitclient) and asserts on user-visible outcomes (HTTP responses, real
filesystem state via `dockerExec`, real DB rows, real tombstone payload)
rather than mock invocations.

This feature establishes the playground e2e patterns that the failure /
chaos / fuzz features inherit:

- Real bare-repo path assertion shape (sibling to existing
  `lifecycle_evict_on_lease_release_test.go > VerifyCacheEvicted`)
- `/test/clock-advance` usage to drive destruction-worker behavior
  without waiting wall-clock time
- Anonymous-bearer header injection through the `binary` fixture
- Two-participant orchestration via parallel binary processes
- Tombstone read-back after sweep

## Child stories

This feature has 5 child stories, all carried over from the
`e2e-test-design --audit` run (slug prefix `e2e-audit-` preserved for
provenance):

1. `e2e-audit-playground-solo-create-push-tombstone-journey` (Critical) —
   solo creator full journey: create → push → tombstone after hard-cap
2. `e2e-audit-playground-two-participant-join-merge-journey` (Critical) —
   two-participant join + collaborative push + auto-merge
3. `e2e-audit-cli-jam-playground-flag-end-to-end` (Critical) — `jamsesh
   jam --playground` real-binary end-to-end
4. `e2e-audit-playground-abandonment-destruction-sweep-journey` (High) —
   idle → destruction sweep → tombstone served (uses `/test/clock-advance`)
5. `e2e-audit-playground-handler-unit-tautology-stubstorage` (Medium) —
   cross-cutting cleanup: every existing unit test that asserts on
   `stubStorage.repos[key]` gets a sibling e2e test asserting on real
   on-disk state via `dockerExec`. This is a discipline story, not a
   single test — its closing condition is that the tautology pattern
   stops being introduced in new handler tests.

## Design status

The audit findings supply test sketches, file paths, and invariant
statements for each child story. The remaining design work
(`/agile-workflow:e2e-test-design feature-e2e-playground-coverage-golden`)
is:

- Lock the mock-boundary plan — confirm that the existing fixture
  inventory (`postgres`, `portal`, `binary`, `gitclient`, `wsclient`)
  covers every dep without needing a new custom container.
- Surface the test-data strategy (how the parked-bug clock-injection
  defect interacts with the abandonment-destruction story's clock-advance
  approach).
- Pre-mortem: which story is most likely to be flaky? (Candidate: the
  two-participant journey if the auto-merge ordering is sensitive to
  parallel push timing.)

## Next

`/agile-workflow:e2e-test-design feature-e2e-playground-coverage-golden`
— design pass that writes mock-plan, taxonomy-plan, implementation units
into this file body, then advances `stage: drafting → implementing`.
