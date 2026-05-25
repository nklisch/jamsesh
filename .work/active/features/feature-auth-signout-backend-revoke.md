---
id: feature-auth-signout-backend-revoke
kind: feature
stage: implementing
tags: [security, portal, ui, auth, tokens]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# Sign-out backend revoke

## Brief

Today `signOut()` in `frontend/src/lib/auth.svelte.ts` clears local
tokens but never tells the backend. The access token and (more
importantly) the refresh token stay valid on the server until their
natural expiry, so a token that leaked earlier (browser extension,
shoulder-surf, malware) remains replayable through a "sign-out" event.

This feature adds a server-side revoke endpoint and wires the SPA's
sign-out flow to call it best-effort. It is cleanly scoped — one new
backend route, one DB query (mark refresh token revoked), and a
best-effort SPA call that does not block local sign-out on network
failure. Two prior autopilot triages flagged this as needing
feature-scope design rather than a single-stride story; this feature
captures that.

## Design decisions

- **Endpoint shape**: `POST /api/auth/logout` with no request body — the
  Bearer token in the `Authorization` header is the credential. The server
  reads the authenticated account from context (already set by
  `BearerMiddleware`) and revokes all tokens for that account. No body
  required; simpler than `/api/auth/session/revoke` with an explicit body,
  and clearly distinct from the existing `/api/auth/revoke` (which requires
  passing a `token` field and is used for cross-device revocation).

- **Access token revocation**: Yes, revoke all tokens for the account
  (`revokeAll: true` semantics) using the existing
  `RevokeAllOAuthTokensForAccount` query. The Bearer token passed by the
  caller identifies the account; no need to separately pass the refresh
  token. This is simpler and more secure than relying on short TTL alone —
  both access (1h) and refresh (30d) tokens are invalidated in one query.
  The `Revoke` service method already supports this via `revokeAll: true`.

- **Server-side state**: The `oauth_tokens` table already has a `revoked_at`
  column and `RevokeAllOAuthTokensForAccount` query. No schema changes, no
  new table. The middleware (`BearerMiddleware` → `Validate` → checks
  `revoked_at`) already enforces revocation on every subsequent request.

- **Story split**: Two stories — backend first (endpoint + spec + wiring +
  handler tests), frontend second (make `signOut()` async, best-effort
  `POST /api/auth/logout` call, update tests). Sequential dependency:
  frontend story `depends_on` the backend story.

## Architectural choice

The logout endpoint is a thin method on the existing `tokens.Handler` — no
new package, no new service interface method needed. `Revoke(ctx,
accountID, bearerToken, revokeAll=true)` in `tokens.Service` already does
the right thing. The handler reads the account from context (same as
`RevokeToken`) and the raw token from the `Authorization` header (same as
`BearerMiddleware` — strip `Bearer ` prefix). One new operation on an
existing handler; one new route in the authenticated group in `main.go`; one
new openapi path in the spec.

**Why not reuse `POST /api/auth/revoke`?** The existing endpoint requires
passing `token` in the request body and supports selective single-token
revocation. The SPA would need to pass `refresh` in the body (which it has
at logout time), but that conflates two concerns. `POST /api/auth/logout` is
cleaner semantics for "the current session is ending; revoke everything" and
gives the frontend a zero-body call.

## Implementation Units

### Unit 1: OpenAPI spec — `POST /api/auth/logout`

**File**: `docs/openapi.yaml`
**Story**: `feature-auth-signout-backend-revoke-backend`

```yaml
  /api/auth/logout:
    post:
      summary: Revoke all tokens for the authenticated account (sign-out)
      operationId: logout
      security:
        - bearerAuth: []
      responses:
        '204':
          description: All tokens revoked; local state may now be cleared
        '401':
          $ref: '#/components/responses/Unauthorized'
```

No request body — the Bearer header is the credential. No response body —
`204 No Content` is the success signal.

**Implementation Notes**:
- Insert after `POST /api/auth/revoke` (line ~1807) in the spec.
- `operationId: logout` generates `Logout` / `LogoutRequestObject` /
  `LogoutResponseObject` in the openapi Go and TS types after `make generate`.

**Acceptance Criteria**:
- [ ] `make generate` succeeds; `LogoutRequestObject` and
  `Logout204Response` appear in `internal/api/openapi/`.
- [ ] TS types in `frontend/src/lib/api/types.gen.ts` include
  `/api/auth/logout` path with `post` method.

---

### Unit 2: Backend handler — `tokens.Handler.Logout`

**File**: `internal/portal/tokens/handlers.go`
**Story**: `feature-auth-signout-backend-revoke-backend`

