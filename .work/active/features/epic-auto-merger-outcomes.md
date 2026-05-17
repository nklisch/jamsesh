---
id: epic-auto-merger-outcomes
kind: feature
stage: implementing
tags: [portal]
parent: epic-auto-merger
depends_on: [epic-auto-merger-merge-engine, epic-portal-api-events-log, epic-portal-git-storage]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-17
---

# Auto-Merger — Outcome Handlers

## Brief

The side-effecting half of the merge cycle. Given a `MergeResult` from
the merge engine, this feature:

- **On clean-merge or safe-auto-resolve**: creates a merge commit on the
  draft ref with the appropriate trailers, advances `jam/<session>/draft`
  to point at it, emits `merge.succeeded` event into the event log, and
  for `Resolves-Conflict` trailers on the source commit, marks the named
  conflict event resolved.
- **On hard-conflict**: inserts a `conflict_events` row with the structured
  payload (source ref, source sha, draft tip, ancestor, conflicts array),
  computes the `addressed_to` recipients (source ref's owner + owners of
  the conflicting draft commit), emits `conflict.detected` event.

**Merge-commit identity** (locked at epic-design):

- Author = source commit's author (the human whose work is being
  integrated — preserves attribution; `git log` reads "alice: Add foo")
- Committer = synthetic `jamsesh auto-merger <auto-merger@<portal-host>>`
  identity
- Trailers:
  - `Auto-Merger: true` (always)
  - `Source-Commit: <sha>` (the commit being merged)
  - `Source-Ref: jam/<session>/<user>/<branch>` (source ref name)
  - `Auto-Resolved: <heuristic>` (only when MergeResult.Kind ==
    SafeAutoResolve; one of `whitespace` / `additions` / `identical`)
  - `Resolves-Conflict: <event-id>` (propagated from source commit when
    auto-closure fires)

**Conflict-event addressing**: per `docs/PROTOCOL.md > Conflict event
schema`, the `addressed_to` array includes the source ref's owner
(`@<user>/<branch>`) AND the owners of conflicting commits in the draft
tip's history. Walk the draft history to find the most recent commit
touching each conflicted file/range; include those commits' authors.

**`Resolves-Conflict` auto-closure**: parse the source commit's
trailers; if `Resolves-Conflict: <event-id>` is present AND merge
succeeded AND the event matches an open conflict event in this session:
mark it resolved (`resolved_at`, `resolving_commit_sha`), emit
`conflict.resolved`. Silent no-op if the event-id doesn't match an open
event (safe under replays). Log a warning if it matches a closed event
with a different resolving_commit_sha.

**Auto-resolved cases do NOT emit `conflict.detected`** (locked at
epic-design): emit `merge.succeeded` as for any successful merge. The
`Auto-Resolved: <heuristic>` trailer in `git log` is the audit trail;
`conflict.detected` is reserved for cases requiring human attention.

**Cross-epic touchpoints**:

- Reads from `epic-portal-git-storage` (bare repo handle for go-git ref
  updates)
- Emits via `epic-portal-api-events-log`
- Inserts into `conflict_events` table owned by `epic-portal-api-comments-rest`
  (the comments-rest feature exposes the table; this feature writes via the
  sqlc query package)

Does NOT include the worker that calls this feature (the `worker`
feature owns the orchestration). Does NOT include the merge logic
itself (`merge-engine`).

## Epic context

- Parent epic: `epic-auto-merger`
- Position in epic: IO side of the Ports & Adapters cut; consumes
  `merge-engine`'s pure result.

## Foundation references

- `docs/ARCHITECTURE.md` — The auto-merger > "If the merge succeeds"
  and "If the merge conflicts" sections
- `docs/PROTOCOL.md` — Commit trailer conventions (the required and
  optional auto-merger trailers), Conflict event schema (`addressed_to`
  computation), WebSocket event types (`merge.succeeded` /
  `conflict.detected` / `conflict.resolved` payloads)
- `docs/SECURITY.md` — Auto-merger authorization (server-side privileged
  write to `draft` — only this feature touches that ref)

## Inherited epic design decisions

- **Merge-commit author/committer identity**: author = source author,
  committer = synthetic auto-merger identity, trailers as listed.
- **Source-Ref trailer**: added to every auto-merger commit alongside
  Source-Commit.
- **Resolves-Conflict mismatch handling**: silent no-op for unknown
  event-ids; warn on conflict with closed events.
- **Auto-resolved cases**: emit `merge.succeeded` only, never
  `conflict.detected`.

## Design decisions

