---
id: wire-load-current-user-to-me-endpoint
kind: story
stage: implementing
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
