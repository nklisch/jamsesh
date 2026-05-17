---
id: epic-portal-api-events-log
kind: feature
stage: done
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

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Payload type in Go**: `json.RawMessage`. Emit helpers accept
  `[]byte` (or any marshalable) and store as JSON TEXT.
  Type-safe payload structs live in the openapi-generated
  `internal/api/openapi` package and are consumed by event
  PRODUCERS (post-receive, auto-merger, etc.) — each producer
  marshals its typed struct before calling EmitEvent. This avoids
  coupling the events package to the generated types.
- **Sequence allocation**: a per-session row-locked
  `UPDATE event_seq SET next = next + 1 WHERE session_id = ?
  RETURNING next - 1` pattern. The `event_seq` table is a side
  counter keyed on `session_id` — one row per session. Created
  lazily on first emit (upsert). Cheaper than `MAX(seq)+1` under
  contention, and works identically on SQLite and Postgres.
- **Schema additions** (00004 migration):
  - `events(id, org_id, session_id, seq, type, payload, created_at)`
    with PK on `id` (ULID), unique index on `(session_id, seq)`,
    index on `(session_id, created_at)`
  - `event_seq(session_id PK, next INTEGER NOT NULL DEFAULT 0)`
  - `presence(org_id, session_id, account_id, ref, current_sha,
    last_active_at)` with composite PK
- **EmitEvent contract**:
  ```go
  type EmitEvent struct {
      Type    string
      Payload json.RawMessage
  }
  func (l *Log) Emit(ctx, orgID, sessionID, type string, payload json.RawMessage) (uint64 seq, err)
  func (l *Log) EmitBatch(ctx, orgID, sessionID string, events []EmitEvent) (firstSeq uint64, err)
  func (l *Log) UpdatePresence(ctx, orgID, sessionID, accountID, ref, sha string) error
  ```
- **Tx scoping**: all emit helpers run in a single Tx that
  acquires the session counter row lock, allocates seq(s),
  inserts the event row(s), commits.
- **Story decomposition**: 2 stories, parallel-safe.
  1. `schema-queries-emit` — schema, queries, Store extension,
     Log type with emit helpers, tests. depends_on: []
  2. `openapi-event-payloads` — openapi.yaml schemas for
     EventEnvelope + 12 per-type payload schemas; regen. depends_on: []

## Architectural choice

**`internal/portal/events/` package exposing a `Log` type built
over the data-layer Store. Two responsibility lanes:**

- Schema + queries + Go emit helpers (story 1) — the persistence
  + write-side surface
- OpenAPI event-payload schemas (story 2) — the contract
  consumers compile against

The Log's writers are infrastructure-pure; type-safety of
payloads is enforced at producer call sites (which marshal their
typed structs) and at consumer call sites (which unmarshal via
generated types).

## Implementation Units

### Unit 1: Schema + migrations

**Files**:
- `db/schema/sqlite.sql` + `db/schema/postgres.sql` (edit — append `events`, `event_seq`, `presence`)
- `internal/db/migrations/sqlite/00004_events.sql` + postgres variant
**Story**: `epic-portal-api-events-log-schema-queries-emit`

Schema (SQLite; Postgres analogous with TIMESTAMPTZ):

```sql
CREATE TABLE event_seq (
    session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    next INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE events (
    id TEXT PRIMARY KEY,                                   -- ULID
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    type TEXT NOT NULL,
    payload TEXT NOT NULL,                                  -- JSON
    created_at DATETIME NOT NULL,
    UNIQUE(session_id, seq)
);
CREATE INDEX events_session_created_idx ON events(session_id, created_at);
CREATE INDEX events_org_idx ON events(org_id);

CREATE TABLE presence (
    org_id TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    ref TEXT NOT NULL,
    current_sha TEXT NOT NULL,
    last_active_at DATETIME NOT NULL,
    PRIMARY KEY (session_id, account_id, ref)
);
CREATE INDEX presence_org_idx ON presence(org_id);
```

### Unit 2: sqlc queries

**Files**: `db/queries/sqlite/events.sql`, `db/queries/postgres/events.sql`, presence variants

Queries:
- `BumpEventSeq :one` — `INSERT INTO event_seq (session_id) VALUES (?) ON CONFLICT(session_id) DO UPDATE SET next = next + 1 RETURNING next` (SQLite); Postgres equivalent with `ON CONFLICT`. Returns the next seq value AND increments atomically. Use the returned `next-1` (or have the query do the subtraction).

  Actually a cleaner pattern: maintain seq as last-allocated. Initial row has `next = 0`. The query `UPDATE event_seq SET next = next + 1 WHERE session_id = ? RETURNING next` (after first `INSERT … ON CONFLICT … DO NOTHING`) returns the new seq. On first emit the row is created with `next = 1` and that's returned.

  Use two queries: `EnsureEventSeqRow :exec` (idempotent insert) + `AllocateNextSeq :one` (UPDATE … RETURNING next).

