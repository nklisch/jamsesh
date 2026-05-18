---
id: gate-tests-wsclient-cursor-replay-fixture
kind: story
stage: drafting
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
