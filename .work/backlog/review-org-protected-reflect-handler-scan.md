---
id: review-org-protected-reflect-handler-scan
kind: story
stage: drafting
tags: [testing, portal, accounts, nit]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Add `reflect`-based scan of `OrgsHandler` methods to org-protected regression trip

## Source
Spawned as a nit from review of
`gate-tests-org-protected-guard-regression-trip`.

## Context
The current regression-trip test
(`TestOrgProtectedGuard_RegressionTrip_AllMutationHandlers` in
`internal/portal/accounts/orgs_test.go`) is table-driven and requires the
dev to remember to add a new row when a new mutation handler ships. A
`reflect`-based scan of `OrgsHandler` method names (excluding read-only
verbs) would close the loop by failing the test if any handler exists
without a corresponding table row.

## Why nit, not important
The current pattern works for now and is well-documented. The
defense-in-depth invariant is correctly tested for PatchOrg today. The
risk is only at the point where a future handler is added without
extending the table — at which time the dev will see the inline comment
block explaining the extension steps.
