---
id: gate-tests-magic-link-playground-domain-collision
kind: story
stage: implementing
tags: [testing, portal, security, auth]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Magic-link `@playground.local` collision — no test for documented synthetic-email race

## Priority
High

## Spec reference
Item: `feature-epic-ephemeral-playground-anon-bearer`

Acceptance criterion: Implementation notes call out: "@playground.local suffix not reserved in magic-link validation: ... A user could register `anon_<anything>@playground.local` via magic-link and potentially collide with a synthetic anonymous-account email. This is a real (if low-probability) issue. Parked as follow-up item."

## Gap type
missing test for adversarial-spec-silent (security)

## Suggested test
```go
func TestRequestMagicLink_ReservedPlaygroundDomain_Rejected(t *testing.T) {
    // Attempt magic-link request with email "user@playground.local".
    // EITHER: 400 with reserved-domain error (preferred — fix the bug).
    // OR: explicit test silencing with link to backlog (test-integrity rule).
    // Today: a green test would lie; this gap means there is no signal that
    // the collision foot-gun exists.
}
```
Per project test-integrity discipline: either fix the validation OR file the
failing test to document the bug.

## Test location (suggested)
`internal/portal/auth/magic_link_test.go`