- `InsertEvent :exec` — insert one event row
- `ListEventsSince :many` — `WHERE session_id = ? AND seq > ? ORDER BY seq ASC LIMIT ?` for digest cursor reads
- `UpsertPresence :exec` — `INSERT ... ON CONFLICT (session_id, account_id, ref) DO UPDATE SET current_sha = ..., last_active_at = ...`
- `ListPresenceForSession :many` — `WHERE session_id = ?`

### Unit 3: Store extension

**File**: `internal/db/store/store.go` (edit)

Add `EventLogStore` and `PresenceStore` sub-interfaces. Both adapters implement the new methods.

### Unit 4: Log type

**File**: `internal/portal/events/log.go`

```go
package events

import (
    "context"
    "database/sql"
    "encoding/json"
    "time"

    "github.com/oklog/ulid/v2"
    "jamsesh/internal/db/store"
)

type Log struct {
    store store.Store
    // For Tx wrapping, the Store needs Tx support — we may need to
    // extend the Store interface with WithTx (or pass DBTX directly).
    // For v0, use a simple lock-then-update pattern via the queries.
}

func New(s store.Store) *Log {
    return &Log{store: s}
}

func (l *Log) Emit(ctx context.Context, orgID, sessionID, eventType string, payload json.RawMessage) (uint64, error) {
    // 1. Ensure event_seq row exists (idempotent)
    // 2. Allocate next seq (UPDATE ... RETURNING)
    // 3. Insert event row
    // For atomicity under SQLite, BEGIN IMMEDIATE TRANSACTION is the right
    // primitive. Wrap via the Store's Tx helper (or extend Store interface to expose it).
}

type DraftEvent struct {
    Type    string
    Payload json.RawMessage
}

func (l *Log) EmitBatch(ctx context.Context, orgID, sessionID string, drafts []DraftEvent) (firstSeq uint64, err error) {
    // Same as Emit but allocates len(drafts) seq values and inserts all rows
}

func (l *Log) UpdatePresence(ctx context.Context, orgID, sessionID, accountID, ref, currentSHA string) error {
    // Upsert presence + emit "presence.updated" event in one Tx
}

func (l *Log) ListSince(ctx context.Context, sessionID string, sinceSeq uint64, limit int) ([]Event, error) {
    // Cursor read for digest endpoint
}
```

The Store's Tx capability needs careful thought. For SQLite,
`BEGIN IMMEDIATE` acquires a write lock; queries within run
atomically. For Postgres, `BEGIN` + `SELECT ... FOR UPDATE` on
the event_seq row works. The simplest path: extend Store with a
`WithTx(ctx, func(Tx) error) error` method that opens a
dialect-appropriate Tx; both adapters implement it.

### Unit 5: OpenAPI event-payload schemas (story 2)

**File**: `docs/openapi.yaml` (edit)
**Story**: `epic-portal-api-events-log-openapi-event-payloads`

Add under `components/schemas/`:

```yaml
EventEnvelope:
  type: object
  required: [seq, version, type, payload, timestamp, session_id]
  properties:
    seq: { type: integer, format: int64 }
    version: { type: integer, enum: [1] }
    type: { type: string }  # discriminator value
    timestamp: { type: string, format: date-time }
    session_id: { type: string }
    payload: { oneOf: [<the 12 payload schemas referenced by $ref>] }
  discriminator:
    propertyName: type
    mapping:
      commit.arrived: '#/components/schemas/CommitArrivedPayload'
      merge.succeeded: '#/components/schemas/MergeSucceededPayload'
      ...
```

Plus the 12 per-payload schemas. Each carries the fields PROTOCOL.md specifies for that event type.

Story-level scope: read PROTOCOL.md's event catalog, transcribe each payload's fields into a JSON Schema entry, regenerate Go types + TS types.

## Implementation Order

1. (parallel) schema-queries-emit + openapi-event-payloads
2. Both feed downstream features (websocket-gateway, sessions-rest, etc.)

## Testing

- `events/log_test.go` — single Emit returns seq=1, second returns seq=2; batch emits N events with contiguous seqs; concurrent goroutines emitting to the same session produce unique seqs (no duplicates, no gaps); UpdatePresence upsert behavior; ListSince cursor read

## Risks

- **Sequence-allocation contention**: every push lands ≥1 commit
  triggering ≥1 event. Concurrent pushes from multiple agents
  serialize on the event_seq row lock. v0 acceptable for the
  scale (few agents per session); revisit if profiling shows
  contention.
- **Tx interface design**: the Store interface needs Tx support.
  This is a non-trivial interface extension. Mitigation: add
  `WithTx(ctx, func(TxStore) error) error` where `TxStore`
  embeds the same sub-interfaces (`OrgStore`, `EventLogStore`,
  etc.); each adapter implements it by opening a Tx on its
  underlying conn and wrapping the same Queries against the Tx.
  Document this as a non-breaking additive change.

## Implementation summary

Both child stories at done. Event log persistence + typed openapi event-payload contracts landed in parallel.

### Verification
- `go build ./...` clean
- `go test ./...` green
- `make generate && git diff --exit-code` green

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none

**Notes**: Capability complete. Foundation for the API epic + auto-merger outcomes/worker chain. Downstream consumers (websocket-gateway, sessions-rest, comments-rest, mcp-endpoint, outcomes, worker) can now emit + read events via the typed contract.
