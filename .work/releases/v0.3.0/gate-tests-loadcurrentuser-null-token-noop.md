---
id: gate-tests-loadcurrentuser-null-token-noop
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

## Implementation notes

**Placement:** Added after the existing `'loadCurrentUser resolves without
throwing on network failure'` test (around line 122 in auth.test.ts). That
cluster tests unauthenticated/error paths of `loadCurrentUser` without
calling `setTokens`, so this new test fits there naturally.

**Pattern:** Follows the module-reset pattern: `beforeEach` does
`localStorage.clear()` + `vi.resetModules()`, so `await import('$lib/auth.svelte')`
gives a fresh store with `_token === null`. The test calls
`auth.loadCurrentUser()` without any `setTokens`, mocks a 200 /api/me
response, and asserts `currentUser` and `orgs` remain null.

**Guard mechanics:** In `auth.svelte.ts:73-77`, `tokenAtStart = _token = null`.
The guard `_token !== null && _token === tokenAtStart` short-circuits on the
first clause (`null !== null` → false), so the response is discarded.

**Negative-case verification:** Temporarily mutated the guard to
`_token === tokenAtStart` (removing `_token !== null &&`). With both
`_token` and `tokenAtStart` null, this becomes `null === null` → true, and
the response was written to state. The new test caught this regression:
`AssertionError: expected { id: 'u', email: 'x', … } to be null`. Production
code was then reverted and both test runs confirmed green (466/466).

**No design-flaw escape needed:** The guard fires as expected — the null-token
partition is real and the test exercises it correctly.

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Test fits the existing unauthenticated-path cluster in auth.test.ts. Exercises the null-token partition of the cross-tenant guard at auth.svelte.ts:77. Negative-case (removing the `_token !== null` clause) confirmed the test catches the regression: with both `_token` and `tokenAtStart` null, `_token === tokenAtStart` is true and the response would be written.
