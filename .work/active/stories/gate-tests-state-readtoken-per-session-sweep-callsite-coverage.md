---
id: gate-tests-state-readtoken-per-session-sweep-callsite-coverage
kind: story
stage: drafting
tags: [testing, plugin, refactor]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# `feature-state-readtoken-per-session-sweep` call-site coverage uncertain

## Priority
Medium

## Spec reference
Item: `feature-state-readtoken-per-session-sweep`

Acceptance criterion: Inferred from feature naming: extract a helper + sweep call-sites. Without confirmation, the existing tests in `cmd/jamsesh/state/{state,migrate}_test.go` cover migration but call-site coverage of the new helper is unverified.

## Gap type
missing test for valid partition (worth confirming the helper + each call-site has a test)

## Suggested test
For each call-site (status, fork, etc.), verify the migrated `ReadToken`
helper is invoked.

## Test location (suggested)
`cmd/jamsesh/state/`, `cmd/jamsesh/sessioncmd/`
