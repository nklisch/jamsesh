---
id: story-refactor-events-log-emit-batch-shared-helper
kind: story
stage: review
tags: [portal, refactor]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-23
---

# Extract shared insert-and-publish helper for events.Log Emit / EmitBatch

## Brief

`internal/portal/events/log.go` defines `Emit` (~7 lines) and
`EmitBatch` (~60 lines). Both:

1. Acquire / verify the event-seq row.
2. Allocate the next sequence number(s).
3. Insert the event row(s).
4. Fan out via the WebSocket gateway after commit.

The single-event path is `Emit`; `EmitBatch` reimplements the core
sequence with a loop. The shared shape lives in two places, and any
change to event-emit semantics (e.g. fan-out timing, retries) has to
be made in both.

Surfaced by a discovery-mode `/agile-workflow:refactor-design` scan.

## Target state

A package-private helper `insertAndPublish(ctx, tx, events []Event) error`
that handles seq allocation, row insertion, and post-commit fan-out for
a slice of events. `Emit` becomes a one-liner that wraps a single event
into a slice. `EmitBatch` stays the public batch entry but loses the
duplicated core loop.

Preserves the `tx-emit-then-fanout` pattern documented in
`.claude/skills/patterns/tx-emit-then-fanout.md` — fanout still
strictly outside the transaction.

## Acceptance criteria

- `events.Emit` and `events.EmitBatch` share the same insert + fanout
  helper.
- The `tx-emit-then-fanout` discipline is preserved — fan-out happens
  after `WithTx` returns, never inside.
- All event-log tests pass without modification.
- `go test ./internal/portal/events/...` clean.

## Notes

Behavior-preserving — same event payload shape, same fan-out timing,
same error-return shape on either entry point.

## Implementation notes

**Approach:** Extracted `(l *Log) insertAndPublish(ctx, orgID, sessionID, drafts []DraftEvent) (firstSeq int64, ids []string, emittedAt time.Time, err error)` as the shared core. The helper owns the full tx-emit-then-fanout lifecycle: opens the transaction, allocates seqs, inserts rows, commits, then fans out to subscribers. Fan-out is explicitly outside the `WithTx` closure, making the discipline visible by construction.

**AllocateNextSeq vs AllocateNextSeqN:** The helper uses `AllocateNextSeqN` for both the single and batch cases. SQL confirms both methods are identical for n=1 (`UPDATE event_seq SET next = next + ? RETURNING next`). Using `AllocateNextSeqN(ctx, sessionID, 1)` is semantically equivalent to `AllocateNextSeq` — no branch needed.

**UpdatePresence decision:** Left untouched. `UpdatePresence` has a `UpsertPresence` step inside the tx that is not shared with `Emit`/`EmitBatch`, and folding it through `insertAndPublish` would require passing a pre-tx callback or splitting the helper into two halves — neither improves readability. Decision documented; `UpdatePresence` continues its current implementation.

**Metrics:** Both `Emit` (`.Inc()`) and `EmitBatch` (`.Add(float64(n))`) still increment `EventLogEmitTotal` in their own bodies after calling `insertAndPublish`, preserving the existing per-caller count behavior.

**Verification:** `go build ./...`, `go test ./internal/portal/events/...`, and `go test ./...` all pass clean. No tests modified.

**LoC delta:** Removed ~50 lines of duplicated logic from `Emit` + `EmitBatch`; added ~65 lines for `insertAndPublish` + slimmed callers. Net: `log.go` went from 353 to ~370 lines (helper is slightly longer than either individual method it replaces, but the callers are each 5–7 lines vs 40–50 lines).
