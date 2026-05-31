---
id: bug-squash-ws-connection-never-closed
kind: story
stage: review
tags: [bug, ui, resource-leak]
parent: epic-bug-squash-frontend-ws-lifecycle
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: medium
bug_domain: resource-leak
bug_location: frontend/src/lib/ws.svelte.ts:317
---

# Per-session WebSocket is never torn down — leaks a live socket per visited session

**Location**: `frontend/src/lib/ws.svelte.ts:317` (`close`, never called in production) · **Severity**: medium · **Pattern**: WebSocket not closed on unmount

`subscribe()` opens a socket and returns a closure that only removes one handler — it never closes the underlying connection. `close()` (the only path that tears down the socket, its `records` entry, and the reconnect timer) is invoked nowhere in production (only in `ws.test.ts`). Every consumer dutifully unsubscribes its handler, but the WebSocket stays open in `records`. Navigating between sessions in the SPA accumulates live server-side WS connections (each holding a gateway per-conn `send` goroutine) plus orphaned client sockets and reconnect loops, unboundedly over a long session. Fix: reference-count handlers per session and call `close(sessionId)` when the last handler is removed, or have screens call `close(sessionId)` in `onDestroy`.

```ts
return () => { byType.get(type)?.delete(handler); };  // removes handler only — never closes ws
// close() exists but is called only from ws.test.ts
```

## Implementation notes

Implemented as Unit 1 of the coordinated `ws.svelte.ts` lifecycle rework (all
three stories in one file, one worktree, sequential commits).

**Changes in `frontend/src/lib/ws.svelte.ts`:**
- Added `teardownTimer: ReturnType<typeof setTimeout> | null` field to
  `ConnectionRecord`.
- Added `handlerCount(sessionId)` helper — sums Set sizes across all event
  types for a session.
- Added `teardown(sessionId)` — the connection-closing body formerly inline in
  `close()`: sets `closedByUs`, clears `teardownTimer` + `reconnectTimer`,
  calls `ws?.close()`, deletes the record and handler map entry, calls
  `setStatus(null)`. This intentionally drops `lastSeenSeq` (cursor
  invalidation — "left and came back").
- Added `scheduleTeardown(sessionId)` — uses `rec.teardownTimer ??=
  setTimeout(..., 0)` to enqueue a macrotask that calls `teardown()` only if
  `handlerCount` is still 0 when it fires. If no record exists, cleans up
  handler map and status immediately.
- Modified `subscribe()`: on entry, cancels any pending `teardownTimer` (new
  subscriber arrives before the linger fires). The unsubscribe closure now
  prunes the empty per-type `Set` after deletion, then calls
  `scheduleTeardown()` if `handlerCount === 0`.
- Modified `close()`: cancels any pending `teardownTimer`, then delegates to
  `teardown()`.

**Linger rationale:** `setTimeout(0)` is a macrotask; Svelte 5 `$effect`
teardown runs immediately before the synchronous rerun, so the linger timer
cannot fire between a cleanup and its re-subscribe. Verified against the design
doc (codex confirm). The linger absorbs exactly the synchronous re-run window.

**Regression tests in `frontend/src/lib/ws.test.ts`** (describe block
`ws — Unit 1: ref-counted teardown`):
- Unsubscribe last handler → `readyState === 3` (CLOSED) after macrotask tick,
  `wsStatus.for()` returns null.
- Unsubscribe one of several → socket still open after macrotask tick.
- Synchronous unsubscribe-all → resubscribe → linger cancelled, socket not
  closed after macrotask tick.

Async-cancellation guard for Unit 1 (`ws — async cancellation guards` block):
- Unsubscribe while `fetchTicket` is in-flight → `open()` post-ticket guard
  sees `handlerCount === 0` and creates no `WebSocket`.

All 747 tests pass; `svelte-check` clean (0 errors, 1 pre-existing warning in
an unrelated file).
