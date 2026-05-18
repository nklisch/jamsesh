# Pattern: Transactional Emit-Then-FanOut

A service-layer method that mutates state opens `store.WithTx`, performs
the row insert/update, calls `tx.EnsureEventSeqRow(sessionID)` +
`tx.AllocateNextSeq(sessionID)` to get a per-session monotonic seq,
inserts the event row inside the same transaction, then — _after
commit_ — calls `events.Log.FanOut(...)` to broadcast to WebSocket
subscribers. Persistence and ordering are atomic; fanout is best-effort,
non-blocking.

## Rationale

The per-session `event_seq` counter is the source of truth for
WebSocket replay; if the seq is allocated outside the tx, two
concurrent writes can race and produce gaps or duplicates. Fanout
outside the tx avoids holding the DB lock across slow socket writes,
and a dropped subscriber doesn't block commit. The shape is identical
whether the emitting code owns its own event payload (comments) or
delegates to `events.Log.Emit` (presence updates).

## Examples

### Example 1: comments.Service.Create — domain row + event row in one tx, fanout after

**File**: `internal/portal/comments/service.go:123-222`

```go
err := s.Store.WithTx(ctx, func(tx store.TxStore) error {
    if err := tx.InsertComment(ctx, ...); err != nil { return ... }
    payload, _ := json.Marshal(commentAddedPayload{...})
    if err := tx.EnsureEventSeqRow(ctx, p.SessionID); err != nil { return ... }
    seq, err := tx.AllocateNextSeq(ctx, p.SessionID)
    if err != nil { return ... }
    return tx.InsertEvent(ctx, store.InsertEventParams{ID: ..., Seq: seq, Type: "comment.added", ...})
})
if err != nil { return store.Comment{}, err }
// After commit only:
s.Log.FanOut(events.Event{Type: "comment.added", Payload: payloadBytes, ...})
```

### Example 2: events.Log.Emit — same shape, generic over event type

**File**: `internal/portal/events/log.go:154-195`

```go
err := l.s.WithTx(ctx, func(tx store.TxStore) error {
    if err := tx.EnsureEventSeqRow(ctx, sessionID); err != nil { return ... }
    allocated, err := tx.AllocateNextSeq(ctx, sessionID)
    if err != nil { return ... }
    seq = allocated
    return tx.InsertEvent(ctx, store.InsertEventParams{Seq: seq, Type: eventType, ...})
})
if err != nil { return 0, err }
l.fanOut(Event{Seq: seq, Type: eventType, ...})
```

### Example 3: events.Log.UpdatePresence — presence upsert + event in one tx

**File**: `internal/portal/events/log.go:282-310` follows the same
shape: `UpsertPresence` then `EnsureEventSeqRow` then `AllocateNextSeq`
then `InsertEvent`, all inside `WithTx`; fanout afterward.

Also: `events.Log.EmitBatch` at `log.go:213` and
`comments.Service.Resolve` at `service.go:244`.

## When to Use

- Any service-layer mutation that must produce a corresponding event
  row visible to subscribers.
- The event must carry the correct per-session `seq` for ordered
  replay.

## When NOT to Use

- Pure-read operations.
- Background or replay operations that emit events without mutating
  other domain state — those can use `events.Log.Emit` directly
  without composing into a larger tx.
- Out-of-band system events that should not be persisted.

## Common Violations

- Calling `s.Log.FanOut` _inside_ the `WithTx` closure — broadcasts a
  seq that hasn't committed yet; if the tx rolls back, subscribers see
  a phantom event.
- Skipping `EnsureEventSeqRow` — first-emit-per-session will fail
  because the counter row doesn't exist yet (the helper is idempotent
  and safe to call every time).
- Allocating the seq _outside_ the tx and passing it in — race window
  across concurrent writers.
