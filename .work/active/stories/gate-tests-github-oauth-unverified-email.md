---
id: gate-tests-github-oauth-unverified-email
kind: story
stage: implementing
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
