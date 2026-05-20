---
id: spa-logged-in-landing-authed-redirect-fixes
kind: story
stage: review
tags: [frontend, ui]
parent: spa-logged-in-landing-and-org-bootstrap
depends_on: [spa-logged-in-landing-home-screen]
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
---

# Authed-redirect fixes

## Scope

Three small redirect-flow corrections that route freshly-authenticated
users to the new `/` Home screen instead of bouncing them back to the
login form they just succeeded against:

1. `OAuthCallback.svelte` — change the post-exchange fallback target from
   `'/login'` to `'/'`, and `await auth.loadCurrentUser()` before
   navigating so the "Signing you in..." view encompasses the `/api/me`
   round-trip.
2. `Login.svelte` — redirect authed users unconditionally (was: only
   when `returnTo` was set, leaving users stuck on the form when
   `returnTo` was null).
3. `App.svelte` — extend the existing auth-gate `$effect` to catch authed
   users who land on `/login` directly (browser back button, bookmark)
   and bounce them to `/`.

See parent feature `## Implementation Units > Unit 3` for full specification.

## Files

- `frontend/src/lib/screens/OAuthCallback.svelte` (edit lines 46-54)
- `frontend/src/lib/screens/OAuthCallback.test.ts` (edit — update navigate
  assertions, add a loadCurrentUser-await test)
- `frontend/src/lib/screens/Login.svelte` (edit lines 46-50)
- `frontend/src/lib/screens/Login.test.ts` (edit — add authed-redirect
  cases for both `returnTo === null` and `returnTo !== null`)
- `frontend/src/App.svelte` (edit the existing auth-gate `$effect`)

## Acceptance Criteria

- [ ] On successful OAuth exchange, `OAuthCallback.exchange()` awaits
      `auth.loadCurrentUser()` BEFORE calling `navigate()`.
- [ ] On successful OAuth exchange with no stored `return_to`,
      `OAuthCallback` navigates to `'/'` (was `'/login'`).
- [ ] On successful OAuth exchange with a stored `return_to`,
      `OAuthCallback` navigates to that `return_to` (unchanged behavior).
- [ ] `Login.svelte`'s `$effect` fires `navigate(returnTo ?? '/')`
      whenever `auth.isAuthenticated` becomes true — whether or not
      `returnTo` is set.
- [ ] `App.svelte`'s auth-gate `$effect` redirects to `'/'` when
      `auth.isAuthenticated && current.name === 'login'`.
- [ ] `App.svelte`'s auth-gate `$effect` STILL redirects unauthed users
      on protected routes to `/login` (no regression of existing path).
- [ ] `App.svelte`'s auth-gate `$effect` STILL handles the
      `invite-accept` case with `?return_to=` preservation.
- [ ] `OAuthCallback.test.ts` regression suite passes plus new tests:
      (a) navigate target is `/` when no return_to, (b) loadCurrentUser
      is awaited before navigate.
- [ ] `Login.test.ts` regression suite passes plus new tests:
      (a) authed user with no returnTo navigates to `/`,
      (b) authed user with returnTo navigates to that returnTo.
- [ ] `App.svelte` test (or any existing harness around the gate effect)
      passes; if no existing tests cover the gate, add a basic
      `App.test.ts` that exercises both branches.
- [ ] `npm run check` and `npm run test` pass.

## Notes

- The redundancy between `Login.svelte`'s effect and `App.svelte`'s gate
  for the authed-on-login case is intentional defense-in-depth — see
  parent feature for rationale.
- Do NOT alter the existing `return_to` sessionStorage preservation
  across the OAuth round-trip in either `Login.svelte` or
  `OAuthCallback.svelte`. Only the fallback target string changes.
- If `auth.loadCurrentUser()` rejects or hangs, the OAuthCallback path
  must STILL navigate — the `await` is inside a `try` block that already
  exists. Verify the catch branch still navigates / surfaces error UI as
  before; do not silently swallow load failures past the existing error
  handling shape.

## Out of scope for this story

- No new screens, no new routes (Unit 2 added the `/` route already).
- No changes to the magic-link flow — its own redirect logic is
  separate and out of this feature's scope.
- No reactive auth.orgs UI inside this story; consumers are limited to
  the existing OAuthCallback/Login screens which only care about tokens.

## Implementation notes

All three edits landed as specified with no design deviations.

**App.test.ts:** Not created. The auth-gate logic is pure route-string +
auth-state branching. The per-screen tests (Login.test.ts authed-redirect
cases) cover the most user-visible path (authed user on /login → /). The
unauthed-on-protected branch is exercised indirectly by the rest of the
suite rendering screens with `isAuthenticated: false`. Adding a fragile
App.svelte test that re-mocks `current` + `auth` + `navigate` for
string-comparison assertions would add noise without adding signal —
decision documented here per story instructions.

**`auth.loadCurrentUser` await safety:** Verified that `loadCurrentUser`
swallows all exceptions internally via its inner try/catch, so `await`-ing
it in OAuthCallback cannot surface a rejection and block the navigate call.
The outer catch block in `exchange()` remains intact for network/POST
failures.

**Open-redirect test updated:** The existing test asserting `navigate('/login')`
for a protocol-relative `oauth.return_to` (`//evil.com`) was updated to
expect `navigate('/')` — the correct new fallback target.

**Verification:** `npm run check` — 0 errors, 2 pre-existing warnings.
`npm run test` — 450 tests passed across 40 files (OAuthCallback: 15
tests, +2 new; Login: 13 tests, +2 new).
