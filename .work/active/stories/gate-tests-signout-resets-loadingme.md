---
id: gate-tests-signout-resets-loadingme
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

# `auth.signOut()` resetting `_loadingMe` not verified

## Priority
High

## Spec reference
Item: `spa-logged-in-landing-auth-store-orgs-cache`
Acceptance criterion: "`auth.signOut()` clears `_orgs` to `null` and **resets `_loadingMe`**."

## Gap type
missing test for valid partition / adversarial-spec-silent

## Suggested test
```ts
test('signOut while a loadCurrentUser is in-flight allows a subsequent loadCurrentUser to fetch again', async () => {
  // Without _loadingMe reset, a sign-out + sign-in in the same tab would
  // see _loadingMe pinned to the abandoned (resolved-but-discarded) promise
  // and the next loadCurrentUser would await it as a no-op, never firing
  // a fresh fetch for the new user.
  const fetchSpy = vi.spyOn(globalThis, 'fetch')
    .mockResolvedValueOnce(/* user A */)
    .mockResolvedValueOnce(/* user B */);
  auth.setTokens('a','a');
  const p1 = auth.loadCurrentUser();
  auth.signOut();         // must reset _loadingMe
  await p1;
  auth.setTokens('b','b');
  await auth.loadCurrentUser();
  expect(fetchSpy).toHaveBeenCalledTimes(2);
});
```

## Test location (suggested)
`frontend/src/lib/auth.test.ts`

## Context
Spec calls out `_loadingMe = null` explicitly in signOut, and the
implementation does it (`auth.svelte.ts:58`), but the existing
`signOut clears orgs to null` test only verifies `_orgs`. The
stale-write race test ("discards stale /api/me response when signOut
raced...") asserts that state isn't poisoned, but does NOT assert that
a subsequent loadCurrentUser can actually fetch again — the AC's second
half is untested.
