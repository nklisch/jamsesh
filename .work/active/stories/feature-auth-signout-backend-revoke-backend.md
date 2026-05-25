---
id: feature-auth-signout-backend-revoke-backend
kind: story
stage: done
tags: [security, auth, tokens]
parent: feature-auth-signout-backend-revoke
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# Auth sign-out backend: POST /api/auth/logout endpoint

## Scope

Implement the server-side `POST /api/auth/logout` endpoint so that the
SPA (and any other client) can revoke all tokens for the authenticated
account in a single call with no request body.

This story covers:
1. OpenAPI spec — add `POST /api/auth/logout` path (`operationId: logout`)
2. `tokens.Handler.Logout` method
3. Context helper to thread the raw bearer through `BearerMiddleware`
4. Route wiring in `cmd/portal/main.go`
5. Handler and service tests

Does NOT include frontend changes (see `feature-auth-signout-backend-revoke-frontend`).

## Units

### 1. OpenAPI spec (`docs/openapi.yaml`)

Insert after `POST /api/auth/revoke`:

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

Run `make generate` after adding the path. Verify `LogoutRequestObject`
and `Logout204Response` are generated in `internal/api/openapi/` and that
`/api/auth/logout` appears in `frontend/src/lib/api/types.gen.ts`.

### 2. Context helper in `internal/portal/tokens/middleware.go`

Add a `rawBearerCtxKey` and extend `BearerMiddleware` to store the raw
token in context (purely additive — existing consumers unaffected):

```go
type rawBearerCtxKey struct{}

func bearerFromContext(ctx context.Context) string {
    v, _ := ctx.Value(rawBearerCtxKey{}).(string)
    return v
}
```

In `BearerMiddleware`, after stripping the prefix and before calling
`svc.Validate`, store the raw token:

```go
ctx := context.WithValue(r.Context(), rawBearerCtxKey{}, tok)
// then also store the account after Validate succeeds:
ctx = context.WithValue(ctx, ctxKey{}, acct)
next.ServeHTTP(w, r.WithContext(ctx))
```

### 3. Handler method in `internal/portal/tokens/handlers.go`

```go
// Logout implements POST /api/auth/logout.
// Revokes all tokens for the authenticated account. No request body needed.
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
    rawToken := bearerFromContext(ctx)
    if err := h.svc.Revoke(ctx, acct.ID, rawToken, true); err != nil {
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

### 4. Route wiring in `cmd/portal/main.go`

Inside the Bearer-protected `r.Group`, alongside `r.Post("/auth/revoke", ...)`:

```go
r.Post("/auth/logout", apiWrapper.Logout)
```

### 5. Tests in `internal/portal/tokens/handlers_test.go`

- Add `Logout` panic stub to `tokensOnlyHandler`.
- Wire `r.Post("/api/auth/logout", strictAPI.Logout)` in `newTestEnv`'s
  Bearer-protected group.
- Add test cases:
  - `TestHandler_Logout_RevokesAllTokens`: issue pair → logout → confirm
    access token rejected → confirm refresh rejected.
  - `TestHandler_Logout_NoBearerReturns401`: no `Authorization` header → 401.
  - `TestHandler_Logout_IdempotentAfterAlreadyRevoked`: logout twice →
    second call also 204 (Revoke is idempotent via existing logic).

## Acceptance Criteria

- [ ] `make generate && go build ./...` succeeds.
- [ ] `POST /api/auth/logout` with valid Bearer returns `204`.
- [ ] After `204`, subsequent calls with the same access token return `401`.
- [ ] After `204`, `POST /api/auth/refresh` with the associated refresh
  token returns `401`.
- [ ] `POST /api/auth/logout` without a Bearer header returns `401`.
- [ ] All existing `tokens` package tests continue to pass.
- [ ] New handler tests pass.

## Implementation notes

- `docs/openapi.yaml`: added `POST /api/auth/logout` with `operationId: logout`,
  `bearerAuth` security, no request body, 204 + 401 responses. `make generate-api`
  regenerated `internal/api/openapi/server.gen.go` (LogoutRequestObject,
  Logout204Response, Logout401JSONResponse, route registration in
  HandlerFromMux) and `frontend/src/lib/api/types.gen.ts`.
- `internal/portal/tokens/middleware.go`: added `rawBearerCtxKey{}` and the
  pair `rawBearerFromContext(ctx) string` (package-private) +
  `ContextWithRawBearer(ctx, raw) context.Context` (exported for tests that
  bypass HTTP middleware). `BearerMiddleware` now also stores the validated
  raw token in context — purely additive.
- `internal/portal/tokens/handlers.go`: `Handler.Logout(ctx, req)` reads the
  account and raw bearer from context, then calls
  `h.svc.Revoke(ctx, acct.ID, rawToken, true)` (logout-everywhere). All
  failures map to 401 to avoid leaking internals (`ErrForbidden` is
  unreachable in practice — same account on both sides).
- `cmd/portal/main.go`: `r.Post("/auth/logout", apiWrapper.Logout)` mounted
  inside the BearerMiddleware group alongside `/auth/revoke`.
- `internal/portal/tokens/handlers_test.go`:
  - `newTestEnv` wires `/api/auth/logout` into the test router under the
    Bearer-protected group.
  - Four new tests:
    - `TestHandler_Logout_RevokesAllTokens`
    - `TestHandler_Logout_NoBearerReturns401`
    - `TestHandler_Logout_InvalidBearerReturns401`
    - `TestHandler_Logout_IsIdempotent_AlreadyRevoked`
- `tokensOnlyHandler` embeds `*tokens.Handler` so it gains the new `Logout`
  method automatically; no shim update needed.

Verified: `go test ./internal/portal/tokens/... -count 1` passes, `go build ./...` clean.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Additive context-key pattern (`rawBearerCtxKey`) keeps existing `AccountFromContext` consumers unaffected. 401-on-any-error mapping is correct (`ErrForbidden` cannot occur — caller and owner are the same account). Four tests cover the happy path, no-bearer, invalid-bearer, and idempotent-after-revoke. OpenAPI spec / codegen / route wiring all aligned.
