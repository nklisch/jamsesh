---
id: epic-portal-api-websocket-gateway-handler-and-fanout
kind: story
stage: implementing
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

- [ ] Subprotocol-token upgrade: `Sec-WebSocket-Protocol: jamsesh.bearer.<token>` accepted on valid token + membership
- [ ] Bad token → 401; non-member → 403; unknown session → 404
- [ ] Live fan-out: emit commit.arrived → connected client receives EventEnvelope
- [ ] Replay-from-cursor: first frame `{"replay_from": <seq>}` → server streams events with seq>replay_from then transitions to live
- [ ] Slow consumer: per-conn send buffer of 64; overflow → close with 1008
- [ ] Heartbeat: 30s ping interval; no pong → close
- [ ] Origin enforcement: AllowOrigins from config; empty → deny all

## Notes

- The `coder-websocket` skill carries verified patterns. Use them.
- events.Log.Subscribe("") (empty filter = all) is the right primitive for the gateway.
- Each conn has `sessionID`, `orgID`, `account`, `send chan events.Event`.
- The Start(ctx) goroutine reads from Log.Subscribe channel, looks up conns for the event's sessionID, non-blocking sends.
- Use `wsjson.Write` for sending EventEnvelope JSON to clients.
- For the EventEnvelope shape: the openapi-generated `EventEnvelope` struct uses `EventEnvelope_Payload` (a wrapped RawMessage). For sending over WS, marshal an inline struct `{seq, version: 1, type, payload (raw json), timestamp, session_id}` directly — simpler than using the generated EventEnvelope_Payload wrapper.
