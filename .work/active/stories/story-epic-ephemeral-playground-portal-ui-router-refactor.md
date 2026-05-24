---
id: story-epic-ephemeral-playground-portal-ui-router-refactor
kind: story
stage: implementing
tags: [ui]
parent: feature-epic-ephemeral-playground-portal-ui
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Router refactor + auth-state extensions

## Scope

Story 1 of the parent feature. Substrate refactor that:
1. Restructures `router.svelte.ts` so every route declares
   `requiresAuth: boolean` (default true)
2. Replaces the hardcoded auth-gate allowlist in `App.svelte` with
   `currentRoute.requiresAuth` check
3. Adds `_playgroundContext` rune field to `auth.svelte.ts` with
   getter `auth.playgroundContext`

Full design in the parent feature body's "Story 1" section.

## Files delivered

- `frontend/src/lib/router.svelte.ts` (refactor)
- `frontend/src/App.svelte` (modify)
- `frontend/src/lib/auth.svelte.ts` (extend)
- `frontend/src/lib/auth.svelte.test.ts` (extend)
- `frontend/src/lib/router.svelte.test.ts` (extend or add)

## Acceptance criteria

See the parent feature body's "Story 1 acceptance criteria" section.

## Notes for the implementing agent

- The refactor is invisible to end-users when done correctly. Manual
  test: visit every existing route in the app, verify no spurious
  redirects to /login.
- `_playgroundContext` is intentionally separate from `_currentUser` /
  `_orgs` — playground identity is orthogonal to authenticated identity.
  The two states can coexist (rare but valid: a signed-in user clicks
  someone's playground share link).
- Tests should cover both the migrated existing routes (still public:
  /login, /auth/magic-link, /auth/oauth-callback) and the gate behavior
  for protected routes.
