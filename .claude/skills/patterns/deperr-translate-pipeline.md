# Pattern: Three-Tier Error Translation Pipeline

Failures travel through three layers: (1) raw call sites wrap dependency
failures with `deperr.Wrap{SMTP,DB,DBIfTransient,OAuthProvider,GitSubprocess}`
sentinels, (2) the strict-server's `ResponseErrorHandlerFunc` is
`httperr.WriteFromError`, which uses `errors.Is` to classify, (3)
`httperr.Err*Unavailable` constructors build the canonical envelope with
`Retry-After` headers. Handler code never reaches for
`http.ResponseWriter.WriteHeader` directly.

## Rationale

Single source of truth for the JSON error envelope
(`{"error", "message", "details"}` per `docs/PROTOCOL.md`). The `deperr`
sentinels are the seam between business logic (which knows _what
failed_) and the wire format (which decides _how to express it_).
`WrapDBIfTransient` is the key nuance: it returns business sentinels
(`store.ErrNotFound`, `store.ErrUniqueViolation`) unchanged so handler
code can still `errors.Is` them, while wrapping anything else as
`deperr.ErrDB`.

## Examples

### Example 1: handler wraps at call site, translator classifies

**File**: `internal/portal/accounts/orgs.go:101`

```go
if err := tx.UpdateOrgSessionInvitePolicy(ctx, ...); err != nil {
    return nil, deperr.WrapDBIfTransient(fmt.Errorf("accounts: update org session invite policy: %w", err))
}
```

### Example 2: translator uses `errors.Is` against sentinels

**File**: `internal/portal/httperr/translate.go:24`

```go
func WriteFromError(w http.ResponseWriter, r *http.Request, err error) {
    var e *Error
    if errors.As(err, &e) {
        Write(w, r, e); return
    }
    switch {
    case errors.Is(err, deperr.ErrSMTP):           Write(w, r, ErrSMTPUnavailable(err))
    case errors.Is(err, deperr.ErrDB):             Write(w, r, ErrDBUnavailable(err))
    case errors.Is(err, deperr.ErrOAuthProvider):  Write(w, r, ErrOAuthProviderUnavailable(err))
    case errors.Is(err, deperr.ErrGitSubprocess):  Write(w, r, ErrGitSubprocessFailed(err))
    default:                                       Write(w, r, ErrInternal(err))
    }
}
```

### Example 3: OAuth provider wrap

**File**: `internal/portal/auth/oauth.go:146`

```go
return nil, deperr.WrapOAuthProvider(...)
```

107 `deperr.Wrap*` call sites across non-test Go code.

## When to Use

- Any handler-returned error that is _not_ a recognized business
  sentinel — wrap it so the translator emits the typed envelope with
  the right `Retry-After`.
- New external-dependency classes — add a new sentinel and
  `Err*Unavailable` constructor; do not invent ad-hoc error codes
  inline.

## When NOT to Use

- Building a typed `*httperr.Error` directly (e.g.
  `httperr.ErrSessionNotFound()`) — those pass through
  `WriteFromError` unchanged via `errors.As`.
- 4xx business errors expressed through the operation's response union
  (e.g. `409 comment.already_resolved`) — those return successfully,
  not as `err`.

## Common Violations

- Calling `deperr.WrapDB` (unconditional) at a site where
  `store.ErrNotFound` may also surface — masks the business sentinel
  and breaks `errors.Is(err, store.ErrNotFound)` checks upstream. Use
  `WrapDBIfTransient`.
- Writing JSON to `http.ResponseWriter` directly from a handler —
  bypasses the envelope and breaks the PROTOCOL.md contract.
- Inventing a new `dep.*` error code by writing the envelope literal —
  should be a constructor in `httperr/httperr.go`.
