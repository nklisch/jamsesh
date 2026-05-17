---
id: spa-websocket-reconnect-logic-wsclient-helper
kind: story
stage: review
tags: [testing]
parent: spa-websocket-reconnect-logic
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E wsclient — ConnectFromSeq helper

## Scope

Extend the Go e2e WebSocket fixture at
`tests/e2e/fixtures/wsclient/wsclient.go` with a `ConnectFromSeq`
helper that opens a session WebSocket and writes a `replay_from`
first frame so the gateway replays missed events with `seq > replaySeq`
before transitioning to live mode. This unlocks e2e scenarios that
drive reconnect-with-replay flows from the Go layer in addition to
Playwright.

No SPA dependency — this is a Go-only fixture extension and can ship
in parallel with the SPA-side stories.

## Files touched

- `tests/e2e/fixtures/wsclient/wsclient.go` (edit) — add
  `ConnectFromSeq` alongside the existing `Connect`. Refactor the
  shared dial-and-readloop path into a small `dial` helper so both
  functions reuse it.
- `tests/e2e/fixtures/wsclient/wsclient_test.go` (new or edit if it
  already exists) — table test that exercises the helper against a
  real portal Testcontainer fixture: seed N events, connect with
  `replaySeq = 1`, assert seqs 2..N arrive in order.

## Specification

### API

```go
// Connect (existing) — opens a live stream, no replay frame.
func Connect(ctx context.Context, t *testing.T, portalURL, sessionID, bearer string) *Client

// ConnectFromSeq behaves like Connect but additionally writes a
// {"replay_from": replaySeq} text frame as the first message after
// the WebSocket handshake, so the gateway replays missed events with
// seq > replaySeq before transitioning to live mode.
//
// If replaySeq <= 0, ConnectFromSeq is equivalent to Connect (no
// frame is sent).
func ConnectFromSeq(ctx context.Context, t *testing.T, portalURL, sessionID, bearer string, replaySeq int64) *Client
```

### Internals

Both `Connect` and `ConnectFromSeq` share the dial-and-register path.
Sketch:

```go
func dial(ctx context.Context, t *testing.T, portalURL, sessionID, bearer string) (*Client, *websocket.Conn) {
    t.Helper()
    wsURL := strings.Replace(portalURL, "http://", "ws://", 1) +
        "/ws/sessions/" + sessionID
    proto := "jamsesh.bearer." + bearer
    conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
        Subprotocols: []string{proto},
    })
    if err != nil {
        t.Fatalf("wsclient: dial %s: %v", wsURL, err)
    }
    cctx, cancel := context.WithCancel(ctx)
    c := &Client{
        conn:   conn,
        events: make(chan Event, 64),
        cancel: cancel,
        done:   make(chan struct{}),
    }
    t.Cleanup(c.Close)
    return c, conn
}

func Connect(ctx context.Context, t *testing.T, portalURL, sessionID, bearer string) *Client {
    c, _ := dial(ctx, t, portalURL, sessionID, bearer)
    go c.readLoop(/* derived ctx */)
    return c
}

func ConnectFromSeq(ctx context.Context, t *testing.T, portalURL, sessionID, bearer string, replaySeq int64) *Client {
    c, conn := dial(ctx, t, portalURL, sessionID, bearer)
    if replaySeq > 0 {
        type replayHdr struct {
            ReplayFrom int64 `json:"replay_from"`
        }
        if err := wsjson.Write(ctx, conn, replayHdr{ReplayFrom: replaySeq}); err != nil {
            t.Fatalf("wsclient.ConnectFromSeq: write replay_from: %v", err)
        }
    }
    go c.readLoop(/* derived ctx */)
    return c
}
```

The exact ctx-derivation pattern matches the existing implementation
(separate `cctx, cancel := context.WithCancel(ctx)` for the readLoop).

The replay frame is written BEFORE `readLoop` starts so the Read half
is free when we call `wsjson.Write`. Per the
`internal/portal/wsgateway/gateway.go:225` comment, the portal expects
the first frame to be the replay header — only the WS write half is
used here, which doesn't conflict with the Read in the portal's reply
goroutine.

## Acceptance criteria

- [ ] `ConnectFromSeq` writes a single `{"replay_from": N}` text
      frame before starting the read loop when `replaySeq > 0`.
- [ ] `ConnectFromSeq` is equivalent to `Connect` (no frame written)
      when `replaySeq <= 0`.
- [ ] The shared `dial` helper is the single source of truth for the
      auth + URL construction; `Connect` and `ConnectFromSeq` both
      call it. Behaviour of the existing `Connect` is unchanged.
- [ ] A `wsclient_test.go` test drives the helper against a portal
      fixture, seeds events 1..3, and asserts seqs 2 and 3 arrive
      when called with `replaySeq = 1`.
- [ ] All existing Go tests in `tests/e2e/...` continue to pass.

## Test approach

The new fixture test depends on the `portal` Testcontainer fixture
(`tests/e2e/fixtures/portal`). Pattern:

