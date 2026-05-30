---
id: bug-scan-ws-reconnect-cursor-reset
created: 2026-05-30
tags: [bug, async]
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
