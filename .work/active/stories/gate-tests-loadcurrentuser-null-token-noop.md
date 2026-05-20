---
id: gate-tests-loadcurrentuser-null-token-noop
kind: story
stage: drafting
tags: [testing]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: tests
created: 2026-05-20
updated: 2026-05-20
---

# `loadCurrentUser` no-op when unauthenticated (null `_token` path) untested

## Priority
Medium

## Spec reference
Item: `spa-logged-in-landing-auth-store-orgs-cache`
Acceptance criterion: Implicit in the cross-tenant guard — "discard the response if signOut (or a sign-in as a different user) raced this call". Plus review nit: "The new semantics (loadCurrentUser is a no-op when not authenticated)".

## Gap type
missing test for valid partition / boundary

## Suggested test
```ts
it('loadCurrentUser discards response when _token is null at completion', async () => {
  // Call loadCurrentUser WITHOUT setTokens — token never set, response should not write state.
  vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
    new Response(JSON.stringify({ id: 'u', email: 'x', display_name: 'X', orgs: [] }),
                 { status: 200, headers: { 'Content-Type': 'application/json' } }));
  const { auth } = await import('$lib/auth.svelte');
  await auth.loadCurrentUser();
  expect(auth.currentUser).toBeNull();
  expect(auth.orgs).toBeNull();
});
```

## Test location (suggested)
`frontend/src/lib/auth.test.ts`

## Context
The race-test sets up a token-rotation scenario, but the `_token === null`
partition of the guard (no token ever set) is not directly tested. This
is the everyday "Login.svelte mounts before tokens exist" path — the
second clause of the guard (`_token !== null && _token === tokenAtStart`)
has a null branch that gets no assertion.
