---
id: epic-portal-api-events-log-openapi-event-payloads
kind: story
stage: done
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

## Implementation notes

### Field-name deviations from story task description vs PROTOCOL.md

PROTOCOL.md is canonical per the task. The following deviations exist
between the task description's suggested field names and the PROTOCOL.md
canonical names (PROTOCOL.md names were used in all schemas):

| Event type        | Task description field | PROTOCOL.md canonical field |
|-------------------|------------------------|------------------------------|
| commit.arrived    | commit_sha             | sha                          |
| commit.arrived    | author_display         | (omitted, not in PROTOCOL)   |
| commit.arrived    | message                | summary                      |
| commit.arrived    | trailers               | (omitted, not in PROTOCOL)   |
| commit.arrived    | created_at             | (omitted, not in PROTOCOL)   |
| merge.succeeded   | merge_commit_sha       | merge_commit_sha (same)      |
| merge.succeeded   | source_commit          | source_sha                   |
| merge.succeeded   | draft_tip_before       | (omitted, not in PROTOCOL)   |
| merge.succeeded   | draft_tip_after        | draft_sha                    |
| merge.succeeded   | heuristic              | (omitted, not in PROTOCOL)   |
| conflict.detected | (event_id at top)      | id                           |
| conflict.detected | files                  | conflicts                    |
| comment.added     | comment_id             | id                           |
| comment.added     | kind                   | kind (same)                  |
| comment.added     | addressing             | addressed_to                 |
| comment.added     | anchor.file            | anchor.file_path             |
| comment.added     | anchor.range           | anchor.line_range            |
| comment.added     | body_excerpt           | body (full body in PROTOCOL) |
| ref.forked        | parent_commit          | parent_sha                   |
| ref.forked        | owner_id               | (omitted; ref ownership is   |
|                   |                        |  implicit in ref path)       |
| mode.changed      | from_mode              | old_mode                     |
| mode.changed      | to_mode                | new_mode                     |
| mode.changed      | changed_by             | (omitted, not in PROTOCOL)   |
| turn.ended        | turn_id                | (omitted, not in PROTOCOL)   |
| turn.ended        | author_id              | user_id                      |
| turn.ended        | commit_count           | (omitted, not in PROTOCOL)   |
| turn.ended        | ended_at               | (omitted, not in PROTOCOL)   |
| presence.updated  | account_id             | user_id                      |
| presence.updated  | last_active_at         | last_active                  |
| session.finalizing| initiator_id           | by_user_id                   |
| session.finalizing| started_at             | (omitted, not in PROTOCOL)   |
| session.ended     | end_reason             | reason                       |
| session.ended     | final_branch           | (omitted, not in PROTOCOL)   |
| session.ended     | ended_at               | (omitted, not in PROTOCOL)   |

Fields present in the task description but absent from PROTOCOL.md
were not added — PROTOCOL.md is the source of truth, and producers
should extend PROTOCOL.md before adding fields.

### Generated Go shape (oapi-codegen v2.7.0)

`EventEnvelope_Payload` is a struct wrapping `json.RawMessage` (not
an interface). The generated API for consumers is:

```go
// Read a payload:
switch env.Type {
case openapi.CommitArrived:
    p, err := env.Payload.AsCommitArrivedPayload()
case openapi.MergeSucceeded:
    p, err := env.Payload.AsMergeSucceededPayload()
// ... etc for all 12 types
}

// Write a payload (at emit time):
var payload openapi.EventEnvelope_Payload
if err := payload.FromCommitArrivedPayload(p); err != nil { ... }
```

The `MergeXxxPayload` variants perform JSON merge of the union; use
`FromXxxPayload` to set or overwrite.

### Generated TypeScript shape (openapi-typescript 7.13.0)

`components["schemas"]["EventEnvelope"]["payload"]` is typed as a
union of all 12 payload schemas. The TypeScript consumer discriminates
via the sibling `type` field on the envelope object:

```typescript
if (env.type === "commit.arrived") {
    const p = env.payload; // typed as CommitArrivedPayload
}
```

Standard TypeScript discriminated-union narrowing works via the `type`
string literal union on `EventEnvelope`.

### New indirect dependency

`github.com/apapsch/go-jsonmerge/v2 v2.0.0` added to go.mod — pulled
in by `github.com/oapi-codegen/runtime` v1.4.0 for the MergeXxx methods
in the generated union type.

### Build status

`go build ./...` fails on pre-existing errors in the sibling story
(`schema-queries-emit`) for `AllocateNextSeq` missing from store
adapters — unrelated to this story's openapi-only changes. The openapi
package itself compiles cleanly. Full build will pass once both stories
in this wave are merged.

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: 12 payload schemas + EventEnvelope + 3 shared sub-schemas (ConflictFileRange, ConflictFile, CommentAnchor). Deviation table from task-description fields to PROTOCOL.md canonical fields is well-documented; PROTOCOL.md remained authoritative throughout. The generated EventEnvelope_Payload Go shape (json.RawMessage with As/From/Merge accessors) is the standard oapi-codegen oneOf pattern.
