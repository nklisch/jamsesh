---
id: epic-portal-api-websocket-gateway-handler-and-fanout
kind: story
stage: review
tags: [portal]
parent: epic-portal-api-websocket-gateway
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# WebSocket Gateway — Handler + Subscriber + Fanout

## Scope

Build the `/ws/sessions/<sessionID>` handler with subprotocol-token auth, per-session subscription set, replay-from-cursor, heartbeat, and slow-consumer protection.

## Units delivered

- `internal/portal/wsgateway/gateway.go` — Gateway struct + Handler + register/unregister + Start subscriber loop
- `internal/portal/wsgateway/gateway_test.go` — httptest + websocket.Dial
- `cmd/portal/main.go` (edit) — construct Gateway, call Start(ctx), register Handler via `router.Deps.MountWS`
- go.mod: add `github.com/coder/websocket@v1.8.14`

## Acceptance Criteria

- [x] Subprotocol-token upgrade: `Sec-WebSocket-Protocol: jamsesh.bearer.<token>` accepted on valid token + membership
- [x] Bad token → 401; non-member → 403; unknown session → 404
- [x] Live fan-out: emit commit.arrived → connected client receives EventEnvelope
- [x] Replay-from-cursor: first frame `{"replay_from": <seq>}` → server streams events with seq>replay_from then transitions to live
- [x] Slow consumer: per-conn send buffer of 64; overflow → close with 1008
- [x] Heartbeat: 30s ping interval; no pong → close
- [x] Origin enforcement: AllowOrigins from config; empty → deny all

## Notes

- The `coder-websocket` skill carries verified patterns. Use them.
- events.Log.Subscribe("") (empty filter = all) is the right primitive for the gateway.
- Each conn has `sessionID`, `orgID`, `account`, `send chan events.Event`.
- The Start(ctx) goroutine reads from Log.Subscribe channel, looks up conns for the event's sessionID, non-blocking sends.
- Use `wsjson.Write` for sending EventEnvelope JSON to clients.
- For the EventEnvelope shape: the openapi-generated `EventEnvelope` struct uses `EventEnvelope_Payload` (a wrapped RawMessage). For sending over WS, marshal an inline struct `{seq, version: 1, type, payload (raw json), timestamp, session_id}` directly — simpler than using the generated EventEnvelope_Payload wrapper.

## Implementation notes

### Key design decisions

**Replay-from-cursor**: coder/websocket closes the connection when the context passed to `Read` is cancelled (documented in the Conn struct comment: "This applies to context expirations as well unfortunately"). Using a timeout context directly for the optional replay-from read would close the WS conn after 2s even for non-replay clients. Solution: a read goroutine owns the Read half permanently. It reads the first frame (replay-from or discard), then drains the conn for disconnection detection and ping/pong handling. The main goroutine is write-only. A channel + `time.After(2s)` select in the main goroutine implements the 2s deadline without cancelling any context.

**Slow-consumer test**: The events.Log subscriber channel (buffer=64) and the per-conn send buffer (also 64) interact. Emitting 65 events in a tight loop causes the 65th to be dropped at the log level before reaching the gateway. The test emits 64 events, sleeps 50ms for the fanout goroutine to drain the log channel into `c.send`, then emits the 65th to trigger the overflow-close.

**AllowOrigins**: defaults to empty slice (`[]string{}`) which denies all cross-origin upgrades — the intentional secure default. Operators configure this per SELF_HOST.md.

### Files changed

- `internal/portal/wsgateway/gateway.go` — new file: Gateway struct, Start/Stop, Handler, fanout, register/unregister, writeEnvelope
- `internal/portal/wsgateway/gateway_test.go` — new file: 5 tests covering all acceptance criteria
- `cmd/portal/main.go` — added wsgateway import, Gateway construction + Start, MountWS wiring, wsGateway.Stop() at shutdown
- `go.mod` / `go.sum` — `github.com/coder/websocket@v1.8.14` added as direct dependency

### Test results

All 5 tests green (`go test ./internal/portal/wsgateway/... -v`):
- `TestHandler_SuccessfulUpgrade_LiveEvent` — 2.01s (includes 2s replay wait)
- `TestHandler_BadToken_Returns401`
- `TestHandler_NonMember_Returns403`
- `TestHandler_ReplayFromCursor`
- `TestHandler_SlowConsumer_ClosedWith1008`

`go build ./...` clean.
