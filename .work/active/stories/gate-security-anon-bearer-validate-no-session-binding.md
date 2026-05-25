---
id: gate-security-anon-bearer-validate-no-session-binding
kind: story
stage: review
tags: [security, portal, tokens, defense-in-depth]
parent: feature-playground-hardening
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-24
updated: 2026-05-25
---

# Playground anonymous bearer `Validate` does not enforce the bearer's bound `session_id`, leaving cross-session safety entirely to session-member checks

## Severity
Low

## Domain
Authentication & Authorization

## Location
`internal/portal/tokens/service_impl.go:114-150`

## Evidence
```go
func (s *service) Validate(ctx context.Context, raw string) (*store.Account, error) {
    // ...
    row, err := s.store.GetOAuthTokenByHash(ctx, hashToken(raw))
    // ...
    acct, err := s.store.GetAccountByID(ctx, row.AccountID)
    // returns *store.Account; the row.SessionID binding is never compared
    // against the caller's intended session.
}
```

The migration at `internal/db/migrations/postgres/00016_anonymous_bearers.sql:5-6`
adds `session_id` to `oauth_tokens`, and the create path pins each anonymous
bearer to one session. The Validate path discards that binding — every
protected handler must call `GetSessionMember` to constrain the bearer.
`docs/SECURITY.md:258-287` documents the intended threat model (anon accounts
are never in another session's members), so the existing handlers are correct
today, but any future handler that calls `RequireAccount` without an
accompanying membership check will treat an anonymous bearer as a generic
authenticated identity.

## Remediation direction
Have `Validate` (or a thin wrapper consumed by REST/MCP paths) consult
`row.SessionID` and return a typed `ErrBearerSessionMismatch` when the
request's session context disagrees; alternatively, gate every
anon-bearer-accepting handler behind a
`RequireAnonymousSessionMember(sessionID)` helper so the binding is
structurally impossible to forget.

## Implementation notes

- Added `handlerauth.RequireAnonymousSessionMember(ctx, s, orgID, sessionID)`.
  It's a thin documenting alias for `RequireSessionMember` (same signature,
  same body). The name pins the playground-specific contract: per-session
  anonymous bearers are bound to the session_id they were issued for via the
  session-member check. The alias keeps `Validate` unchanged (which is used
  by durable sessions too) — session binding is enforced structurally at the
  consumption site.
- `GetPlaygroundSession` consolidated: previously called `RequireAccount`
  then `GetSessionMember` separately; now calls
  `RequireAnonymousSessionMember` in one step. Behaviour preserved:
  - 404 on missing session row (priority over 401/403)
  - 401 on missing bearer (auth.invalid_token)
  - 401 with `auth.not_a_member` when the bearer's account is not a member
    of the requested session (i.e. issued for a different session). The
    helper returns 403 internally; we map it to 401 with the not_a_member
    code at the handler to preserve the prior wire shape.
- `GetPlaygroundTombstone` deliberately left unauthenticated. The story
  design proposed gating it, but the tombstone endpoint is intentionally
  open — any caller with a session ID can fetch the destruction summary
  (it's been destroyed; there's nothing to leak). Documented as a deviation
  here.
- Two handlerauth tests added:
  - `TestRequireAnonymousSessionMember_BearerForDifferentSession_Rejected`
  - `TestRequireAnonymousSessionMember_BearerForCorrectSession_Allowed`

Verified: `go test ./internal/portal/playground/... ./internal/portal/handlerauth/... -count 1` passes.
