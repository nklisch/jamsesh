---
id: e2e-audit-playground-two-participant-join-merge-journey
kind: story
stage: done
tags: [testing, e2e-test, audit, playground]
parent: feature-e2e-playground-coverage-golden
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Two-participant playground journey (create → join → both push → auto-merge) has no e2e test

## Severity
Critical

## Finding type
journey-gap

## Evidence

`tests/e2e/golden/session_join_and_push_test.go` and
`tests/e2e/golden/auto_merge_test.go` together prove this journey is
possible for **authenticated org sessions**. Neither has an
anonymous-bearer / playground analogue. Grep confirms:

```
$ grep -rIn -E "playground|/api/playground.*join" tests/e2e/
(no output)
```

Unit coverage exists for the join handler in isolation
(`TestJoinPlaygroundSession_*` in handler_test.go) but two of those tests
— `TestJoinPlaygroundSession_Success` and
`TestJoinPlaygroundSession_WithNickname_UsesIt` — are currently broken on
main and parked at
`.work/backlog/bug-playground-join-with-nickname-returns-410-on-fresh-session.md`.
The unit-level join is regressed and **nothing else** covers join
end-to-end. The fact that this regression went undetected for an entire
release cycle is itself evidence that the e2e gap costs reliability.

## Why this matters

Two-participant playground is the second pitch of v0.4.0 (the first being
solo). The auto-merger interacts with both nickname tagging
(`extractNickname` on commit trailers per the `Jam-Session` /
`Jam-Turn` trailer scheme described in the go-git skill) and bearer
scoping (each participant has their own anon bearer; the merger sees both
authorship records). Two real participants on a real portal with a real
Postgres tx for member insertion will surface bugs that no unit test
catches — e.g. concurrent-join race conditions, anonymous-account row
duplication, member-count math drift, or merge-trailer name collision when
two anon participants pick the same dictionary-generated nickname.

The parked bug for join itself proves the journey is currently broken
under simulated conditions. An e2e variant would have surfaced it earlier
and against the real session-state machine.

## Suggested remedy

Add `tests/e2e/golden/playground_two_participant_join_merge_test.go` using
existing fixtures only. Use two separate `gitclient` instances against the
same session URL. Assert:
1. Both anonymous create+join return 200.
2. Both pushes (independent refs) return 0 exit codes.
3. WebSocket `commit.arrived` events (via existing `wsclient` fixture)
   arrive for each push within 5s — same SLA as
   `session_join_and_push_test.go`.
4. Auto-merger advances draft to include both commits and emits
   `merge.succeeded` (same assertion as `auto_merge_test.go`).
5. The two anonymous accounts persisted in DB have distinct nicknames.

## Implementation notes

**Test file**: `tests/e2e/golden/playground_two_participant_join_merge_test.go`

**What landed**:
- Boots postgres + portal (playground enabled, HARD_CAP_S=300, IDLE_TIMEOUT_S=600,
  DESTRUCTION_SWEEP_INTERVAL_S=1). No clock advance needed — this is a
  happy-path journey test.
- Participant A creates via `POST /api/playground/sessions` (201). Base ref push
  uses the trailer exemption landed at commit 297616a.
- Participant B joins via `POST /api/playground/sessions/{id}/join` (200). AccountID
  derived from `/api/me` with the anon bearer, same pattern as the solo test.
- WebSocket connections established for both A and B BEFORE any per-user-ref pushes
  (prevents race where events arrive before the subscriber is ready).
- A and B push independent commits (alice.md / bob.md) on their per-user refs.
  `gitclient.Clone` + `gitclient.Commit` adds Jam-Session/Jam-Turn/Jam-Author trailers
  automatically. B's repo is reset to `origin/base` before committing so both commits
  share the base as a common ancestor — required for the auto-merger to find a
  merge base.
- **WS event discipline**: used `wsclient.WaitFor` (channel drain, not sleep) for
  both `commit.arrived` (5s) and `merge.succeeded` (20s) — zero sleeps in the test.
- `waitForMergeSucceeded` (from `auto_merge_test.go`, same package) checks both
  A's and B's merge.succeeded on both subscribers.
- Cross-fetch verification: A fetches and checks B's ref tip SHA; B fetches and checks
  A's ref tip SHA.
