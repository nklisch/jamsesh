---
id: story-refactor-events-log-emit-batch-shared-helper
kind: story
stage: implementing
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
