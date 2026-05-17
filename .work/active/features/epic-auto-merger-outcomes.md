---
id: epic-auto-merger-outcomes
kind: feature
stage: drafting
tags: [portal]
parent: epic-auto-merger
depends_on: [epic-auto-merger-merge-engine, epic-portal-api-events-log, epic-portal-git-storage]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
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

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
