---
id: gate-tests-org-protected-guard-regression-trip
kind: story
stage: review
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

## Implementation notes

Added `TestOrgProtectedGuard_RegressionTrip_AllMutationHandlers` to
`internal/portal/accounts/orgs_test.go`.

The test uses a `protectedMutationGuardStore` wrapper that overrides every store
mutation method that an org-mutation handler could call (currently
`UpdateOrgSessionInvitePolicy`). If the `OrgProtected` guard fires correctly,
none of these methods should be reached. If a future handler bypasses the guard,
the store wrapper records the bypassed method name and the test fails with a
clear diagnostic.

The test is table-driven over `(method, path, body)` tuples, one per mutation
handler. Adding a future `DeleteOrg` handler requires:
1. Wiring its route in the local chi router.
2. Adding a `protectedMutationGuardStore` override for any new store mutation.
3. Appending a table row for the new method.

The existing `TestPatchOrg_ProtectedOrg_Returns409` test continues to cover the
happy-path 409 response for `PatchOrg`; this regression-trip test adds the
store-bypass sentinel and establishes the extensible table pattern.
