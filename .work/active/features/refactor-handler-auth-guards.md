---
id: refactor-handler-auth-guards
kind: feature
stage: drafting
tags: [refactor, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Refactor — Handler Auth-Guards

## Why

The portal HTTP handlers repeat three identical auth/authorization checks
across at least seven sites. Each handler hand-rolls:

1. **Account extraction + 401** — `tokens.AccountFromContext(ctx)` → if missing,
   return `<Op>401JSONResponse{ UnauthorizedJSONResponse{ Error: "auth.invalid_token", Message: "invalid token" } }`. Roughly 8 lines per site.
2. **Org-membership + 403** — `store.GetOrgMember(...)` → on `ErrNotFound` return
   `<Op>403JSONResponse{ ForbiddenJSONResponse{ Error: "auth.insufficient_permission", Message: "not a member of this org" } }`. Roughly 12 lines per site.
3. **Session-membership + 403** — `store.GetSessionMember(...)` with the same
   pattern. Roughly 12 lines per site.

Concrete sites observed:

- `internal/portal/sessions/handler.go:50-77` (CreateSession), `:152-178` (PatchSession), `:244+` (FinalizeSession), `:317+` (AbandonSession)
- `internal/portal/comments/handlers.go:30-66`, `:146-173`, `:257-285`
- `internal/portal/accounts/handlers.go:38-`, `:82-89`
- `internal/portal/tokens/handlers.go:53-62`

Today the boilerplate also **leaks store internals into handler packages** —
every handler that gates on org membership imports `store` and calls
`store.GetOrgMember` directly, instead of going through a portal-level authz
helper.

## Constraint that shapes the design

`oapi-codegen` generates a **per-operation response type**
(`CreateSession401JSONResponse`, `PatchSession401JSONResponse`, …) so a single
helper cannot return a typed envelope that fits every caller. The helper must
return the inner `UnauthorizedJSONResponse` / `ForbiddenJSONResponse` payload
plus a sentinel (or `ok bool`), and each caller wraps that inner payload in its
own operation-specific outer type with one line.

## Target shape

New package `internal/portal/handlerauth` (or extend `internal/portal/tokens`):

```go
// RequireAccount returns the authenticated account or a typed 401 payload.
// Callers wrap the payload in their operation-specific 401 response.
func RequireAccount(ctx context.Context) (store.Account, openapi.UnauthorizedJSONResponse, bool)

// RequireOrgMember returns the org-member row or a typed 401/403 payload.
// First-class flow: extracts account, checks org membership, returns the row.
func RequireOrgMember(ctx context.Context, s store.Store, orgID string) (store.Account, store.OrgMember, AuthFail, bool)

// RequireSessionMember composes RequireOrgMember + session-membership check.
func RequireSessionMember(ctx context.Context, s store.Store, orgID, sessionID string) (store.Account, store.SessionMember, AuthFail, bool)

// AuthFail carries the typed payload to wrap and an HTTP status hint.
type AuthFail struct {
    Status int                              // 401 or 403
    Unauthorized openapi.UnauthorizedJSONResponse
    Forbidden    openapi.ForbiddenJSONResponse
}
```

Call sites collapse from ~20 lines to ~3:

```go
acc, member, fail, ok := handlerauth.RequireSessionMember(ctx, h.store, req.OrgID, req.SessionID)
if !ok {
    if fail.Status == 401 {
        return openapi.PatchSession401JSONResponse{UnauthorizedJSONResponse: fail.Unauthorized}, nil
    }
    return openapi.PatchSession403JSONResponse{ForbiddenJSONResponse: fail.Forbidden}, nil
}
```

(Further collapsible with a small `wrapFail[T any]` constructor per package if
desired, but the above is the minimum.)

## Acceptance criteria for the feature

- [ ] All four sessions handlers, all three comments handlers, and the accounts
      + tokens handlers use the new helpers
- [ ] No handler imports `store.GetOrgMember` / `store.GetSessionMember` directly
- [ ] All existing handler tests pass unchanged (behavior-preserving)
- [ ] At least one unit test per helper covering: missing token, valid token,
      missing org membership, missing session membership

## Risk

LOW — purely a code reshuffling. Strong existing test coverage at the handler
level catches any regression in response shape (error codes/messages).

## Implementation order

1. `refactor-handler-auth-guards-helpers-and-sessions` — define the package,
   migrate `internal/portal/sessions/handler.go`
2. `refactor-handler-auth-guards-comments` — migrate `internal/portal/comments/handlers.go`
3. `refactor-handler-auth-guards-accounts-tokens` — migrate
   `internal/portal/accounts/handlers.go` and `internal/portal/tokens/handlers.go`
