---
id: epic-portal-api-events-log-openapi-event-payloads
kind: story
stage: implementing
tags: [portal]
parent: epic-portal-api-events-log
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Event Log — OpenAPI Event-Payload Schemas

## Scope

Add the canonical event-envelope schema and 12 per-event-type
payload schemas to `docs/openapi.yaml`. After regen, consumers
(websocket gateway, frontend WS client, future MCP tools) get
typed payloads via the generated code.

## Units delivered

- `docs/openapi.yaml` (edit) — add `EventEnvelope` schema with
  discriminator + 12 payload schemas
- Regenerated `internal/api/openapi/server.gen.go` and
  `frontend/src/lib/api/types.gen.ts`

## Event types to schema-ize (per `docs/PROTOCOL.md`)

- `CommitArrivedPayload` — fields per PROTOCOL.md
- `MergeSucceededPayload`
- `ConflictDetectedPayload`
- `ConflictResolvedPayload`
- `CommentAddedPayload`
- `CommentResolvedPayload`
- `RefForkedPayload`
- `ModeChangedPayload`
- `TurnEndedPayload`
- `PresenceUpdatedPayload`
- `SessionFinalizingPayload`
- `SessionEndedPayload`

For each, transcribe PROTOCOL.md's fields verbatim into JSON
Schema. Mark required fields explicitly.

## EventEnvelope shape

```yaml
EventEnvelope:
  type: object
  required: [seq, version, type, payload, timestamp, session_id]
  properties:
    seq: { type: integer, format: int64 }
    version: { type: integer, enum: [1] }
    type: { type: string }
    timestamp: { type: string, format: date-time }
    session_id: { type: string }
    payload:
      oneOf:
        - { $ref: '#/components/schemas/CommitArrivedPayload' }
        - { $ref: '#/components/schemas/MergeSucceededPayload' }
        ...
  discriminator:
    propertyName: type
    mapping:
      commit.arrived: '#/components/schemas/CommitArrivedPayload'
      merge.succeeded: '#/components/schemas/MergeSucceededPayload'
      ...
```

## Acceptance Criteria

- [ ] `docs/openapi.yaml` validates as OpenAPI 3.0.3
- [ ] `make generate-api && git diff --exit-code` green
- [ ] `internal/api/openapi/server.gen.go` exports
      `EventEnvelope` and all 12 payload structs
- [ ] `frontend/src/lib/api/types.gen.ts` exposes
      `components['schemas']['EventEnvelope']` as a discriminated
      union over `payload` (verified by reading the generated TS)
- [ ] The discriminator mapping covers every event type listed in
      `docs/PROTOCOL.md`'s WebSocket event catalog

## Notes

- The auto-loaded `openapi-typescript` skill carries verified
  patterns for `oneOf` + `discriminator` discriminated unions on
  the TS side.
- The auto-loaded `oapi-codegen` skill carries patterns for the Go
  side; `oneOf` may map to interface types or specific structs
  depending on configuration — verify the generated shapes.
- This story's payload schemas are CONSUMED by the websocket
  gateway feature (in this same epic) and the ws.svelte.ts client
  (which currently uses a fallback string-keyed envelope). Once
  this story lands, the TS client's discriminator narrowing
  activates automatically with no code changes there.
