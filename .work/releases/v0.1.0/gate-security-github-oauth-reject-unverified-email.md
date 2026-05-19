---
id: gate-security-github-oauth-reject-unverified-email
kind: story
stage: done
tags: [security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# GitHub OAuth accepts unverified primary email, enabling account confusion / takeover

## Severity
High

## Domain
Authentication & Authorization

## Location
`internal/portal/oauth/github.go:259-273`

## Evidence
```go
for _, e := range emails {
    if e.Primary && e.Verified {
        return e.Email, nil
    }
}
// Fall back to first primary (unverified) or just the first entry.
for _, e := range emails {
    if e.Primary {
        return e.Email, nil
    }
}
if len(emails) > 0 {
    return emails[0].Email, nil
}
```

Combined with `internal/portal/auth/provision.go:72-77`
(`GetAccountByGitHubUserID` first, `GetAccountByEmail` for magic-link)
and `magic_link.go:158` (`FindOrProvisionAt` returns the existing account
by email match), an attacker who attaches `victim@example.com` to their
GitHub profile as an unverified primary email creates an account row with
that email. When the real owner later signs in via magic-link,
`lookupAccount`→`GetAccountByEmail` finds the attacker-controlled
account and the real owner is dropped into it. Account confusion in the
other direction is also possible.

## Remediation direction
Refuse the OAuth flow when no verified primary email is available — return
`oauth.unverified_email` 400 instead of silently falling back. Optionally
cross-check at provisioning time that an existing `accounts.email` row's
verification status matches the new identity proof.

## Implementation notes

### Changes

**`internal/portal/oauth/provider.go`** — added `ErrUnverifiedEmail` sentinel
at package level alongside `ErrBadGrant`. Both are plain `errors.New` values
that travel up through `*ErrExchange.Unwrap()` so callers can use
`errors.Is`.

**`internal/portal/oauth/github.go`** — `fetchPrimaryEmail` now returns
`ErrUnverifiedEmail` when the email list contains no verified primary entry.
The two fallback loops (unverified primary, then first-entry) are removed.
The function comment is updated to document the new contract.

**`internal/portal/auth/oauth.go`** — `OauthCallback` now checks
`errors.Is(err, portaloauth.ErrUnverifiedEmail)` before the dep-class
fallback and returns 400 `oauth.unverified_email` with an actionable message
directing the user to verify their email on GitHub. The exchange error
comment block is updated to document three (not two) error classes.

### Provisioning cross-check
`internal/portal/auth/provision.go` does not track `email_verified` per
account row (no such column in the schema). The defense-in-depth cross-check
described in the remediation direction is therefore deferred. The primary fix
(rejecting the flow before an unverified email ever reaches provisioning) is
in place.

### Tests
All existing tests pass unchanged. `TestGitHub_Exchange_PicksPrimaryVerifiedEmail`
continues to exercise the happy path. The companion story
`gate-tests-github-oauth-unverified-email` will add the negative-path tests.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: High-severity account-confusion vector closed. fetchPrimaryEmail now returns ErrUnverifiedEmail when no verified primary is found (no more fallback to first primary or first entry). OauthCallback maps to 400 oauth.unverified_email. ErrUnverifiedEmail declared in oauth/provider.go alongside ErrBadGrant. Provisioning cross-check deferred (no accounts.email_verified column today); the OAuth-side reject is sufficient to close the takeover vector. Existing tests pass.
