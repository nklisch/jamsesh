---
id: gate-tests-projectlanding-hardcoded-version-string
kind: story
stage: drafting
tags: [testing, ui, cleanup]
parent: feature-spa-bootstrap-hygiene
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-25
updated: 2026-05-25
---

# ProjectLanding colophon hard-codes `v0.4.0`

## Priority
Low

## Spec reference
Item: `story-portal-visitor-entry-pages-spa-landing`
Test `ProjectLanding.test.ts:97` asserts the literal `v0.4.0` string.

## Gap type
Drift pressure — every release bump rots this test.

## Location
`frontend/src/lib/screens/ProjectLanding.svelte:118` —
`jamsesh / Apache-2.0 / v0.4.0 / 2026`. Test at
`ProjectLanding.test.ts:97` matches the literal. After bumping the
release this assertion goes stale.

## Remediation direction
Make the version a build-time constant (Vite `define`) and assert "the
version string is present and matches a semver-ish pattern" rather than
the literal. Or fold a ProjectLanding version-bump into `release-bump.sh`.
