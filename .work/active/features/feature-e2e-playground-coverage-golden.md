---
id: feature-e2e-playground-coverage-golden
kind: feature
stage: implementing
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

## Mock-boundary plan

Every external dependency is covered by existing Testcontainers fixtures
at `tests/e2e/fixtures/`. No new custom containers, no in-process mocks.

| External dep | Fixture | Notes |
|---|---|---|
| Postgres | `tests/e2e/fixtures/postgres/postgres.go` | Already exposes a clean `Start(t)` + DSN; the portal fixture's `ExtraEnv` accepts `JAMSESH_PG_DSN`. |
| Portal binary | `tests/e2e/fixtures/portal/portal.go` | `ExtraEnv` accepts the full `JAMSESH_PLAYGROUND_*` env-var suite (verified at `internal/portal/config/config_test.go:446-452`). Set `JAMSESH_PLAYGROUND_ENABLED=true`, `JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S`, `JAMSESH_PLAYGROUND_HARD_CAP_S`, etc. |
| Clock | `tests/e2e/fixtures/portal/clockadvance.go` | `p.AdvanceClock(ctx, t, d)` POSTs `/test/clock-advance` to advance the process-global clock. Portal must be built with `-tags e2etest` (the standard `make test-portal-image` target does this). Advance is cumulative + forward-only. |
| `jamsesh` CLI binary | `tests/e2e/fixtures/binary/jamsesh.go` | `Build(t)` returns the binary path; tests `exec.Command(binaryPath, "jam", "--playground", ...)` directly. No new fixture method needed. |
| Git repo client | `tests/e2e/fixtures/gitclient/` (assumed; verify in implementation) | For push-back operations against the playground session's smart-HTTP endpoint. |
| WebSocket client | `tests/e2e/fixtures/wsclient/` (assumed; verify in implementation) | For destruction-warning event subscriptions if the test needs WS observability. |

No mock-boundary violations — every dep maps to a service-level fixture
that already exists. The audit's claim "the work is purely additive" is
confirmed.

## Taxonomy plan

This feature is the **golden** layer only. Failure / chaos / fuzz are
separate sibling features that depend on this one.

- **Golden**: 5 tests covering 4 journeys + 1 cross-cutting discipline:
  - Solo creator full journey (create → push → tombstone)
  - Two-participant join + collaborative push + auto-merge
  - CLI `jam --playground` real-binary end-to-end
  - Abandonment → idle-timeout destruction sweep → tombstone served
  - Handler-unit-tautology cleanup discipline (sibling e2e assertion shape
    on every existing unit test that asserts on `stubStorage.repos[key]`)

## Anti-tautology guardrails

Every test in this feature must:

- Assert on **user-visible outcomes** — real HTTP responses, real DB row
  presence, real on-disk bare-repo existence via `dockerExec`, real
  tombstone JSON via `GET /tombstone`. Never assert on mock invocations.
- Run against the **real portal binary** (`jamsesh/portal:e2e` image)
  with the real Postgres container, not against an in-process
  `httptest.NewServer`.
- State its **invariant** in the test's top-of-function comment in plain
  English. If the invariant can't be written in one line, the test is
  tautological.

The cleanup-discipline story
(`e2e-audit-playground-handler-unit-tautology-stubstorage`) is the
codification of this guardrail — it asserts the pattern stays observed.

## Implementation Units

### Unit 1: solo-create-push-tombstone journey
**File**: `tests/e2e/golden/playground_solo_create_push_tombstone_test.go`
**Story**: `e2e-audit-playground-solo-create-push-tombstone-journey` (Critical)
**Invariant**: After an anonymous create + push, the bare repo exists on
real disk with hooks installed, and after hard-cap elapses + a destruction
sweep fires, the session row is gone but the tombstone is served with
accurate counts.

### Unit 2: two-participant join + merge journey
**File**: `tests/e2e/golden/playground_two_participant_join_merge_test.go`
**Story**: `e2e-audit-playground-two-participant-join-merge-journey` (Critical)
**Invariant**: Two anonymous participants on the same session can both push,
the auto-merger composes their changes, and both observe the merged result
in subsequent fetches.

### Unit 3: CLI `jam --playground` end-to-end
**File**: `tests/e2e/golden/cli_jam_playground_flag_test.go`
**Story**: `e2e-audit-cli-jam-playground-flag-end-to-end` (Critical)
**Invariant**: Running the real `jamsesh jam --playground` binary against
the real portal creates a playground session, prints the session ID and
attach URL, persists state to a tempdir-scoped `~/.jamsesh/state.json`,
and the resulting session is visible via `GET /api/playground/sessions/{id}`.

