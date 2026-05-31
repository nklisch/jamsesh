---
id: auth-signout-clear-playground-context
kind: story
stage: backlog
tags: [frontend, auth, cleanup]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# `auth.signOut()` should clear `playgroundContext`

## Idea
Raised in the Opus review of `story-fix-playground-joiner-401-bounce` (nit N4).
`frontend/src/lib/auth.svelte.ts` `signOut()` clears `_token`, `_refresh`,
`_currentUser`, `_orgs`, and `_loadingMe`, but NOT `_playgroundContext`. After a
playground bearer is rejected (or the session ends), `unauthorizedMiddleware`
calls `signOut()` and navigates to `/login` while a stale `playgroundContext`
lingers. Not exploitable — every API path that reads it is playground-scoped and
will keep 401ing, and the server enforces membership — but it's untidy and could
cause confusing re-entry into a dead session. Clear `_playgroundContext` in
`signOut()` (and/or when a playground session ends), and add a test mirroring
the existing signOut coverage in `auth.test.ts`.
