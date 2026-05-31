---
id: bug-squash-subscribe-floats-open-rejection
kind: story
stage: done
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

## Implementation notes

Implemented as Unit 3 of the coordinated `ws.svelte.ts` lifecycle rework.

**Changes in `frontend/src/lib/ws.svelte.ts`:**
- Replaced `throw new Error('ws: cannot open socket — no auth token')` with
  `setStatus(sessionId, null); return null;` in `open()`. This is consistent
  with the existing ticket-failure path (also returns null). `void open()` in
  `subscribe()` is now safe — no rejection can escape.
- `WsStatus` union intentionally NOT widened (no 'failed' member) — null
  (disconnected) is the honest state for no-token. The Low severity doesn't
  justify touching all consumers.
- Documented limitation: a handler registered before a token exists stays in
  the map but does NOT auto-open when a token later appears; a new `subscribe()`
  or `open()` call is required.

**Regression tests in `frontend/src/lib/ws.test.ts`** (describe block
`ws — Unit 3: open() null on no-token`):
- `open()` with falsy `auth.token` → 0 `WebSocket` instances, `wsStatus`
  returns null; no unhandled rejection surfaces.
- Documented limitation test: token set after a no-token subscribe → still 0
  sockets (no auto-open).

All 747 tests pass; `svelte-check` clean.
