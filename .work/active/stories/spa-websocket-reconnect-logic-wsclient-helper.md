---
id: spa-websocket-reconnect-logic-wsclient-helper
kind: story
stage: implementing
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
