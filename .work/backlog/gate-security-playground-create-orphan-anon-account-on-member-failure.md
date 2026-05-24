---
id: gate-security-playground-create-orphan-anon-account-on-member-failure
kind: story
stage: drafting
tags: [security, portal, playground, data-protection]
parent: null
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-24
updated: 2026-05-24
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