```go
// Logout implements POST /api/auth/logout.
// Revokes all tokens for the authenticated account. The Bearer token in the
// Authorization header identifies both the caller and the account to revoke.
// No request body required.
//
// auth flow: same import-cycle constraint as RevokeToken — AccountFromContext
// is called directly rather than via handlerauth.
func (h *Handler) Logout(ctx context.Context, req openapi.LogoutRequestObject) (openapi.LogoutResponseObject, error) {
    acct, ok := AccountFromContext(ctx)
    if !ok {
        return openapi.Logout401JSONResponse{
            UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
                Error:   "auth.invalid_token",
                Message: "invalid token",
            },
        }, nil
    }

    // Extract the raw bearer token from context so Revoke can locate the
    // row to determine account ownership (Revoke re-fetches by hash).
    // We pass revokeAll=true: logout revokes ALL tokens for the account.
    rawToken := bearerFromContext(ctx)
    if err := h.svc.Revoke(ctx, acct.ID, rawToken, true); err != nil {
        // ErrForbidden cannot occur: we pass acct.ID as both caller and
        // the token owner. Any other error → 401 (don't leak internals).
        return openapi.Logout401JSONResponse{
            UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
                Error:   "auth.invalid_token",
                Message: "invalid token",
            },
        }, nil
    }
    return openapi.Logout204Response{}, nil
}
```

The raw bearer token must be threaded through context. Add a context key
and helper in `middleware.go`:

```go
type rawBearerCtxKey struct{}

// rawBearerMiddleware wraps BearerMiddleware's functionality but also stores
// the raw token string in context so handlers can pass it to service methods
// that need to look up the token row by hash.
//
// In practice: BearerMiddleware already validates and sets the account.
// We add rawBearerFromContext here for the Logout handler; other handlers
// don't need the raw token.
func rawBearerFromContext(ctx context.Context) string {
    v, _ := ctx.Value(rawBearerCtxKey{}).(string)
    return v
}
```

**Implementation Notes**:
- Modify `BearerMiddleware` to also store the raw token in context using
  `rawBearerCtxKey{}` before calling `next.ServeHTTP`. This is a purely
  additive change — existing consumers of `BearerMiddleware` are unaffected.
- `Revoke(ctx, acct.ID, rawToken, true)` uses the existing code path: hashes
  the raw token, fetches the row, ownership-checks against `acct.ID`, then
  calls `RevokeAllOAuthTokensForAccount`. All existing paths remain.
- No new service interface method needed — `Revoke` already supports
  `revokeAll: true`.

**Acceptance Criteria**:
- [ ] `POST /api/auth/logout` with a valid Bearer token returns `204`.
- [ ] After `204`, subsequent requests with the same token return `401
  auth.invalid_token` (the token was revoked).
- [ ] After `204`, the refresh token is also revoked (validate via
  `POST /api/auth/refresh` returning `401`).
- [ ] `POST /api/auth/logout` without a Bearer header returns `401`.

---

### Unit 3: Route wiring

**File**: `cmd/portal/main.go`
**Story**: `feature-auth-signout-backend-revoke-backend`

Add to the authenticated `r.Group` block (alongside `r.Post("/auth/revoke", ...)`):

```go
r.Post("/auth/logout", apiWrapper.Logout)
```

**Acceptance Criteria**:
- [ ] Route is registered in the Bearer-protected group (not the public group).
- [ ] `make generate && go build ./...` succeeds.

---

### Unit 4: Handler tests

**File**: `internal/portal/tokens/handlers_test.go`
**Story**: `feature-auth-signout-backend-revoke-backend`

Add `Logout` to `tokensOnlyHandler` shim (panic stub). Wire the route in
`newTestEnv`. Add test cases:

```go
func TestHandler_Logout_RevokesAllTokens(t *testing.T) { ... }
func TestHandler_Logout_NoBearerReturns401(t *testing.T) { ... }
func TestHandler_Logout_TokenStillWorksBeforeCall(t *testing.T) { ... }
```

Test shape: `mustIssue` → `POST /api/auth/logout` with access Bearer →
assert `204` → `GET /api/me` with same token → assert `401`.

**Implementation Notes**:
- Also add `POST /api/auth/logout` to the `newTestEnv` chi router inside the
  Bearer-protected group (mirrors the wiring in `main.go`).
- Add `Logout` stub to `tokensOnlyHandler` (same panic pattern as the other
  shims).

**Acceptance Criteria**:
- [ ] All three test cases pass.
- [ ] `tokensOnlyHandler` implements `openapi.StrictServerInterface` (compile
  check at bottom of file).

---

### Unit 5: Frontend — async `signOut` with best-effort logout call

**File**: `frontend/src/lib/auth.svelte.ts`
**Story**: `feature-auth-signout-backend-revoke-frontend`

