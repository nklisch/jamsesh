---
id: gate-tests-signout-resets-loadingme
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

## Implementation notes

Added test "signOut while a loadCurrentUser is in-flight allows a subsequent
loadCurrentUser to fetch again" in the `// --- _loadingMe reset on signOut ---`
section, immediately before the existing stale-write race block.

**Deviation from story snippet:** The story's sketch used
`mockResolvedValueOnce(/* user A */)` for both mocked responses, implying they
could be queued and awaited in-order without manual control. That would have
raced non-deterministically: if user A's response resolved before `signOut()`
ran, `_loadingMe` would already be null (cleared by the `finally` block) and the
test would pass vacuously regardless of whether `signOut()` resets `_loadingMe`.
To make the test actually exercise the invariant, user A's fetch was kept
controllable via a manually-resolved `Promise<Response>` (same pattern as the
existing stale-write race test), so `signOut()` is guaranteed to run while the
promise is still in-flight.

**Patterns reused:**
- `vi.doMock('$lib/router.svelte', ...)` for navigate stub (matches all signOut
  tests in the file).
- `vi.spyOn(globalThis, 'fetch').mockReturnValueOnce(fetchPromise).mockResolvedValueOnce(...)`
  chaining — same spy/chain shape as the `loadCurrentUser calls GET /api/me` and
  stale-write tests.
- `new Response(JSON.stringify({...}), { status: 200, headers: { 'Content-Type':
  'application/json' } })` for realistic `Response` objects the openapi-fetch
  client can call `.json()` on.
- `vi.resetModules()` in `beforeEach` gives each test a fresh module instance
  with isolated `_loadingMe` state.

Test count: 19 (was 18). All passing.

## Review (2026-05-20)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: The deviation from the story's suggested snippet is exactly right — the story's `mockResolvedValueOnce` for both responses would have made the test race-dependent and potentially vacuous. Using a controllable `Promise<Response>` for user A guarantees `signOut()` fires while `_loadingMe` is still set, which is the invariant the test exists to protect. Reuses the same control-the-in-flight-fetch pattern as the adjacent stale-write race test — good local consistency. This is the kind of design-flaw-in-the-suggestion-but-fix-it-in-the-impl call we want agents making.
