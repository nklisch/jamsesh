---
id: gate-tests-anon-bearer-cross-session-rejection
kind: story
stage: implementing
tags: [testing, portal, security, tokens]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Anonymous bearer cross-session use — no test asserts a bearer for session A is rejected on session B

## Priority
High

## Spec reference
Item: `feature-epic-ephemeral-playground-anon-bearer`

Acceptance criterion: SECURITY.md (rolled forward by Unit 7): "Token leak blast radius: a leaked anonymous bearer authenticates only the session it was issued for. No cross-session privilege; no org-scope access."

## Gap type
missing test for error case (security)

## Suggested test
```go
func TestIssueAnonymousSessionBearer_BearerRejectedOnDifferentSession(t *testing.T) {
    // 1. Create sessions A and B (both playground).
    // 2. Issue an anonymous bearer for A.
    // 3. Use it against an endpoint scoped to B (RequireSessionMember or
    //    git Basic-auth resolver).
    // 4. Assert: 401/403 with session-membership error — NOT successful auth.
}
```
Existing tests prove the bearer authenticates the right account but never
assert the negative.

## Test location (suggested)
`internal/portal/tokens/anon_bearer_test.go` or `internal/portal/handlerauth/handlerauth_test.go`