```typescript
async signOut(): Promise<void> {
  // Best-effort: tell the server to revoke all tokens for this account.
  // Ignore failures — network down, server error, or already-revoked tokens
  // must not block the local sign-out. The user is still signed out locally
  // even if the server call fails.
  if (_token) {
    try {
      await client.POST('/api/auth/logout');
    } catch {
      // Swallow: offline sign-out still clears local state.
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

**Implementation Notes**:
- `signOut` becomes `async`. All callers currently call `auth.signOut()` and
  do not `await` the result — this is intentional. The sign-out is
  fire-and-forget from the UI's perspective (the navigate happens inside
  `signOut` itself).
- `client.POST('/api/auth/logout')` uses the existing `bearerMiddleware` in
  `client.ts` to attach the `Authorization` header. No changes to the client
  singleton needed.
- The `unauthorizedMiddleware` in `client.ts` intercepts `auth.*` 401s and
  calls `auth.signOut()`. This cannot recurse: by the time the response
  arrives, the first `signOut` call has already nulled `_token`, so the
  second invocation of `signOut` skips the `if (_token)` guard and the
  `client.POST` call entirely. Idempotent.
- Callers in `Home.svelte` and `SessionsLanding.svelte` use
  `onclick={() => auth.signOut()}`. These remain syntactically valid — an
  async function called without `await` is still valid JS/TS; the returned
  `Promise` is ignored. No call-site changes required.

**Acceptance Criteria**:
- [ ] `signOut()` calls `POST /api/auth/logout` before clearing local state
  when `_token` is non-null.
- [ ] Network failure or server error does not block local sign-out
  (token/orgs cleared, `/login` navigated regardless).
- [ ] `signOut()` when already signed out (`_token === null`) does not call
  the endpoint.
- [ ] Concurrent `signOut()` calls (e.g. `unauthorizedMiddleware` racing a
  user click) do not double-call the endpoint (second call skips due to
  `_token` already null).
- [ ] Return type change (`void` → `Promise<void>`) satisfies TypeScript;
  no call-site changes needed (callers may ignore the promise).

---

### Unit 6: Frontend tests

**File**: `frontend/src/lib/auth.test.ts`
**Story**: `feature-auth-signout-backend-revoke-frontend`

Existing tests mock `navigate` but do not mock `client.POST`. Extend the
test suite:

```typescript
// signOut calls POST /api/auth/logout before clearing state
test('signOut calls POST /api/auth/logout before clearing local state', ...)
// signOut does not call endpoint when token is null
test('signOut when unauthenticated does not call POST /api/auth/logout', ...)
// signOut still clears local state on network error from logout endpoint
test('signOut clears state even when POST /api/auth/logout throws', ...)
// signOut clears state even when POST /api/auth/logout returns 4xx
test('signOut clears state even when POST /api/auth/logout returns 401', ...)
```

**Implementation Notes**:
- Use the `spa-test-module-mock-barrel` pattern: mock `$lib/api/client` at
  the top of the test file with `vi.mock(...)`, expose `mockPost = vi.fn()`
  through the factory, and control resolve/reject per test.
- Existing tests that call `auth.signOut()` and then assert on state will
  need to `await auth.signOut()` (the method is now async). Update the
  existing test call sites.

**Acceptance Criteria**:
- [ ] All existing `signOut` tests continue to pass (after updating `await`).
- [ ] Four new test cases pass.
- [ ] `client.POST` mock is not called when `_token` is null.

---

## Implementation Order

1. `feature-auth-signout-backend-revoke-backend` — spec + handler + route +
   tests (all changes internal to Go/openapi; no frontend dependency)
2. `feature-auth-signout-backend-revoke-frontend` — async signOut + frontend
   tests (depends on backend story completing so the endpoint exists)

## Testing

### Backend unit tests: `internal/portal/tokens/handlers_test.go`
- Happy path: valid Bearer → `204` → subsequent calls with same token → `401`
- No bearer → `401`
- Already-revoked token → `204` (idempotent via existing `Revoke` logic)

### Frontend unit tests: `frontend/src/lib/auth.test.ts`
- `POST /api/auth/logout` called exactly once per sign-out (token non-null)
- Error from server → local state still cleared
- Double call race → endpoint called at most once

### Integration (manual / e2e)
- Sign in → sign out → verify subsequent API calls with the old token return
  `401 auth.invalid_token`

## Risks

- **`signOut` async return type** — callers ignore the returned `Promise`.
  TypeScript does not flag this as an error; the old behavior (synchronous
  navigate) still happens because the `navigate('/login')` call is at the
  end of the async function body. Low risk.
- **`unauthorizedMiddleware` recursion** — if `POST /api/auth/logout` itself
  returns `auth.*` 401, `unauthorizedMiddleware` calls `auth.signOut()`
  again. The `if (_token)` guard prevents the second call from hitting the
  endpoint again (token is already null). Verified idempotent. Low risk.
- **`BearerMiddleware` context mutation** — adding `rawBearerCtxKey` to
  the context is purely additive. Existing consumers of `AccountFromContext`
  are unaffected. Low risk.