1. Spin up the portal + an authenticated test session via the
   existing fixtures.
2. Use the portal's REST API (or events.Log direct injection if
   that's exposed in the test seam — verify during implementation)
   to emit 3 events with seqs 1, 2, 3.
3. Call `ConnectFromSeq(ctx, t, portalURL, sessionID, bearer, 1)`.
4. Call `WaitFor` repeatedly; assert two events arrive with seqs 2
   and 3 before any live event.
5. Optional: emit a live event (seq 4) post-connection and assert it
   too is delivered.

If the seed-events seam doesn't yet exist in fixtures, the test can
fall back to driving real events via `git push` through the portal's
smart-HTTP route (which the golden-path tests already exercise).

## Notes

- This is the Go-side mirror of the SPA's replay-on-reconnect; both
  use the exact same wire format (`{"replay_from": N}` text frame).
- The Go gateway test at `internal/portal/wsgateway/gateway_test.go:297`
  already proves the server side end-to-end; this story surfaces the
  same capability in the e2e fixtures layer for consumers higher up
  the stack.
- Tag is `testing` (not `ui`) because no UI is touched.

## Implementation notes

### Files touched

- `tests/e2e/fixtures/wsclient/wsclient.go` — added `ConnectFromSeq` and
  extracted a private `dial` helper that both it and the existing `Connect`
  now call. `Connect`'s public signature is unchanged. The `Client` struct
  gained an unexported `readCtx` field so `dial` can construct the derived
  context once and hand it to either entry point's `go c.readLoop(...)` call.
- `tests/e2e/fixtures/wsclient/wsclient_test.go` (new) — four table-style
  tests exercising the helpers against an in-process `httptest.Server` with
  a stub WebSocket handler that mirrors the relevant slice of the portal
  gateway's wire contract (subprotocol-bearer auth + first-frame
  `replay_from` parsing + seq-ordered replay emission).

### Design discrepancy: in-process stub, not the portal Testcontainer

The story's "Test approach" section sketches a test that spins up the real
portal Testcontainer fixture and seeds events via `events.Log` or smart-HTTP
push. That requires Docker and the `jamsesh/portal:e2e` image, neither of
which is available in the substrate's default CI/agent environment — the
portal fixture's `requireDocker` / `requirePortalImage` guards would `t.Skip`
the test, defeating the verification.

The portal-side wire contract is already covered end-to-end by
`internal/portal/wsgateway/gateway_test.go::TestHandler_ReplayFromCursor`,
which seeds events via `events.Log.Emit` and drives the same `{"replay_from":
N}` first-frame protocol against a real in-process handler. What this story
needs to verify is the **client** half: that `ConnectFromSeq` produces
exactly the expected wire payload (`{"replay_from": N}` for `N > 0`, nothing
for `N <= 0`), shares the dial path with `Connect`, and routes replay
events into the same `Events()` channel consumers already use.

An in-process `httptest.Server` with a stub gateway handler is the right
granularity for that contract — it lets the test run on every developer
machine without Docker, and it pins the client-side wire format
independently of the portal binary's evolution. The Docker-gated golden
flows in `tests/e2e/golden/` will exercise `ConnectFromSeq` against the real
portal once a consumer story un-skips a flow that needs replay (e.g. the
`ws_reconnect_after_drop` subtest in
`tests/e2e/failure/interrupted_ops_test.go`).

### Internal shape

The `dial` helper returns a fully-constructed `Client` with its read loop
NOT yet started. Both `Connect` and `ConnectFromSeq` are responsible for
firing `go c.readLoop(c.readCtx)` — `ConnectFromSeq` does so AFTER
`wsjson.Write` of the replay frame, so the read half is free for the
portal's first-frame reader goroutine.

A small correctness note: if the replay-frame write fails, the function now
starts the read loop before calling `t.Fatalf`. Without that, the
`t.Cleanup`-registered `Close()` would block forever on `<-c.done`
during test teardown because the read loop's `defer close(c.done)` would
never run.

### Verification

```
$ cd tests/e2e
$ go build ./fixtures/wsclient/...           # clean
$ go vet ./fixtures/wsclient/...             # clean
$ go test ./fixtures/wsclient/... -v -timeout 60s
=== RUN   TestConnect_NoReplayFrame
--- PASS: TestConnect_NoReplayFrame (0.30s)
=== RUN   TestConnectFromSeq_WritesReplayFrame
--- PASS: TestConnectFromSeq_WritesReplayFrame (0.00s)
=== RUN   TestConnectFromSeq_ZeroSeqDoesNotWriteFrame
--- PASS: TestConnectFromSeq_ZeroSeqDoesNotWriteFrame (0.30s)
=== RUN   TestConnectFromSeq_NegativeSeqDoesNotWriteFrame
--- PASS: TestConnectFromSeq_NegativeSeqDoesNotWriteFrame (0.30s)
PASS
ok      jamsesh/tests/e2e/fixtures/wsclient     0.906s
```

The whole `tests/e2e` module still builds cleanly (`go build ./...`).
