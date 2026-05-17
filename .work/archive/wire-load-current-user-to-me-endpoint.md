---
id: wire-load-current-user-to-me-endpoint
kind: story
stage: done
tags: [ui]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Wire `loadCurrentUser` to GET /api/me

## Why

`frontend/src/lib/auth.svelte.ts:50-55` ships as a stub:

```ts
async loadCurrentUser(): Promise<void> {
  // TODO: call GET /api/me once epic-portal-foundation-accounts ships.
  // Once paths has an entry for /api/me, this becomes:
  //   const { data } = await client.GET('/api/me');
  //   if (data) _currentUser = data;
},
```

The dependency it was waiting for is shipped:

- `docs/openapi.yaml:1528` defines `/api/me`
- `frontend/src/lib/api/types.gen.ts` exposes the path
- `internal/portal/accounts/handlers.go:35-69` implements `GetMe`
- `epic-portal-foundation` is at `stage: done`

The TODO is now actively misleading. This story replaces the stub with a
real implementation so any caller of `auth.loadCurrentUser()` actually
populates `_currentUser`.

This is **not tagged `[refactor]`** because it changes behavior (an
intentional stub becomes a working call). It's small and well-scoped so
it ships as a single-file story rather than a feature design pass.

## Files

- Modify: `frontend/src/lib/auth.svelte.ts`
- New (probably): `frontend/src/lib/auth.test.ts` — add a test that exercises
  `loadCurrentUser` against a mocked client

## Target shape

```ts
async loadCurrentUser(): Promise<void> {
  const { data } = await client.GET('/api/me');
  if (data) {
    _currentUser = {
      id: data.id,
      email: data.email,
      displayName: data.display_name,  // or data.displayName — match generated types
    };
  }
},
```

Confirm the exact field names from `types.gen.ts` during implementation —
the generator may emit snake_case or camelCase depending on
openapi-typescript configuration.

## Acceptance

- [ ] `loadCurrentUser` issues `client.GET('/api/me')` and populates
      `_currentUser` on success
- [ ] On error (network failure, 401, 500), `_currentUser` is left as `null`
      and the error does not throw out of the call (silent failure is
      acceptable here; the UI handles the null state)
- [ ] At least one unit test covers the success path and one covers the
      failure path (with a mocked openapi-fetch client)
- [ ] The TODO comment is removed (no lingering "once X ships" notes)
- [ ] `pnpm test` (or `npm test`) under `frontend/` passes

## Risk

LOW. The function is currently a no-op so any behavior is additive. The
only risk is mis-shaping the response — caught by the unit test.

## Rollback

`git revert` the commit; the stub is restored.

## Implementation notes

- TODO comment removed; `loadCurrentUser` now issues a real `client.GET('/api/me')` call.
- Added `import { client } from '$lib/api/client'` to `auth.svelte.ts`. The circular dependency (`client.ts` imports `auth.svelte.ts` for the bearer token; `auth.svelte.ts` now imports `client.ts`) is safe — JavaScript module system resolves circular refs via live bindings and both modules initialize correctly.
- Test approach: 3 tests added using `vi.spyOn(globalThis, 'fetch')` at the fetch layer rather than `vi.doMock('$lib/api/client')`. This mirrors the pattern already used in `client.test.ts` and sidesteps any circular-dependency complications with module mocking. The `client.ts` module already uses a `lateFetch` wrapper that routes through `globalThis.fetch`, making `vi.spyOn` reliable.
- Field mapping: `MeResponse.display_name` (snake_case from openapi-typescript generator) maps to `_currentUser.displayName` (camelCase in internal rune state).
- Test count: 3 new tests (success path, failure/no-throw path, call-shape assertion). Full suite: 32 files, 286 tests, all passing.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none

**Nits**:
- The new `auth.svelte.ts ↔ client.ts` circular import is functionally safe
  (lazy reference: `client` is only read inside `loadCurrentUser`'s async
  body, not at module init). A future cleanup could extract a small
  `token-getter` module that both files import to break the cycle, but
  it's not worth a follow-up item right now — the cycle is documented in
  the implementation notes and the pattern matches how `client.test.ts`
  already mocks the layer.

**Notes**: TODO removed, snake_case → camelCase mapping (`display_name` →
`displayName`) is correct against the generated `MeResponse` type. Three
tests added at the fetch layer via `vi.spyOn(globalThis, 'fetch')` —
consistent with the established pattern in `client.test.ts`. Silent
failure on network/parse error is the right call (UI handles null
`currentUser`). Full frontend suite (300 tests) passes.
