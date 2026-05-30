# Pattern: AuthFail Three-Branch Handler Guard

Strict-server handlers call
`handlerauth.Require{Account,OrgMember,SessionMember}(...)`, then branch
three ways on the returned `AuthFail`: a 500 path that returns the error
to the strict-server, a typed `*Op*Fail(fail)` wrapper that maps to the
per-operation 401/403 response object, and the happy path. The mapper
is a tiny per-operation function adjacent to the handler.

## Rationale

oapi-codegen's strict-server generates a different response type per
operation (e.g. `openapi.CreateComment401JSONResponse`,
`openapi.GetOrg401JSONResponse`), so 401/403 envelopes cannot be
returned through a single shared helper. `handlerauth.AuthFail`
centralises the auth lookup + envelope construction, and the local
`*Fail(f handlerauth.AuthFail)` mapper converts the union shape into
the operation's required response type. The 500 branch routes through
`deperr.WrapDBIfTransient` so unexpected store errors surface as the
canonical `dep.db_unavailable` envelope.

## Examples

### Example 1: comments — session-member gate with three branches

**File**: `internal/portal/comments/handlers.go:35`

```go
acc, _, fail, ok := handlerauth.RequireSessionMember(ctx, h.s, orgID, sessionID)
if !ok {
    if fail.Err != nil {
        return nil, deperr.WrapDBIfTransient(fmt.Errorf("comments: create: %w", fail.Err))
    }
    return createCommentFail(fail), nil
}
```

With the matching mapper at `internal/portal/comments/handlers.go:248`:

```go
func createCommentFail(f handlerauth.AuthFail) openapi.CreateCommentResponseObject {
    if f.Status == 401 {
        return openapi.CreateComment401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
    }
    return openapi.CreateComment403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}
```

### Example 2: sessions — same shape across 4 operations

**File**: `internal/portal/sessions/handler.go:451-472` defines
`createSessionFail`, `patchSessionFail`, `finalizeSessionFail`,
`abandonSessionFail` — all with the identical 401/403 branch shape.

### Example 3: accounts/orgs — same shape across 6 operations

**File**: `internal/portal/accounts/orgs.go:55,124,336,345` and
`internal/portal/accounts/handlers.go:134,138`.

41 total `*Fail` mapper functions across the bundle.

## When to Use

- A new strict-server handler that requires authentication and an org
  or session membership check.
- The auth check fronts the handler — no business logic should precede
  it.

## When NOT to Use

- Internal handlers that aren't expressed through oapi-codegen
  strict-server (e.g. `internal/portal/githttp` smart-HTTP routes —
  they write responses directly).
- Operations where the operation-specific failure shape genuinely
  diverges (e.g. some auth flows return 401 with provider-specific
  envelope details).

## Common Violations

- Hand-rolling the 401/403 envelope inline instead of going through
  `handlerauth.Require*` (drift in the error code/message text).
- Forgetting the 500-with-err branch and returning a bare 500 with no
  `deperr` wrap — produces a generic `"internal"` envelope instead of
  `dep.db_unavailable`.
- Returning `fail.Forbidden` directly without the operation-specific
  outer wrapper — won't satisfy the strict-server response interface.
