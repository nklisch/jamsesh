---
id: spa-logged-in-landing-authed-redirect-fixes
kind: story
stage: done
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

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none.
**Important**: none.
**Nits**: none worth filing.

**Notes**:

- OAuthCallback's `await auth.loadCurrentUser()` is safe — `loadCurrentUser`
  swallows exceptions internally (`auth.svelte.ts:73-78`), so the await
  cannot surface a rejection that blocks the subsequent `navigate(returnTo ?? '/')`
  call. Verified by reading the post-fix `auth.svelte.ts`.
- 401-path edge case traced through: token issued at OAuth callback turns out
  invalid → `/api/me` returns 401 → `unauthorizedMiddleware` fires
  `auth.signOut()` (clears tokens, navigates to `/login`) → in-flight call
  resolves with `data: undefined` → write-guard's `if (data && ...)` skips →
  OAuthCallback's subsequent `navigate(returnTo ?? '/')` lands on `/` →
  App.svelte gate sees unauthed-on-`/` and redirects to `/login`. Net result:
  user ends up on `/login` with a clean store. Two extra navigation hops but
  no security issue and no UI thrash visible (the OAuth screen is full-bleed
  during this whole sequence).
- App.svelte's new top-branch only matches `current.name === 'login'`, leaving
  the existing oauth-callback and magic-link exclusions untouched. Verified.
- The intentional redundancy between Login.svelte's `$effect` and App.svelte's
  gate (both catch authed-on-`/login`) converges correctly via Svelte 5's
  $effect re-run semantics: whichever fires first navigates; the other re-runs
  against the new route and either no-ops or harmlessly re-navigates. Confirmed
  acceptable — defense in depth, no behavioral surprise.
- Open-redirect protection intact: the existing `startsWith('/') &&
  !startsWith('//')` validator in OAuthCallback (lines 35-38) still rejects
  protocol-relative `oauth.return_to`. Fallback target changed from `/login`
  to `/`, both in-origin. The "rejects protocol-relative return_to" test was
  updated to expect the new fallback.
- Test additions: 2 new in Login.test.ts (authed-redirect with/without
  returnTo), 1 new + 2 updated in OAuthCallback.test.ts (call-order
  assertion + navigate-target updates). The call-order assertion uses
  vitest's `mock.invocationCallOrder` to prove `loadCurrentUser` fired
  before `navigate`. Good idiom.
- Design alignment: no deviations. Implementation matches Unit 3 verbatim.
- Foundation-doc alignment: none affected.
- Security: lightweight — no new attack surface, redirect-target literal,
  open-redirect protection retained.
- Test integrity: clean. No silenced tests; the 3 navigate-target
  reassertions reflect the genuine spec change.

**What's now possible**: the SPA's authentication flow is closed-loop end to
end. A freshly-signed-up GitHub user clicks OAuth → exchange completes →
`auth.loadCurrentUser()` populates identity → user lands on `/` →
auto-routed to their only org (if exactly one), shown the picker (2+), or
shown the create-first-org screen (zero). An already-authed user landing on
`/login` (browser back, bookmark) bounces to `/` instead of staring at a
form they don't need. The feature ships its promise — no dead ends for new
signups.

**Verification (review-side)**: full diff read of all 6 changed files,
tests at 451/451 pass.

## Children complete

This is the final child story of `spa-logged-in-landing-and-org-bootstrap`.
With all three children at `stage: done`, the parent feature (currently at
`stage: review`) is ready for its own feature-level review pass.
