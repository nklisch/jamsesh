---
id: bug-scan-ws-connection-never-closed
created: 2026-05-30
tags: [bug, resource-leak]
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
