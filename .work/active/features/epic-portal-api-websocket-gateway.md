---
id: epic-portal-api-websocket-gateway
kind: feature
stage: implementing
tags: [portal]
parent: epic-portal-api
depends_on: [epic-portal-api-events-log, epic-portal-foundation-http-skeleton, epic-portal-foundation-tokens]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
---

# Portal API — WebSocket Gateway

## Brief

The real-time push surface for portal UI clients. Per-session
subscriptions, per-connection fan-out, replay-from-cursor for clients
reconnecting after disconnect, heartbeat/ping for liveness detection,
backpressure handling for slow consumers.

**Route**: `GET /ws/sessions/<session_id>` (one connection = one session
subscription; want multiple sessions, open multiple connections).

**Upgrade-time auth** (locked at epic-design): client sends
`Sec-WebSocket-Protocol: jamsesh.bearer.<token>` in the upgrade
request. Server validates the token via the foundation tokens feature,
verifies the account is a member of `session_id`, accepts the upgrade
echoing the chosen subprotocol. Rejects with 401/403 on auth/membership
failure. Rationale: browser WebSocket API doesn't allow custom
Authorization headers; subprotocol avoids tokens in URLs (which would
log to access logs/history).

**Subscription mechanics**:

- On upgrade success, server registers the connection in an in-memory
  per-session subscription set.
- Client may optionally send an initial `replay-from: <seq>` text frame.
  Server reads events from the `events` table where `seq > <replay-from>`
  and `session_id = <session>`, streams them to the client in order.
- After replay (or immediately if no replay), server transitions to
  live mode — each `EmitEvent` call into the events-log feature fans
  out to all subscribers of that session_id via an in-process
  notification channel.

**Library** (locked at epic-design): `nhooyr.io/websocket`. Modern,
context-aware API; smaller surface than `gorilla/websocket`; the gorilla
project recommends nhooyr for new code.

**Disconnect handling**:

- Client disconnect cleanly: connection removed from subscription set.
- Network drop / silent client: detected by a 30-second heartbeat ping;
  no pong → close + cleanup.
