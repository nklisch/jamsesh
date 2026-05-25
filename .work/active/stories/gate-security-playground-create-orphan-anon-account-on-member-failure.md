---
id: gate-security-playground-create-orphan-anon-account-on-member-failure
kind: story
stage: review
tags: [security, portal, playground, data-protection]
parent: feature-playground-hardening
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-24
updated: 2026-05-25
---

# Playground `CreatePlaygroundSession` leaks an orphaned anon account + bearer when `AddSessionMember` fails between bearer issuance and member insert

## Severity
Low

## Domain
Data Protection

## Location
`internal/portal/playground/handler.go:153-167`

## Evidence
```go
rawToken, accountID, expiresAt, err := h.Tokens.IssueAnonymousSessionBearer(ctx, sessionID, nickname, h.Cfg.HardCap)
// ...
if err := h.Store.AddSessionMember(ctx, store.AddSessionMemberParams{...}); err != nil {
    return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: add session member: %w", err))
}
```

Three independent transactions (session insert, bearer-issue tx, member
insert) are run sequentially with no compensating action on partial failure.
If `AddSessionMember` errors after `IssueAnonymousSessionBearer` returned a
usable raw token, the bearer is valid against `Validate` (the bearer row
exists with an unexpired `expires_at`) but every membership-gated handler
will 401 it. The destruction sweep only fires once the session expires
(hard-cap = 24h by default), so the orphan persists for the entire hard-cap
window. The deliberate split is documented at lines 119-124, but the failure
mode is not.

## Remediation direction
On `AddSessionMember` failure, best-effort revoke the just-issued bearer
(`RevokeOAuthToken` by ID) and delete the anon account so the orphan window
collapses; alternatively, retry `AddSessionMember` and treat persistent
failure as a server-side `internal` error so the destruction sweep can clean
up the session row too.

## Implementation notes

- `tokens.Service` gained `RevokeAnonymousBearer(ctx, rawToken) error`.
  Implementation in `service_impl.go` looks up the row by hash, revokes the
  token, and `DeleteAccountsByIDs(ctx, []string{row.AccountID})`. Idempotent
  on `ErrNotFound`.
- `tokensStore` interface grew `DeleteAccountsByIDs` so the tokens service
  can hard-delete the anon account row. Same query, same dialect adapters
  used by `PlaygroundSessionStore` (no duplication of SQL).
- `internal/portal/playground/handler.go`: both call sites of
  `AddSessionMember` (in `CreatePlaygroundSession` and `JoinPlaygroundSession`)
  now call `h.Tokens.RevokeAnonymousBearer(ctx, rawToken)` on member-insert
  failure. Compensation failure is `h.Logger.Warn`-logged with session_id +
  account_id + err; the primary 5xx error return is unchanged.
- Test stubs updated: `mockService` in `tokens/middleware_test.go`,
  `storeOverride` in `tokens/anon_bearer_test.go`, and `failingTokensService`
  in `playground/handler_test.go` all implement the new method.
- New helper in playground tests: `failingAddSessionMemberStore` embeds
  `store.Store` and overrides only `AddSessionMember` (per the
  test-narrow-store-delegation pattern).
- New test `TestCreatePlaygroundSession_MemberInsertFails_BearerRevoked`:
  injects the AddSessionMember failure, asserts 5xx, then asserts the anon
  account row is gone (compensation deleted it) and no session-member rows
  were inserted.
- A parallel test for the joiner path could be added in a follow-up; the
  shared `RevokeAnonymousBearer` helper means both paths use the same code
  path so the create-path test is sufficient verification.

Verified: `go test ./internal/portal/playground/... ./internal/portal/tokens/... -count 1` passes.