### Unit 4: abandonment → destruction sweep journey
**File**: `tests/e2e/golden/playground_abandonment_destruction_sweep_test.go`
**Story**: `e2e-audit-playground-abandonment-destruction-sweep-journey` (High)
**Invariant**: A playground session that sees no activity for the
idle-timeout window (advanced via `p.AdvanceClock`) is swept by the
destruction worker, its repo is deleted from disk, and the tombstone is
served at `GET /tombstone`. Uses `JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S=1`
so the test doesn't have to wait the default sweep cadence.

### Unit 5: handler-unit-tautology cleanup discipline
**File**: cross-cutting — adds `dockerExecExists` assertions to every
other golden test in this feature (no dedicated test file)
**Story**: `e2e-audit-playground-handler-unit-tautology-stubstorage` (Medium)
**Invariant**: For every existing unit test in
`internal/portal/playground/*_test.go` that asserts on `stubStorage.repos[key]`,
this feature's e2e siblings include the `dockerExecExists` assertion
shape on the real bare-repo path. The story closes when the documented
pattern is observable in all 4 golden tests above.

## Implementation Order

1. Unit 1 (solo-create-push-tombstone) — establishes the assertion shape
   for every other unit. Land first.
2. Unit 4 (abandonment → destruction sweep) — uses `AdvanceClock`; locks
   in the clock-advance + destruction-worker interaction patterns.
3. Unit 3 (CLI jam --playground) — adds the binary-fixture dimension on
   top of the patterns from 1 + 4.
4. Unit 2 (two-participant join + merge) — most complex; relies on Units
   1+3 patterns being established (auth header injection, repo state
   assertions, etc).
5. Unit 5 (cleanup discipline) — fold into 1-4 as you write them; no
   dedicated file. Closing condition is the `dockerExecExists` pattern
   being present in all four.

## Test integrity (mandatory for every child story)

- **Park production bugs, don't hide them.** If a test the audit specs
  fails because the product is genuinely broken (e.g. the parked join-410
  defect surfaces in Unit 2), the implementer parks via
  `/agile-workflow:park`, lands the failing test with a `t.Skip` linked
  to the backlog id and a one-line reason, and proceeds. The failing
  test is a feature, not a defect.
- **Fix bad tests in-session.** Stale fixtures, drifted assertions,
  drifted mocks — repair as part of the stride.
- **Never game an assertion to make it pass.** No `require.True(t, true)`,
  no asserting on whatever the code happens to return now, no deleting a
  flaky test without root-causing.

## Risks (pre-mortem)

- **Two-participant timing flake (Unit 2).** Parallel pushes against the
  auto-merger may race in ways that produce nondeterministic merge order.
  Mitigation: gate the second participant's push on the first
  participant's `post-receive` event landing (subscribe via wsclient),
  not on a sleep. Land Unit 2 last so the patterns are established.
- **Clock-advance interaction with destruction-worker ticker (Unit 4).**
  The destruction worker uses a `time.NewTicker` (per
  `ticker-sweep-loop` pattern), so advancing the wall clock doesn't
  necessarily fire a sweep — the ticker fires on its own real-time
  cadence. Mitigation: set `JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S=1`
  so a sweep fires within ~1s after AdvanceClock; the test polls
  `GET /tombstone` until 200 or a 10s deadline elapses.
- **CLI state-file pollution across tests (Unit 3).** `~/.jamsesh/state.json`
  is a per-user file in production. Mitigation: tests must set `HOME` to a
  tempdir before invoking the binary, and the binary fixture should be
  documented to require this. (If the binary doesn't honor `HOME` for
  the state path, that's a parked production bug.)
- **Pre-existing parked join-410 bug (Unit 2, possibly Unit 1).** The
  join handler currently returns 410 on freshly-created sessions in unit
  tests; if this reproduces against the real wall clock + real portal,
  Unit 2 will fail. Per test-integrity discipline: park the bug as a
  release-bound item (or update the existing backlog item), `t.Skip` Unit 2
  with the backlog id reference, and proceed. Document the discovery in
  the story body.

## Status

Design complete. Feature advanced to `implementing`. The 5 child stories
are also advanced to `implementing` so `implement-orchestrator` can pick
them up in waves.

## Next

`/agile-workflow:implement-orchestrator feature-e2e-playground-coverage-golden`
— builds the dependency graph from the implementation order above and
spawns sub-agents to write each test. Solo-create-push-tombstone (Unit 1)
has no story-level deps; the rest implicitly depend on Unit 1 for pattern
consistency.
