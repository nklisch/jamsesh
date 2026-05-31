---
id: bug-squash-ws-reconnect-cursor-reset
kind: story
stage: review
tags: [bug, ui, async]
parent: epic-bug-squash-frontend-ws-lifecycle
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: medium
bug_domain: async
bug_location: frontend/src/lib/ws.svelte.ts:248
---

# WS open() overwrites an in-reconnect record, resetting the replay cursor and orphaning the timer

**Location**: `frontend/src/lib/ws.svelte.ts:248` · **Severity**: medium · **Pattern**: websocket reconnect race / lost messages

When `open()` runs while a record exists but `existing.ws` is currently `null` (mid-reconnect: the close handler set `ws=null` and scheduled a timer that hasn't fired), the guard `if (existing && existing.ws)` is false, so `open()` replaces the record with a fresh one whose `lastSeenSeq: 0`. This discards the accumulated replay cursor (so no `replay_from` frame → events during the gap are missed) and orphans the pending reconnect timer, which still holds the old `rec` and will fire `reopen` on a replaced record (zombie reconnect). Fix: in `open()`, reuse an existing record even when `ws === null` (let the reconnect loop own it) or guard the overwrite on `!records.has(sessionId)` without zeroing `lastSeenSeq`.

```ts
const existing = records.get(sessionId);
if (existing && existing.ws) return existing.ws;   // false when reconnecting (ws===null)
...
const rec = { ws, ..., lastSeenSeq: 0 };  // cursor reset; old reconnect timer orphaned
records.set(sessionId, rec);
```

## Implementation notes

Implemented as Unit 2 of the coordinated `ws.svelte.ts` lifecycle rework.

**Changes in `frontend/src/lib/ws.svelte.ts`:**
- `open()` first line now: `const existing = records.get(sessionId); if
  (existing) return existing.ws;` — reuses any existing record whether `ws` is
  live or `null` (mid-reconnect). A lingering record always means
  "open or reconnecting"; the reconnect loop owns it.
- Added post-ticket guard: after `await fetchTicket()`, re-check
  `records.get(sessionId)` — if a record appeared meanwhile, reuse it; if
  `handlerCount === 0`, return null and create no socket.
- `reopen()` now captures `rec` identity BEFORE `await fetchTicket()`, then
  requires `records.get(sessionId) === rec` (identity, not just existence) AND
  `handlerCount(sessionId) > 0` after the await, before attaching a new socket.
  This prevents an in-flight `reopen()` from attaching an orphan socket after a
  teardown + fresh resubscribe during the async gap.

**Regression tests in `frontend/src/lib/ws.test.ts`** (describe block
`ws — Unit 2: reconnect-aware open()`):
- Drive a session into mid-reconnect (close code 1006, `ws=null`, timer
  pending), then call `subscribe()` — asserts only 1 socket ever created and
  `lastSeenSeq` preserved (replay_from frame sent on reconnect).

Async-cancellation guard for `reopen()` (`ws — async cancellation guards`
block):
- `close()` while `reopen()` `fetchTicket` is in flight → identity check fails
  (`records.get(id) !== rec`), no second socket constructed.

All 747 tests pass; `svelte-check` clean.
