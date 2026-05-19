---
id: bug-wsclient-fixture-sends-bearer-not-ticket
kind: story
stage: implementing
tags: [bug, websocket, e2e-test, auth]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Bug: wsclient e2e fixture sends old bearer subprotocol; gateway now requires ticket

## Brief

`TestRuntimeAndClock/automerger_pause` fails at `wsclient.Connect`:

```
wsclient: dial ws://...: failed to WebSocket dial: expected handshake
response status code 101 but got 401
```

(`tests/e2e/chaos/runtime_and_clock_test.go:171-172`, calls
`wsclient.Connect(ctx, t, p.URL, sessionID, alice.AccessToken)`)

## Root cause

The `gate-security-ws-bearer-token-ticket-flow` story (shipped in v0.1.0,
commit `2301005`) changed the WebSocket authentication protocol from:

- Old: `Sec-WebSocket-Protocol: jamsesh.bearer.<bearer-token>`
- New: `Sec-WebSocket-Protocol: jamsesh-ticket.<ticket>` (ticket obtained via
  `POST /api/auth/ws-ticket`)

The gateway (`internal/portal/wsgateway/gateway.go:148`) now does:
```go
ticketVal, ok := strings.CutPrefix(proto, "jamsesh-ticket.")
```
and returns 401 for any other format (including the old bearer format).

The `wsclient` e2e fixture (`tests/e2e/fixtures/wsclient/wsclient.go:94-102`)
was NOT updated in that commit (verified via `git log
tests/e2e/fixtures/wsclient/wsclient.go` — last touched by `3b423a0`,
predating 2301005). It still sends:
```go
proto := "jamsesh.bearer." + bearer  // old format — now rejected with 401
```

The gateway implementation note in the story explicitly states "There is no
backwards-compat path — raw bearer format is rejected with 401."

## Fix site

`tests/e2e/fixtures/wsclient/wsclient.go:94-102` — the `dial` function.

The fix replaces the bearer-in-subprotocol flow with the two-step ticket flow:
1. `POST /api/auth/ws-ticket` with `Authorization: Bearer <bearer>` → get `ticket`.
2. Set `Sec-WebSocket-Protocol: jamsesh-ticket.<ticket>` when dialing.

The `dial` function needs to accept the portal base URL (already passed as
`portalURL` to `Connect`) and call the ws-ticket endpoint before dialing.
The `ws-ticket` endpoint is at `/api/auth/ws-ticket` (POST, requires Bearer
auth, returns `{"ticket": "...", "expires_in_seconds": 60}`).

The ticket is single-use and 60-second TTL, so it must be fetched immediately
before each `websocket.Dial` call. `ConnectFromSeq` reuses `dial`, so both
connect paths are fixed by fixing `dial`.

## File:line pointers

- `tests/e2e/fixtures/wsclient/wsclient.go:94-116` — `dial` function; replace
  `proto := "jamsesh.bearer." + bearer` with ticket-fetch + ticket subprotocol
- `tests/e2e/fixtures/wsclient/wsclient.go:97` — `wsURL` construction is fine
- `internal/portal/wsgateway/gateway.go:147-158` — gateway expectation
  (no change needed here — the gateway is correct)
- The ws-ticket response type is `{"ticket": string, "expires_in_seconds": int}`
  (see `internal/api/openapi/server.gen.go` around `WsTicketResponse`)

## Acceptance criteria

- [ ] `wsclient.Connect` and `wsclient.ConnectFromSeq` successfully upgrade to
      101 when the caller holds a valid bearer token.
- [ ] `TestRuntimeAndClock/automerger_pause` no longer fails at the WS dial step.
- [ ] A `wsclient.Connect` call with an invalid bearer token still fails
      (the ticket endpoint rejects it, so the dial is never attempted).
- [ ] No production code changes (this is a test-fixture-only fix).
