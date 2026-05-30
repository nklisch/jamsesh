---
id: epic-bug-squash-frontend-ws-lifecycle
kind: feature
stage: drafting
tags: [bug, ui]
parent: epic-bug-squash
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Frontend WebSocket connection lifecycle

## Brief

The SPA's WebSocket manager (`frontend/src/lib/ws.svelte.ts`) has three defects
in one connection-lifecycle surface: the per-session socket is never torn down
(consumers only remove a handler; `close()` is dead code in production, leaking
a live socket + reconnect machinery per visited session), `open()` overwrites an
in-reconnect record and resets the `lastSeenSeq` replay cursor (missed events +
zombie reconnect timer), and `subscribe()` floats `open()`'s rejection (a
silently dead subscription with no surfaced status).

This feature delivers a correct connection lifecycle: reference-counted teardown
so the socket closes when its last handler is removed, a reconnect-aware `open()`
that reuses an existing record without zeroing the replay cursor, and a
`subscribe()` that surfaces open failures into status instead of floating a
rejection. A single lifecycle rework resolves all three. It covers the
`ws.svelte.ts` manager only; it does NOT change the event-envelope schema or the
server-side gateway.

This feature is the **foundation for `epic-bug-squash-frontend-async-races`** —
the SessionList/component fixes there build on the corrected subscribe/close
contract, so it lands first.

## Epic context
- Parent epic: `epic-bug-squash`
- Position in epic: foundation frontend feature — `frontend-async-races` depends
  on its corrected `subscribe`/`close` contract.

## Foundation references
- `docs/SPEC.md` — WebSockets via coder/websocket, EventEnvelope spec-driven types
- Patterns: `wrapper-object-rune-store`, `openapi-fetch-middleware-client`

## Child stories (pre-existing, from bug-scan — re-parented here)
- `bug-squash-ws-connection-never-closed` — Medium, resource-leak — `frontend/src/lib/ws.svelte.ts:317`
- `bug-squash-ws-reconnect-cursor-reset` — Medium, async — `frontend/src/lib/ws.svelte.ts:248`
- `bug-squash-subscribe-floats-open-rejection` — Low, async — `frontend/src/lib/ws.svelte.ts:299`

<!-- feature-design fills in the ref-count/teardown design, the reconnect-record
reuse logic, and the vitest approach for the lifecycle. -->
