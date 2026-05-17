---
id: epic-portal-api-websocket-gateway
kind: feature
stage: drafting
tags: [portal]
parent: epic-portal-api
depends_on: [epic-portal-api-events-log, epic-portal-foundation-http-skeleton, epic-portal-foundation-tokens]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
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

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
