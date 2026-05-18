---
id: gate-tests-wsclient-cursor-replay-fixture
kind: story
stage: review
tags: [testing, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# `interrupted_ops_test.go` cursor-replay subtest skipped with bug-ref to absent helper

## Priority
Medium

## Spec reference
`spa-websocket-reconnect-logic-replay-from` (story body): the portal
gateway supports `replay_from`, but the wsclient fixture only supports a
basic subscribe-from-now connection.

## Gap type
test-integrity. `tests/e2e/failure/interrupted_ops_test.go:305` skips
referencing `wsclient.ConnectFromSeq` — but no such story or helper
exists in the bound set. The replay-from path is a core SPA correctness
invariant; the e2e seam is silent.

## Suggested test
Extend the wsclient fixture with `ConnectFromSeq`, then convert the skip:

```go
// TestInterruptedOps_WSCursorReplay_ResumesMissedEvents
//   Subscribe at seq 0; receive seq 1..5; drop connection.
//   wsclient.ConnectFromSeq(ctx, t, url, sess, bearer, fromSeq=3).
//   Assert: receive seq 6+ AND replay frames for seq 4, 5 in order.
```

## Test location (suggested)
`tests/e2e/failure/interrupted_ops_test.go` and
`tests/e2e/fixtures/wsclient/wsclient.go`

## Implementation notes

`wsclient.ConnectFromSeq` and its unit tests were already fully implemented in
`tests/e2e/fixtures/wsclient/wsclient.go` and `wsclient_test.go` — the fixture
gap in the story description had already been resolved before this story was
picked up. The only remaining gap was the unskipped subtest body.

Changes made:

- `tests/e2e/failure/interrupted_ops_test.go`: added `wsclient` import; replaced
  the `t.Skip(...)` in `TestInterruptedOps/ws_reconnect_after_drop` with a
  full test that:
  1. Signs up Alice, creates an org + session (emits seq 1 `session.created`).
  2. Connects a first WS client and emits 5 `mode.changed` events via
     `POST /api/orgs/{orgID}/sessions/{sessionID}/ref-modes` (seqs 2–6).
  3. Waits for the first 2 live events (seqs 2, 3) on the first client, then
     closes it (simulates mid-burst disconnect).
  4. Reconnects with `wsclient.ConnectFromSeq(..., lastSeenSeq=3)` — the
     gateway replays events with seq > 3 (seqs 4, 5, 6) in ascending order.
  5. Emits one more `mode.changed` event and verifies it arrives live on the
     reconnected client (proves live mode is active after replay drains).

Test result: `PASS` (`TestInterruptedOps/ws_reconnect_after_drop`, 2.02s)
against the full Docker-backed e2e stack.
