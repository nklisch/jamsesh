---
id: story-playground-anon-access-ws-live-updates
kind: story
stage: drafting
tags: [playground, ui, auth, websocket, bug]
parent: feature-playground-anon-session-access
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-06-01
---

# Anonymous playground participants get no live WebSocket updates

## Idea
Surfaced while fixing the playground joiner 401 bounce
(`story-fix-playground-joiner-401-bounce`, GitHub #1). The bounce fix lets an
anonymous joiner reach and stay on the session view, but the WebSocket live
feed still does not connect for them, so tree/activity/comment events don't
stream in real time. Two causes in `frontend/src/lib/ws.svelte.ts`: `open()`
and `reopen()` both guard on `if (!auth.token)` and bail when there's no account
token, and the `POST /api/auth/ws-ticket` request is not playground-scoped, so
`bearerMiddleware` won't attach the anonymous `playgroundContext` bearer to it —
the ticket fetch fails / is issued for the wrong identity and the
`/ws/sessions/<id>` upgrade 403s. Fix needs the WS layer to use the playground
bearer (gate on `auth.token || auth.playgroundContext`, and make the ws-ticket
request carry the playground bearer for the active playground session). Larger
than the bounce fix; deferred deliberately to keep that fix minimal.
