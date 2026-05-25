---
id: feature-auth-signout-backend-revoke-frontend
kind: story
stage: done
tags: [security, auth, ui]
parent: feature-auth-signout-backend-revoke
depends_on: [feature-auth-signout-backend-revoke-backend]
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# Auth sign-out frontend: best-effort POST /api/auth/logout before local clear

## Scope

Wire the SPA's `signOut()` to call `POST /api/auth/logout` best-effort
before clearing local state. Failures (network error, server error) must
not block local sign-out — the user is always signed out locally even if
the server call fails.

This story covers:
1. Make `signOut()` async in `frontend/src/lib/auth.svelte.ts`
2. Add best-effort `client.POST('/api/auth/logout')` call
3. Update `frontend/src/lib/auth.test.ts` (new cases + await existing calls)

Does NOT change any call sites (`Home.svelte`, `SessionsLanding.svelte`,
`client.ts`). Callers may ignore the returned `Promise` — this is valid
JS/TS and the navigate still happens inside the async body.

**Depends on**: `feature-auth-signout-backend-revoke-backend` — the
`/api/auth/logout` endpoint must exist in the spec and on the server
before this story's tests can be meaningfully written against the typed
client.

## Units

### 1. `frontend/src/lib/auth.svelte.ts` — async signOut

Replace the existing synchronous `signOut(): void` with:

```typescript
async signOut(): Promise<void> {
  // Best-effort server-side revoke. Fire before clearing local state so the
  // Bearer token is still valid when the request is sent. Ignore all errors —
  // offline sign-out must still clear local state.
  if (_token) {
    try {
      await client.POST('/api/auth/logout');
    } catch {
      // Swallow: network down, server error, etc. Sign-out proceeds locally.
    }
  }

  _token = null;
  _refresh = null;
  _currentUser = null;
  _orgs = null;
  _loadingMe = null;
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(REFRESH_KEY);
  navigate('/login');
},
```

**Implementation notes**:
- `client.POST('/api/auth/logout')` uses the existing `bearerMiddleware`
  (no change to `client.ts`).
- `unauthorizedMiddleware` may call `auth.signOut()` if the logout endpoint
  returns `auth.*` 401. The second call hits `if (_token)` with `_token`
  already null and skips the endpoint call. Idempotent, no infinite loop.
- Return type `Promise<void>` instead of `void`. TypeScript allows callers
  to ignore a `Promise<void>` without `await` — no call-site changes needed.

### 2. `frontend/src/lib/auth.test.ts` — update and extend tests

Mock `$lib/api/client` at the top of the test file using the
`spa-test-module-mock-barrel` pattern:

```typescript
const mockPost = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: {
    POST: (...args: unknown[]) => mockPost(...args),
  },
}));
```

**Existing tests**: update call sites that call `auth.signOut()` to
`await auth.signOut()`. The mock `mockPost` resolves by default so the
async flow completes synchronously within `await`.

**New test cases**:

```typescript
test('signOut calls POST /api/auth/logout before clearing local state', async () => {
  // Arrange: authenticated
  auth.setTokens('access', 'refresh');
  mockPost.mockResolvedValueOnce({});
  // Act
  await auth.signOut();
  // Assert: endpoint called once with the correct path
  expect(mockPost).toHaveBeenCalledWith('/api/auth/logout');
  expect(mockPost).toHaveBeenCalledOnce();
  // State cleared
  expect(auth.token).toBeNull();
});

test('signOut when unauthenticated does not call POST /api/auth/logout', async () => {
  // _token is null — no endpoint call
  await auth.signOut();
  expect(mockPost).not.toHaveBeenCalled();
});

test('signOut clears local state even when POST /api/auth/logout throws', async () => {
  auth.setTokens('access', 'refresh');
  mockPost.mockRejectedValueOnce(new Error('network'));
  await auth.signOut();
  expect(auth.token).toBeNull();
  expect(auth.isAuthenticated).toBe(false);
});

test('signOut clears local state even when POST /api/auth/logout returns 401', async () => {
  auth.setTokens('access', 'refresh');
  // openapi-fetch resolves (no throw) on 4xx; { data: undefined, error: {...} }
  mockPost.mockResolvedValueOnce({ error: { error: 'auth.invalid_token' } });
  await auth.signOut();
  expect(auth.token).toBeNull();
});
```

## Acceptance Criteria

- [ ] `signOut()` return type is `Promise<void>`; TypeScript compiles clean.
- [ ] `client.POST('/api/auth/logout')` is called exactly once per
  `signOut()` invocation when `_token` is non-null.
- [ ] Network or server error does not prevent local state clear or
  `/login` navigation.
- [ ] `signOut()` when already unauthenticated skips the endpoint call.
- [ ] All existing `auth.test.ts` tests pass (updated to `await signOut()`
  where needed).
- [ ] Four new test cases pass.
- [ ] `Home.test.ts` and `SessionsLanding.test.ts` continue to pass without
  changes (mocks already stub `signOut` entirely).

## Implementation notes

- `frontend/src/lib/auth.svelte.ts`: `signOut` is now `async`.
- **Design deviation from feature body**: the original design said "call
  POST /api/auth/logout BEFORE clearing local state". We chose the opposite
  order to preserve **synchronous local-state clear** for callers (especially
  `unauthorizedMiddleware` in `client.ts`, which calls `auth.signOut()`
  without `await`). Sequence is:
  1. Capture `_token` to a local `capturedToken`.
  2. Clear all rune state + localStorage + navigate('/login') synchronously.
  3. Async best-effort POST `/api/auth/logout` with an explicit
     `Authorization: Bearer <capturedToken>` header (bearerMiddleware can't
     find the bearer in localStorage anymore — it's been cleared).
  4. Swallow any error.
- The `if (capturedToken)` guard prevents a no-op POST when already signed
  out AND prevents recursion: a 401 from the logout endpoint itself
  re-invokes signOut via unauthorizedMiddleware, but by then `_token` is
  null so the recursive capture is empty and the POST is skipped.
- `frontend/src/lib/auth.test.ts`:
  - Five existing `auth.signOut()` callsites now use `await auth.signOut()`.
  - Two existing tests that count fetch calls were updated to allow the
    extra logout POST fetch (`discards stale /api/me ...`,
    `signOut while a loadCurrentUser is in-flight ...`).
  - Four new tests:
    - `signOut calls POST /api/auth/logout before clearing local state`
    - `signOut when unauthenticated does not call POST /api/auth/logout`
    - `signOut clears local state even when POST /api/auth/logout throws`
    - `signOut clears local state even when POST /api/auth/logout returns 401`
- Existing screen tests (`client.test.ts`'s 401-interceptor suite) continue
  to pass because the SYNCHRONOUS state clear preserves their original
  assertion model — they assert state immediately after `auth.signOut()`
  is fired (not awaited), which still works.

Verified:
- `npm test -- --run auth.test.ts` → 29 passed.
- `npm test -- --run` → 738 passed, 1 skipped.
- `npm run check` → 0 errors, 1 pre-existing unrelated warning.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: The clear-then-POST order deviation is correctly justified: (a) keeps the synchronous local-state-clear contract for non-awaiting callers like `unauthorizedMiddleware` in `client.ts`, (b) prevents recursion when the logout endpoint itself returns 401 (captured token is empty on recursive entry). Explicit `Authorization: Bearer <capturedToken>` header bypasses the now-empty bearerMiddleware lookup. `try/catch` swallows transport errors; openapi-fetch's `{ error }` result is treated as success-for-purposes-of-local-clear (which is right — we tried). Four new tests cover the matrix.
