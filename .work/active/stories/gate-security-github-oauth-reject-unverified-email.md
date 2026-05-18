---
id: gate-security-github-oauth-reject-unverified-email
kind: story
stage: implementing
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