- **Schema ownership shift**: per the parent body's note that `conflict_events` is "owned by comments-rest", but comments-rest hasn't designed yet. To unblock outcomes, THIS feature ships the `conflict_events` schema (00008 migration). Comments-rest can then expose read/resolve endpoints over it.
- **`comments` table**: also needed for `Resolves-Conflict` lookup (it references a comment id potentially). Actually the trailer is `Resolves-Conflict: <event-id>` so it references conflict_events not comments. Schema for `comments` is shipped by `comments-rest` separately.
- **Package**: `internal/portal/automergeroutcomes/` (or simpler: extend `internal/portal/automerger/` with an outcomes.go). Going with the second for cohesion — the merge-engine package already exists. Add `outcomes.go` + `commit_compose.go` + `conflict_emit.go` there.
- **Public API**: `Apply(ctx, in ApplyInput) (ApplyResult, error)` where:
  ```go
  type ApplyInput struct {
      Repo            *git.Repository
      Session         *store.Session
      SourceRef       string                // e.g., "refs/heads/jam/<sess>/<user>/<branch>"
      SourceCommit    plumbing.Hash
      DraftTip        plumbing.Hash
      Ancestor        plumbing.Hash
      MergeResult     MergeResult           // from merge-engine
      AccountByEmail  func(string) (*store.Account, error)  // for trailer→account lookup
  }
  type ApplyResult struct {
      MergeCommitSHA string                 // "" on hard-conflict
      ConflictEvent  *store.ConflictEvent   // populated on hard-conflict
  }
  ```
- **Merge commit composition** (success path):
  1. Walk MergeResult.MergedTreeSHA into a `*object.Tree`
  2. Build `object.Commit{Author: sourceCommit.Author, Committer: autoMergerIdentity, Message: composeMessage(source, mergeResult), Parents: [draftTip, sourceCommit]}`
  3. Compose trailers per locked decision
  4. Write commit object via repo.Storer
  5. Update `refs/heads/jam/<sess>/draft` to the new commit SHA via `repo.Storer.SetReference` (or `repo.Storer.CheckAndSetReference` for CAS safety)
  6. Emit `merge.succeeded` event
  7. If source's trailers include `Resolves-Conflict: <event-id>`: lookup conflict_events row by id+session; if open, mark resolved + emit `conflict.resolved`
- **Conflict path**:
  1. Compute `addressed_to`: source ref's owner (extracted from SourceRef) + owners of draft commits touching each conflicted file (walk back through draft history to find last-modifier of each file)
  2. Insert conflict_events row with `{id, org_id, session_id, source_commit, draft_tip, ancestor, conflicts(JSON), addressed_to(JSON), created_at, status: "open"}`
  3. Marshal ConflictDetectedPayload (from openapi gen types)
  4. Emit `conflict.detected` event
- **Auto-merger identity**: hardcoded `Name: "jamsesh auto-merger", Email: "auto-merger@" + portalHost` (portalHost from config).
- **Story decomposition**: 1 story. The work is cohesive (merge result → side effects) and the surface is bounded.

## Implementation Units

### Unit 1: conflict_events schema

**File**: `internal/db/migrations/{sqlite,postgres}/00008_conflict_events.sql`
**Story**: `epic-auto-merger-outcomes-apply`

```sql
CREATE TABLE conflict_events (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    source_commit TEXT NOT NULL,
    draft_tip TEXT NOT NULL,
    ancestor TEXT NOT NULL,
    conflicts TEXT NOT NULL,     -- JSON
    addressed_to TEXT NOT NULL,  -- JSON
    status TEXT NOT NULL CHECK (status IN ('open','resolved')),
    resolving_commit_sha TEXT,
    created_at DATETIME NOT NULL,
    resolved_at DATETIME
);
CREATE INDEX conflict_events_session_status_idx ON conflict_events(session_id, status);
```

Plus query file + Store extension (ConflictEventStore: Insert, GetByID, MarkResolved, ListOpenForSession).

### Unit 2: Apply entrypoint

**File**: `internal/portal/automerger/outcomes.go`

Single-story scope. Composes merge commit, advances draft ref, emits events, or inserts conflict_events row.

### Unit 3: Address-computation helper

**File**: `internal/portal/automerger/addressing.go`

```go
// computeAddressedTo walks the draft history backward, finding the
// most-recent commit that touches each conflicted file. Returns the
// union of source-ref owner + those authors.
func computeAddressedTo(repo *git.Repository, draftTip plumbing.Hash, conflicts []Conflict, sourceRef string) ([]string, error)
```

For v1, scan up to 100 commits back from draftTip. Match each conflicted file path against the commit's diff vs first-parent. The first match wins. Document the bounded walk as a known v1 limitation.

## Testing

- Clean merge: verifies merge commit author = source author, committer = auto-merger, all 3+ trailers present, draft ref advanced, merge.succeeded event emitted
- Safe-auto-resolve: same as clean PLUS `Auto-Resolved: <heuristic>` trailer
- Hard-conflict: conflict_events row inserted with correct addressed_to, conflict.detected event emitted, draft ref NOT advanced
- Resolves-Conflict closure: source commit with `Resolves-Conflict: <id>` trailer marks open event resolved, emits conflict.resolved
- Resolves-Conflict mismatch: trailer with unknown id → silent no-op (no error); trailer matching closed event → logged warning

## Risks

- **Bounded history walk for addressing**: a slow-moving conflict against a 500-commit-old change won't find the original author. Mitigation: 100-commit window for v1; revisit if reports come in.
- **Auto-merger commit identity**: the synthetic email contains `portalHost`. For self-host, this is a reasonable namespacing. Document the format.
