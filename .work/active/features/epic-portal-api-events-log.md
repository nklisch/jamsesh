---
id: epic-portal-api-events-log
kind: feature
stage: drafting
tags: [portal]
parent: epic-portal-api
depends_on: [epic-portal-foundation-data-layer]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal API — Event Log

## Brief

The shared event log every other piece of the portal-api epic reads from
or writes into. Owns the `events` and `presence` tables (extending the
foundation data layer), the monotonic per-session sequence-number
generator, and the emission helpers that producers across the codebase
call (`post-receive` from `epic-portal-git`, the auto-merger from
`epic-auto-merger`, comments/MCP/REST handlers from sibling features
here).

**Schema additions** (sqlc migrations owned by this feature, applied on
top of data-layer's initial schema):

- `events` — `id`, `org_id`, `session_id`, `seq` (monotonic per
  session_id), `type` (string from the locked event-type set in
  PROTOCOL.md), `payload` (JSON), `created_at`. Indexed on `(session_id,
  seq)` for the digest cursor read; also on `(session_id, created_at)`
  for time-range queries. Retained until session archival (the 90-day
  post-end window).
- `presence` — `(org_id, session_id, account_id, ref)` composite key,
  `current_sha`, `last_active_at`. Updated by `presence.updated` event
  emitters; read by `query_session_state`.

**Sequence number generator**: per-session monotonic, allocated at
emit-event time within the DB transaction (e.g., `MAX(seq)+1` under
session-scoped row lock; or a per-session counter row in a side table —
design pass picks the cheaper option for the chosen dialect).

**Emit helpers**:

- `EmitEvent(ctx, sessionID, eventType, payload)` — single-event
  emission, returns `seq`.
- `EmitBatch(ctx, sessionID, events []EventDraft)` — multi-event
  emission in one transaction with contiguous `seq` allocation. Used by
  `post-receive` when a push lands multiple commits at once.
- `UpdatePresence(ctx, sessionID, accountID, ref, sha)` — updates the
  presence row AND emits a `presence.updated` event in the same
  transaction.

**Event envelope shape** (locked at epic-design from PROTOCOL.md): every
emitted event has `{seq, version: 1, type, payload, timestamp,
session_id}`. The `version: 1` field is baked in from day one for
forward-compat.

Does NOT include the WebSocket fan-out (consumes events but isn't this
feature). Does NOT include the digest endpoint (consumes events but lives
in `sessions-rest` since it's an HTTP endpoint with session-context
assembly). Does NOT cover comments table (lives in `comments-rest`).

## Epic context

- Parent epic: `epic-portal-api`
- Position in epic: foundation feature — every other feature in this
  epic (and `epic-portal-git-post-receive` via cross-epic) consumes the
  emit helpers and the `events` table contract.

## Foundation references

- `docs/PROTOCOL.md` — WebSocket event types (canonical envelope and
  event-type catalog), Comment schema (cross-reference for payload
  shapes), Conflict event schema
- `docs/ARCHITECTURE.md` — Portal > Data store, WebSocket gateway
- `docs/SPEC.md` — Lifecycle > Retention (events deleted on archival)

## Inherited epic design decisions

- **Event envelope versioning**: `version: 1` baked in from day one;
  envelope shape locked.
- **Persistence model**: DB-persistent, per-session retention until
  archival.
- **Sequence allocation**: per-session monotonic, generated at emit time
  within the transaction.

## Generated-contracts scope

This feature contributes the canonical event-payload schemas to
`docs/openapi.yaml > components/schemas/` (per the SPEC.md generated-
contracts decision):

- `EventEnvelope` — `{seq, version, type, payload, timestamp,
  session_id}` from PROTOCOL.md
- One schema per event type listed in PROTOCOL.md's WebSocket event
  catalog: `CommitArrivedPayload`, `MergeSucceededPayload`,
  `ConflictDetectedPayload`, `ConflictResolvedPayload`,
  `CommentAddedPayload`, `CommentResolvedPayload`, `RefForkedPayload`,
  `ModeChangedPayload`, `TurnEndedPayload`, `PresenceUpdatedPayload`,
  `SessionFinalizingPayload`, `SessionEndedPayload`

The schemas are consumed by:
- Go event-emitter helpers in THIS feature (oapi-codegen-generated
  Go structs become the `payload` type for `EmitEvent`)
- The WebSocket gateway feature (typed marshal of envelopes)
- The TypeScript client (events come typed off the WebSocket as
  discriminated unions on `type`)

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
