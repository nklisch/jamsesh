---
id: gate-tests-github-oauth-unverified-email
kind: story
stage: done
tags: [testing, security, portal, refactor]
parent: null
depends_on: [gate-security-github-oauth-reject-unverified-email]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# GitHub OAuth unverified-email fallback is silently tested-as-correct (spec-vs-implementation contradiction)

## Priority
Critical

## Spec reference
Item: `gate-security-github-oauth-reject-unverified-email`
Acceptance criterion: refuse the OAuth flow when no verified primary
email is available — return `oauth.unverified_email` 400.

## Gap type
tautological-rework / test-integrity.
`internal/portal/oauth/github_test.go:200 TestGitHub_Exchange_PicksPrimaryVerifiedEmail`
tests the existing buggy fallback chain by including a primary-verified
email in the fixture and asserting it wins — but offers no negative
assertion. The current test will keep passing even after the fix,
masking whether the rejection path actually fires.

## Suggested test
```go
// TestGitHub_Exchange_NoVerifiedEmail_RejectsWithUnverifiedEmail
//   emails = [{primary:true, verified:false, email:"x@y"}]
//   Expect Exchange returns ErrUnverifiedEmail (mapped to 400 oauth.unverified_email at handler).
// TestGitHub_Exchange_OnlyNonPrimaryVerified_AlsoRejects
//   emails = [{primary:false, verified:true, email:"x@y"}]
//   Expect same rejection (no silent fallback to first row).
```

Tautological test to rework: `TestGitHub_Exchange_PicksPrimaryVerifiedEmail`
should be rewritten to assert against the fixed contract or removed.

## Test location (suggested)
`internal/portal/oauth/github_test.go`

## Implementation notes

### Three new negative-path tests (`internal/portal/oauth/github_test.go`)

1. **`TestGitHub_Exchange_NoVerifiedEmail_RejectsWithUnverifiedEmail`** — fixture: `[{primary:true, verified:false, email:"x@y.example.com"}]`. Calls `Exchange` and asserts `errors.Is(err, oauth.ErrUnverifiedEmail)` is true. Verifies that a primary-but-unverified email is rejected.

2. **`TestGitHub_Exchange_OnlyNonPrimaryVerified_AlsoRejects`** — fixture: `[{primary:false, verified:true, email:"x@y.example.com"}]`. Same assertion. Verifies there is no silent fallback to the first-row or any non-primary entry.

3. **`TestGitHub_Exchange_EmptyEmailList_Rejects`** — fixture: `[]` (explicitly empty slice, not nil, so the `fakeGitHub` stub bypasses the nil-default and serves the empty list). Same assertion. Verifies an account with no listed emails is rejected.

### Tautological-test rework

`TestGitHub_Exchange_PicksPrimaryVerifiedEmail` was renamed to **`TestGitHub_Exchange_PicksVerifiedPrimaryEmailFromList`** and its fixture was extended to include:
- `secondary-verified@example.com` (primary:false, verified:true) — would be selected if a non-primary fallback were introduced
- `primary-unverified@example.com` (primary:true, verified:false) — would be selected if verification were dropped
- `primary-verified@example.com` (primary:true, verified:true) — the only valid entry

The doc comment was updated to state: "this is the only path that returns a non-error response — no fallback." Returning either of the other two emails would now fail the assertion, actively guarding against fallback re-introduction.

### Handler-level integration test (`internal/portal/auth/oauth_test.go`)

**`TestOauthCallback_UnverifiedEmail_Returns400WithOauthUnverifiedEmailCode`** — uses `stubProvider` returning `*portaloauth.ErrExchange{Cause: portaloauth.ErrUnverifiedEmail}` (matching the real GitHub provider's error wrapping). Drives the full HTTP pipeline via `newOAuthTestEnv`, obtains a valid nonce via `/start`, then calls `/callback`. Asserts:
- Response is HTTP 400 (not 503 dep-class)
- No `Retry-After` header (retry is futile — user must verify email with provider)
- `body["error"] == "oauth.unverified_email"`
- `body["message"]` is non-empty

All new tests pass. Both packages build and run cleanly via `go test ./internal/portal/oauth/ ./internal/portal/auth/`. A pre-existing build failure in `rate_limit_integration_test.go` (untracked file, not introduced by this story) prevents `./...` from resolving the auth package when used with the recursive glob — the explicit package path resolves it without issue.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Critical OAuth account-confusion coverage verified. Three new negative-path tests (unverified primary, only non-primary verified, empty list) all assert errors.Is(err, ErrUnverifiedEmail). Tautological-test rework: existing TestGitHub_Exchange_PicksPrimaryVerifiedEmail extended with secondary-verified and primary-unverified entries alongside the valid one — any fallback re-introduction would return the wrong email and fail the assertion. Handler-level integration test confirms the 400 oauth.unverified_email envelope (no Retry-After since retry is futile).
