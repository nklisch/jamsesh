---
id: gate-tests-app-bootstrap-effect
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

# App.svelte bootstrap-effect (cold-load `loadCurrentUser`) has no direct test

## Priority
High

## Spec reference
Item: `spa-logged-in-landing-auth-store-orgs-cache`
Acceptance criterion: "App.svelte's new effect calls `loadCurrentUser()` once on cold-load when `auth.isAuthenticated && auth.orgs === null` (verify via fetch-mock call count = 1)." AND "App.svelte's new effect does NOT call `loadCurrentUser()` when `auth.isAuthenticated && auth.orgs !== null`."

## Gap type
missing test for valid partition

## Suggested test
```ts
// App.test.ts (new)
it('cold-load: calls auth.loadCurrentUser() once when authed and orgs is null', () => {
  mockAuth.isAuthenticated = true;
  mockAuth.orgs = null;
  render(App);
  expect(mockLoadCurrentUser).toHaveBeenCalledTimes(1);
});

it('does NOT call loadCurrentUser when authed and orgs already loaded', () => {
  mockAuth.isAuthenticated = true;
  mockAuth.orgs = [];
  render(App);
  expect(mockLoadCurrentUser).not.toHaveBeenCalled();
});
```

## Test location (suggested)
`frontend/src/App.test.ts` (new — same file as `gate-tests-app-authed-on-login-redirect`)

## Context
The story's "implementation notes" defer the App.svelte bootstrap-effect
test claiming the `loadCurrentUser` idempotency unit tests cover it.
That's not the same surface — the unit tests verify the function's
internal idempotency guard, not whether `App.svelte` invokes it once and
only once under the right preconditions. The AC explicitly says "verify
via fetch-mock call count = 1" against the **effect**, not the function.