- Slow consumer: per-connection bounded send buffer; if the buffer fills,
  the connection is closed with `1008` (policy violation, "subscriber too
  slow"). Client reconnects with `replay-from: <last_seq>`.

**Fan-out wiring**: this feature exposes a `Notify(sessionID, env
EventEnvelope)` function the events-log feature calls after every
successful `EmitEvent`. The function pushes the envelope to each
subscribed connection's send buffer non-blockingly.

Does NOT cover the events table or emission helpers (events-log feature
owns those). Does NOT cover session-membership validation logic beyond
the upgrade-time check (sessions-rest owns membership management).

## Epic context

- Parent epic: `epic-portal-api`
- Position in epic: depends on events-log for the event stream and the
  emission hook; depends on http-skeleton for the route mount and
  middleware shape; depends on tokens for the upgrade-time validation
  helper.

## Foundation references

- `docs/PROTOCOL.md` — WebSocket event types (the canonical event catalog
  this gateway pushes)
- `docs/ARCHITECTURE.md` — WebSocket gateway component, Data flow
- `docs/SECURITY.md` — Authentication (Bearer token authority extended
  to ws upgrade), What a single-user-token compromise exposes

## Inherited epic design decisions

- **WebSocket library**: `nhooyr.io/websocket`.
- **Subscription model**: per-connection per-session, path-based.
- **Auth at upgrade**: subprotocol-encoded token
  (`Sec-WebSocket-Protocol: jamsesh.bearer.<token>`).
- **Event envelope**: `version: 1`, shape `{seq, version, type, payload,
  timestamp, session_id}`.

## Generated-contracts scope

Per the SPEC.md generated-contracts decision, this feature consumes
(rather than authors) the event-payload schemas defined by
`epic-portal-api-events-log` in `docs/openapi.yaml > components/schemas`:
`EventEnvelope` and the per-event payload schemas. Marshaling on the
Go side uses the oapi-codegen-generated structs; the TypeScript client
unmarshals into the openapi-typescript-generated discriminated union on
`type`. WebSocket itself is not described by OpenAPI, but the
**payload** types ARE — so REST responses, MCP tool returns, and
WebSocket events all share the same typed shapes across both runtimes.

## Decomposition risks

- WebSocket gateway is the highest-risk feature in this epic. Replay-
  from-cursor with bounded retention, per-session fan-out fairness, and
  backpressure under slow consumers are subtle. Design pass produces an
  explicit lifecycle diagram and a replay/backpressure test plan.

## Design decisions

- **Library**: `github.com/coder/websocket` (the maintained fork of nhooyr; locked per research doc + auto-loaded `coder-websocket` skill). The feature body's mention of `nhooyr.io/websocket` is stale; this is the rolling-foundation update.
- **events.Log Subscribe**: already shipped by the auto-merger worker story. Reuse it — the WS gateway is a sibling subscriber.
- **Per-session subscription set**: `gateway` struct with `map[sessionID]map[*conn]bool` guarded by RWMutex. On each subscribed event, fan out to all matching conns.
- **Replay-from-cursor**: client's first frame after upgrade MAY be `{"replay_from": <int>}` JSON. Server reads events via `Log.ListSince(sessionID, replayFrom, limit=1000)` and streams them, then transitions to live mode.
- **Heartbeat**: 30s ping interval via `conn.Ping(ctx)`; on Ping error → close.
- **Slow-consumer protection**: per-conn send buffer of 64 events; on full → close with status 1008.
- **Package**: `internal/portal/wsgateway/`.
- **Single story**: cohesive feature.

## Implementation Units

### Unit 1: Gateway struct + upgrade handler

**File**: `internal/portal/wsgateway/gateway.go`
**Story**: `epic-portal-api-websocket-gateway-handler-and-fanout`

```go
package wsgateway

import (
    "context"
    "net/http"
    "sync"

    "github.com/coder/websocket"
    "github.com/coder/websocket/wsjson"
    "github.com/go-chi/chi/v5"

    "jamsesh/internal/db/store"
    "jamsesh/internal/portal/events"
    "jamsesh/internal/portal/tokens"
)

type Gateway struct {
    Store        store.Store
    Tokens       tokens.Service
    Log          *events.Log
    AllowOrigins []string
    
    mu   sync.RWMutex
    subs map[string]map[*conn]struct{}  // sessionID -> set of conns
}

func (g *Gateway) Handler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        sessionID := chi.URLParam(r, "sessionID")
        
        // Auth: read Sec-WebSocket-Protocol
        proto := r.Header.Get("Sec-WebSocket-Protocol")
        token, ok := strings.CutPrefix(proto, "jamsesh.bearer.")
        if !ok { http.Error(w, "unauthorized", 401); return }
        
        account, err := g.Tokens.Validate(r.Context(), token)
        if err != nil { http.Error(w, "unauthorized", 401); return }
        
        // Verify membership — need session's org_id; look up session by orgID+sessionID requires the org id. Add an alternate query? Or scan all account orgs.
        // Simplest: walk account's session memberships via ListSessionMembershipsForAccount and check sessionID is in the set.
        memberships, err := g.Store.ListSessionMembershipsForAccount(r.Context(), account.ID)
        if err != nil { http.Error(w, "server error", 500); return }
        ok = false
        var orgID string
        for _, m := range memberships {
            if m.SessionID == sessionID { ok = true; orgID = m.OrgID; break }
        }
        if !ok { http.Error(w, "forbidden", 403); return }
        
        ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
            Subprotocols: []string{proto},
            OriginPatterns: g.AllowOrigins,
        })
        if err != nil { return }
        defer ws.CloseNow()
        
        c := &conn{ws: ws, sessionID: sessionID, orgID: orgID, account: account, send: make(chan events.Event, 64)}
        g.register(c)
        defer g.unregister(c)
        
        // Optional replay-from: read first frame with 5s deadline
        replayCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        var hdr struct{ ReplayFrom int64 `json:"replay_from"` }
        if err := wsjson.Read(replayCtx, ws, &hdr); err == nil && hdr.ReplayFrom > 0 {
            // Stream replay
            events, _ := g.Log.ListSince(r.Context(), sessionID, hdr.ReplayFrom, 1000)
            for _, e := range events {
                wsjson.Write(r.Context(), ws, envelopeOf(e))
            }
        }
        cancel()
        
        // Heartbeat ticker
        go g.heartbeat(r.Context(), c)
        
        // Fanout loop: read from send chan, write to ws
        for {
            select {
            case <-r.Context().Done():
                return
            case e, ok := <-c.send:
                if !ok { return }
                if err := wsjson.Write(r.Context(), ws, envelopeOf(e)); err != nil {
                    return
                }
            }
        }
    }
}
```

### Unit 2: Event subscription wire-up

`Start(ctx)`: subscribes to `Log.Subscribe("")` for all event types; on each event, looks up the per-session subscription set and non-blocking-sends to each conn's `send` channel; on full send → close conn with 1008.

### Unit 3: Route registration

In `cmd/portal/main.go`: construct Gateway with allowed origins from config. Register `r.Get("/ws/sessions/{sessionID}", gateway.Handler())` via `router.Deps.MountWS`.

## Implementation Order

Single story.

## go.mod additions

- `github.com/coder/websocket@v1.8.14` (per research doc pin)

## Testing

- httptest server + `websocket.Dial` from test
- Successful upgrade + membership check
- 401 on bad token
- 403 on non-member
- Live event fan-out: emit event → connected client receives envelope
- Replay-from-cursor: emit N events, connect with replay_from=K, verify K+1..N delivered + live events follow
- Slow-consumer: don't read from client; emit > 64 events; conn closed with 1008

## Risks

- **Origin enforcement**: production deployment must set `AllowedOrigins` from config. Default to empty (deny all) in v1 to surface misconfiguration loudly. SELF_HOST.md operator note.
- **Subscribe filter all-events**: the gateway gets all event types and filters by session. Alternative: per-session subscriptions in events.Log. v1 picks the simpler global subscribe path; revisit at scale.
