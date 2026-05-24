---
id: gate-tests-org-protected-guard-regression-trip
kind: story
stage: drafting
tags: [testing, portal, security, defense-in-depth]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Org-protected guard regression-trip test missing for future `OrgsHandler` methods

## Priority
Medium

## Spec reference
Item: `story-extend-org-protected-guard-to-policy-mutations`

Acceptance criterion: Story scope: "Add an `OrgProtected` check to `PatchOrg` (and any future `DeleteOrg` / rename handler)."

## Gap type
missing test for adversarial-spec-silent

## Suggested test
A "regression-trip" test that fails the build if any future `OrgsHandler`
method exists without an `OrgProtected` check — implementable via a small
reflection scan in the test file. Otherwise the next-added handler will
silently bypass the guard.

## Test location (suggested)
`internal/portal/accounts/orgs_test.go`
