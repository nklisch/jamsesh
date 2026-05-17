---
id: epic-portal-git-post-receive
kind: feature
stage: implementing
tags: [portal]
parent: epic-portal-git
depends_on: [epic-portal-git-storage]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Git — Post-Receive Event Emission

## Brief

After the smart-HTTP receive-pack handler has accepted a push (pre-receive
validated, `git-receive-pack` updated the ref), this feature emits the
`commit.arrived` events into the portal event log that the WebSocket gateway
(in `epic-portal-api`) and the auto-merger (in `epic-auto-merger`) consume.

**Inputs** (handed to this feature from the smart-http handler after
receive-pack returns success):

- `session_id`
- For each accepted ref update: `ref_name`, `old_sha`, `new_sha`
- The bare repo handle (from storage)

**Work performed**:

- Resolve the commit range for each ref update (commits in `old_sha..new_sha`)
  using `go-git`
- For each commit: collect `sha`, `author_id` (from trailers), `summary`
  (first line of commit message), `ref`
- Emit a `commit.arrived` event per commit into the event log, with the
  next monotonic per-session `seq` number
- Emit at most one batched event per ref update for ref-meta changes
  (the design pass decides batching strategy)

**Cross-epic touchpoint**: the event log table (`events`) is owned by
`epic-portal-api`. This feature writes to it via the data-layer sqlc query
package. The design pass on this feature is sequenced after portal-api's
events-table feature exists in the substrate (declared as a feature dep then;
intra-epic deps captured here for autopilot readiness).

**Failure handling**: post-receive runs after the ref update has already
committed. Failure to emit an event is logged loudly but does not roll back
the ref update — the source of truth is git. A reconciliation sweep (deferred
out of v1, or shipped as part of the data-layer feature's design) can replay
missed events by diffing the event log against actual git refs.

Does NOT cover the WebSocket fan-out (`epic-portal-api`'s WebSocket gateway
feature subscribes to the event log). Does NOT cover auto-merger triggering
(`epic-auto-merger` subscribes independently).

## Epic context

- Parent epic: `epic-portal-git`
- Position in epic: parallel with pre-receive after storage lands;
  smart-http is the assembly point that calls both.

## Foundation references

- `docs/ARCHITECTURE.md` — Data flow: a turn > post-receive processes the
  push; auto-merger subsection
- `docs/PROTOCOL.md` — WebSocket event types (`commit.arrived` payload
  shape)
- `docs/SPEC.md` — Multi-tenant by design (events carry `org_id` via the
  session lookup)

## Decomposition risks

- The `events` table lives in `epic-portal-api`; this feature writes into
  it without owning the schema. If portal-api's design pass changes the
  event schema, this feature's implementation may need to adapt. Mitigation:
  the design pass on this feature waits until portal-api's events-table
  feature is decomposed (or, if running ahead, locks the canonical
  envelope from `docs/PROTOCOL.md > WebSocket event types` which already
  pins the shape).
- Post-receive after-the-fact failure handling is intentionally weak. A
  missed event means a peer's UI lags one turn — acceptable; git remains
  the source of truth.

## Design decisions

- **Events log is at done**: this feature consumes the `Log.EmitBatch` API directly. No need to re-design event-log primitives.
- **Single entry point**: `Emitter` type with `EmitForUpdates(ctx, repo, session, account, updates) error`. Internally: for each update, walk commits via go-git, collect minimal metadata, marshal `CommitArrivedPayload` (from generated openapi types), batch-emit via `Log.EmitBatch`.
- **Trailer-derived author**: from `Jam-Author` trailer if present; else commit author. Falls back to author.Email if Jam-Author absent.
- **Failure semantics**: errors logged at slog Error level but propagated to caller. Caller (smart-http handler) logs but doesn't fail the HTTP response — the push already succeeded.
- **Single story**: small surface (one Emitter type + tests).

## Implementation Units

### Unit 1: Emitter

**File**: `internal/portal/postreceive/emitter.go`
**Story**: `epic-portal-git-post-receive-emitter`

```go
package postreceive

import (
    "context"
    "encoding/json"

    "github.com/go-git/go-git/v5"
    "jamsesh/internal/db/store"
    "jamsesh/internal/portal/events"
)

type Emitter struct {
    Log *events.Log
}

type RefUpdate struct {
    Ref    string
    OldSHA string
    NewSHA string
}

// EmitForUpdates emits a batch of commit.arrived events for every new
// commit in every accepted update. Returns nil on success.
func (e *Emitter) EmitForUpdates(ctx context.Context, repo *git.Repository, session *store.Session, account *store.Account, updates []RefUpdate) error
```

Implementation:
1. For each update, resolve OldSHA..NewSHA commit range
2. For each commit, build a `CommitArrivedPayload` struct (from generated openapi types):
   - `sha`: commit hash
   - `ref`: update.Ref
   - `summary`: first line of commit message (trimmed at first `\n`)
   - `author_id`: trailer `Jam-Author` value, or commit.Author.Email if absent
3. JSON-marshal each payload as `json.RawMessage`
4. Collect into `[]events.DraftEvent` with `Type: "commit.arrived"`
5. Call `Log.EmitBatch(ctx, account.OrgID...)`

Wait — `session.OrgID` is the right org. Not account.

Final signature: `EmitForUpdates(ctx, repo *git.Repository, session *store.Session, _ *store.Account, updates []RefUpdate) error`. Use `session.OrgID`.

## Implementation Order

Single story.

## Testing

- Synthetic repo with 3 commits on a branch; emit + verify 3 events written with correct seqs, types, and payload contents
- Empty range (OldSHA == NewSHA): zero events emitted
- New ref creation (OldSHA == ""): emits events for all reachable new commits (subject to a sane cap — e.g., 1000 commits — to prevent runaway first-push)
- Missing Jam-Author trailer falls back to commit.Author.Email

## Risks

- **First-push runaway**: a new ref with no OldSHA could in theory have thousands of commits in its ancestry. Mitigation: cap at 1000 commits per emit batch; document as a known limitation.
- **Event ordering**: parallel pushes to the same session could interleave events. The event log's seq allocation is atomic per session, so the seq numbers stay monotonic — but the order of commits across pushes isn't guaranteed. Acceptable; consumers can resequence by timestamp if needed.
