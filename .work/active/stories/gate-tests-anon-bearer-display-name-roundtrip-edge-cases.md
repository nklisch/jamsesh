---
id: gate-tests-anon-bearer-display-name-roundtrip-edge-cases
kind: story
stage: drafting
tags: [testing, portal, tokens]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# `IssueAnonymousSessionBearer` display-name round-trip edge cases not covered

## Priority
Medium

## Spec reference
Item: `feature-epic-ephemeral-playground-anon-bearer`

Acceptance criterion: Unit 4 AC: "After issuance, `Validate(ctx, rawToken)` returns the new `*store.Account` with `IsAnonymous: true, DisplayName: 'amber-otter'`."

## Gap type
partial coverage — round-trip exists but not for collision-fallback handles (`quiet-otter-x1`, `swift-heron-a3f2`)

## Suggested test
Table-driven over `[]string{"amber-otter", "quiet-fox", "swift-heron-a3f2"}`
confirming round-trip preserves the suffix.

## Test location (suggested)
`internal/portal/tokens/anon_bearer_test.go`
