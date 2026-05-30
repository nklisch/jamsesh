---
id: bug-squash-subscribe-floats-open-rejection
kind: story
stage: implementing
tags: [bug, ui, async]
parent: epic-bug-squash-frontend-ws-lifecycle
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: low
bug_domain: async
bug_location: frontend/src/lib/ws.svelte.ts:299
---

# subscribe() floats open()'s rejection — handler registered against a socket that never opens

**Location**: `frontend/src/lib/ws.svelte.ts:299` · **Severity**: low · **Pattern**: fire-and-forget / unhandled rejection

`subscribe()` calls `void open(sessionId)`; `open()` is async and `throw`s when `!auth.token` (and resolves `null` on ticket-fetch failure). The `void` discards the rejected promise (unhandled rejection, no surfaced error), yet the handler is still added to the map, so the subscriber silently receives nothing — no events, no error, no status. Anonymous/playground views that subscribe before a token is present hit this. Fix: have `open()` resolve to `null` instead of throwing (consistent with its other failure path), and reflect the failure into `wsStatus`/a status callback so the UI can show disconnected.

```ts
export function subscribe(sessionId, type, handler) {
  void open(sessionId);   // open() throws (rejects) if !auth.token; rejection discarded
  ...register handler...
}
```
