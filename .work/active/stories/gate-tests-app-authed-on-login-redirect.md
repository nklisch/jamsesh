---
id: gate-tests-app-authed-on-login-redirect
kind: story
stage: done
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

## Implementation notes

Tests live in `frontend/src/App.test.ts` under the `describe('App — auth-gate $effect', ...)` block.

### Test cases (auth-gate `$effect`)

1. **redirects authed user on /login to /**
   `mockAuth.isAuthenticated = true`, `mockRouterCurrent.name = 'login'` → `waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/'))`. Pins the defense-in-depth redirect that's intentionally redundant with Login.svelte's own effect.

2. **redirects unauthed user on protected route to /login**
   `isAuthenticated = false`, `current.name = 'sessions'` → `waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/login'))`. Covers the base redirect for any protected route.

3. **preserves ?return_to=<original> for unauthed invite-accept visitor**
   Stubs `window.location.pathname` to the invite URL, sets `current.name = 'invite-accept'` → asserts `navigate` was called with `/login?return_to=<urlencoded-path>`. Covers the special-case branch in the auth-gate.

4. **does NOT redirect unauthed user on /login** (boundary)
   Prevents regression of an infinite redirect loop on the login page itself.

5. **does NOT redirect unauthed user on magic-link route** (boundary)
   Magic-link is excluded from the gate so the token exchange can complete.

6. **does NOT redirect unauthed user on oauth-callback route** (boundary)
   oauth-callback does its own post-exchange navigation; the gate must stay out of its way.

### Structural decisions

- All nine screen components imported by App.svelte are mocked with minimal Svelte 5 function stubs `(anchor, props) => {}`. Svelte 5's `mount()` internally calls the component as `Component(anchor_node, props)` (render.js:196), so the stub must be a plain function — a plain object would throw `default is not a function`.
- `mockAuth` and `mockRouterCurrent` are plain mutable objects exposed via `get` accessors in the `vi.mock` factory, matching the `spa-test-module-mock-barrel` pattern used in Home.test.ts.
- `window.location` stubbing uses `Object.defineProperty` per the `window-location-defineproperty-stub` pattern from Login.test.ts.

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: The 50ms `setTimeout` waits in the "does NOT navigate" assertions are pragmatic but slightly timing-dependent. Acceptable for now; could be replaced with a polling helper if CI flakiness ever surfaces. Filed only as a future-improvement note, no item created.

**Notes**: Test file goes beyond the spec's 3 minimum cases with 3 extra boundary checks that pin the exclusion list (`/login`, `/magic-link`, `/oauth-callback`) — each tied to an explanatory comment in App.svelte. Screen-component stubbing solution traced to a real Svelte 5 internals quirk and the discovery was parked separately as `idea-app-test-svelte5-component-mock-broken`. Auth + router mocks faithfully mirror the wrapper-object-rune-store production pattern.
