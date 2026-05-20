---
id: gate-tests-app-authed-on-login-redirect
kind: story
stage: implementing
tags: [testing]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: tests
created: 2026-05-20
updated: 2026-05-20
---

# App.svelte authed-on-login gate redirect has no direct test

## Priority
High

## Spec reference
Item: `spa-logged-in-landing-authed-redirect-fixes`
Acceptance criterion: "`App.svelte`'s auth-gate `$effect` redirects to `'/'` when `auth.isAuthenticated && current.name === 'login'`."

## Gap type
missing test for valid partition / e2e-seam

## Suggested test
```ts
// App.test.ts (new)
it('redirects authed user landing on /login to /', async () => {
  mockAuth.isAuthenticated = true;
  mockRouterCurrent.name = 'login';
  render(App);
  await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/'));
});

it('still redirects unauthed user on protected route to /login', async () => {
  mockAuth.isAuthenticated = false;
  mockRouterCurrent.name = 'sessions';
  render(App);
  await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/login'));
});

it('preserves return_to for invite-accept while unauthed', async () => {
  // covers AC: "still handles invite-accept case with ?return_to= preservation"
});
```

## Test location (suggested)
`frontend/src/App.test.ts` (new)

## Context
The story's implementation note explicitly defers App.test.ts on the
grounds that Login.svelte's own `$effect` catches authed-on-login. But
the spec calls out **intentional redundancy** (defense-in-depth) — both
gates are *supposed* to fire. The Login.svelte effect test covers one
branch; the App.svelte gate is presently un-asserted. If someone deletes
Login.svelte's effect later thinking App.svelte covers it, the gate must
actually cover it — and there's no test pinning that.

Additionally the AC "still redirects unauthed users on protected routes"
and the invite-accept return_to preservation are not covered by any
existing test of the gate itself.

A new App.test.ts also closes `gate-tests-app-bootstrap-effect`.
