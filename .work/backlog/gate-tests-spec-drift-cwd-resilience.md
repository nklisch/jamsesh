---
id: gate-tests-spec-drift-cwd-resilience
kind: story
stage: drafting
tags: [testing, portal, infra]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# `TestEventTypeConstants_MatchOpenAPIYAML` path-resolution under non-default cwd not asserted

## Priority
Low

## Spec reference
Item: `story-spec-discipline-drift-ci-check`

Acceptance criterion: Story note: "Path lookup: use a `runtime.Caller`-based or relative-path discovery so the test still works when run from a different cwd."

## Gap type
adversarial-spec-silent (resilience)

## Suggested test
Add a sub-test that invokes the comparison from a temp cwd via
`t.Chdir(t.TempDir())` to confirm path discovery is robust.

## Test location (suggested)
`internal/portal/events/spec_drift_test.go`
