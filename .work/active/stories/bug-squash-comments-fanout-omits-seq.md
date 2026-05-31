---
id: bug-squash-comments-fanout-omits-seq
kind: story
stage: review
tags: [bug, portal, error-handling]
parent: epic-bug-squash-data-tx-integrity
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: medium
bug_domain: error-handling
bug_location: internal/portal/comments/service.go:254
---

# Comments WebSocket fan-out omits the allocated Seq, breaking client replay-cursor dedup

**Location**: `internal/portal/comments/service.go:254` (also `:336`) · **Severity**: medium · **Pattern**: partial/inconsistent value passed downstream

The DB event row gets the correctly allocated `seq` inside the tx, but the post-commit `FanOut` builds an `events.Event` with `Seq` defaulted to 0. `writeEnvelope` copies `e.Seq` into the wire envelope, and the SPA client only advances its `lastSeenSeq` replay cursor when `env.seq > lastSeenSeq`. A seq=0 `comment.added`/`comment.resolved` never advances the cursor, so on reconnect the client requests `replay_from: lastSeenSeq` and the portal re-delivers the same comment events (which DID get a positive seq in the DB) — duplicate comments on every reconnect, defeating seq dedup. The `events/log.go` path sets `Seq` correctly; comments are the outlier. Fix: capture the allocated `seq` (and event id) from inside the tx closure and populate them on the `FanOut` event, or route comment events through `events.Log.Emit`.

```go
seq, err := tx.AllocateNextSeq(ctx, p.SessionID)   // real seq stored on DB row
...
s.Log.FanOut(events.Event{ OrgID: ..., Type: "comment.added", Payload: ..., CreatedAt: now })  // Seq unset -> 0
```

## Implementation notes

Added outer `allocatedSeq int64` and `allocatedEventID string` vars in both `Create` and `Resolve`. Inside the tx closure, assign `allocatedSeq = seq` and `allocatedEventID = eventID` after `InsertEvent` succeeds. Post-commit `FanOut` now passes `ID: allocatedEventID, Seq: allocatedSeq` (mirrors events/log.go's own insertAndPublish pattern). Tests: `TestCommentsFanoutCarriesSeqOnCreate` and `TestCommentsFanoutCarriesSeqOnResolve` in `internal/portal/comments/fanout_seq_test.go` — subscribe before Create/Resolve, assert fanned-out event has Seq > 0 and non-empty ID.
