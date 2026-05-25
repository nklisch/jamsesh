---
id: story-epic-ephemeral-playground-portal-ui-router-refactor
kind: story
stage: done
tags: [ui]
parent: feature-epic-ephemeral-playground-portal-ui
depends_on: []
release_binding: v0.4.0
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

## Implementation notes

### What changed

**`frontend/src/lib/router.svelte.ts`**
- Added `requiresAuth: boolean` field to the `Route` type.
- All existing routes annotated: `/login`, `/auth/magic-link`,
  `/auth/oauth/callback` get `requiresAuth: false`; all other routes
  get `requiresAuth: true` (home, sessions, session-view, finalize,
  invite-accept, org-settings).
- `match()` return type extended to include `requiresAuth`; the
  not-found fallback defaults to `true` (unknown surfaces protected by
  default).
- `current` exported object gains a `get requiresAuth()` accessor.

**`frontend/src/App.svelte`**
- The hardcoded name-based allowlist
  (`current.name !== 'login' && current.name !== 'magic-link' && ...`)
  replaced with a single `current.requiresAuth` flag check.
- All existing behaviour preserved: authed-on-login bounce to `/`,
  invite-accept `?return_to` preservation, and the bootstrap
  `loadCurrentUser` effect are unchanged.

**`frontend/src/lib/auth.svelte.ts`**
- New `PlaygroundContext` type exported:
  `{ sessionId: string; bearer: string; nickname: string }`.
- New private `$state` variable `_playgroundContext` (initially null),
  intentionally separate from `_currentUser`/`_orgs`.
- New `get playgroundContext()` getter on the `auth` object.
- New `setPlaygroundContext(ctx: PlaygroundContext | null)` method.

**`frontend/src/App.test.ts`**
- `mockRouterCurrent` extended with `requiresAuth: boolean`.
- Each test case sets `requiresAuth` to match the route it simulates.

**`frontend/src/lib/router.test.ts`**
- New `describe('router — requiresAuth flag')` suite covering public
  routes (`false`), protected routes (`true`), and not-found fallback.

**`frontend/src/lib/auth.test.ts`**
- New `playgroundContext` tests: starts null, set/clear, orthogonality
  with `isAuthenticated`, coexistence when both are set.

### Deviations from design

None.

### Verification

- `npm run check`: 0 errors, 2 pre-existing warnings (unrelated)
- `npm run test`: 540 tests pass
- `npm run build`: clean bundle

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Implementation matches design 1:1. Router gains `requiresAuth: boolean` with `true` default; App.svelte's gate now reads `current.requiresAuth`; `_playgroundContext` rune is correctly orthogonal to `_currentUser`/`_orgs`. The wrapper-object-rune-store project pattern is followed for both the router and the new auth field. Targeted re-run of `vitest src/lib/router.test.ts src/lib/auth.test.ts src/App.test.ts`: 47/47 pass. Downstream consumers (`JoinerPicker.svelte`, `SessionViewShell.svelte`) already integrate `auth.setPlaygroundContext` / `auth.playgroundContext` as designed. Bounce-on-authed-login and invite-accept `?return_to` preservation behaviors intact.