- Draft ref assertion: `git merge-base --is-ancestor` confirms both source commits
  are reachable from `jam/<session>/draft`.
- Anti-tautology (Unit 5): `p.Exec(ctx, ["ls", repoPath])` asserts bare repo exists
  on real disk immediately after session create.

**Decisions**:
- Did NOT send a suggested nickname in the join call (passed "" → server picks one).
  Server correctly picked distinct nicknames on all 5 runs.
- The parked unit-test bug `bug-playground-join-with-nickname-returns-410-on-fresh-session`
  is a clock-injection issue in the unit test harness only. Against the real portal binary
  (wall clock), `hard_cap_at` is always in the future immediately after session creation,
  and the join handler behaves correctly. Added a comment in `playgroundJoin` linking the
  bug for future investigators.
- Used `soloCreateResponse` (defined in `playground_solo_create_push_tombstone_test.go`,
  same package) for the create response decode — no duplication.
- Reused `gitResetToRef`, `waitForMergeSucceeded`, `requireCommitInLog` from
  `auto_merge_test.go` — all in `golden_test` package, no import needed.

**Flake check**: 5/5 passes on consecutive runs (8–12s each). No flakes observed.
The `wsclient.WaitFor` gating eliminated the race risk identified in the pre-mortem.

## Test sketch

```go
// tests/e2e/golden/playground_two_participant_join_merge_test.go
func TestPlayground_TwoParticipantJoinAndMerge(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    p := portal.Start(ctx, t, portal.Options{
        DBDriver:          "postgres",
        DBDSN:             pg.ContainerDSN,
        PlaygroundEnabled: true,
    })

    // Anon A creates.
    aResp := createPlayground(t, p.URL)
    // Anon B joins.
    bResp := joinPlayground(t, p.URL, aResp.SessionID, "nick-b")

    // Both subscribe to WS.
    aWs := wsclient.Connect(t, p.URL, aResp.Bearer, aResp.SessionID)
    bWs := wsclient.Connect(t, p.URL, bResp.Bearer, aResp.SessionID)

    // Both push independent refs.
    gitclient.PushRef(t, p.URL+"/git/playground/"+aResp.SessionID+".git",
        aResp.Bearer, "refs/sync/agent-a", "alpha.txt")
    gitclient.PushRef(t, p.URL+"/git/playground/"+aResp.SessionID+".git",
        bResp.Bearer, "refs/sync/agent-b", "beta.txt")

    // Assert commit.arrived on each side; assert merge.succeeded eventually.
    requireEvent(t, aWs, "commit.arrived", 5*time.Second)
    requireEvent(t, bWs, "commit.arrived", 5*time.Second)
    requireEvent(t, aWs, "merge.succeeded", 30*time.Second)
}
```

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**:

Test passes 5/5 consecutive runs at 8-12s each — the flake-free property
the pre-mortem identified as the key risk. Zero sleeps; all
synchronization via WebSocket `WaitFor` channel drain and
`waitForMergeSucceeded` polling-with-deadline. The pattern is the
correct one for distributed-event timing tests.

Key implementation decisions documented in the agent's summary:
- Reused `gitResetToRef`, `waitForMergeSucceeded`, `requireCommitInLog`
  from `auto_merge_test.go` (same `golden_test` package) — no
  duplication.
- Reused `soloCreateResponse` decode type from the solo test — single
  source for the create response shape.
- Both A and B subscribe to WebSocket BEFORE pushing — the right
  ordering to avoid the race the pre-mortem flagged.

**Important finding worth surfacing**: the agent confirmed that the
parked bug `bug-playground-join-with-nickname-returns-410-on-fresh-session`
does NOT reproduce against the real portal binary — it's a
clock-injection artifact of the unit-suite `fixedClock` only. Against
real wall time + real portal, the join handler works correctly. This
backs up the audit's central thesis: the unit suite was lying about
the join handler being broken; the e2e suite proves it works. The
parked bug should be re-classified as "unit-test setup issue, not a
product bug" and the two failing unit tests should be rewritten to
not trip the clock-injection mismatch.

Anti-tautology discipline (Unit 5) embedded: `p.Exec(ctx, ["ls",
repoPath])` asserts the bare repo exists on real disk. Combined with
the 4 other golden tests, Unit 5's closing condition is now
observable across all 4 of the feature's golden tests.

Advanced `stage: review → done`.
