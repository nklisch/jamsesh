---
id: e2e-audit-playground-two-participant-join-merge-journey
kind: story
stage: drafting
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
