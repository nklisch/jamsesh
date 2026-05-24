---
id: gate-tests-event-discriminator-triad-completeness
kind: story
stage: drafting
tags: [testing, portal, spec]
parent: null
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Event discriminator triad (enum / mapping / oneOf) not fully cross-checked

## Priority
Low

## Spec reference
Item: `story-spec-discipline-drift-ci-check`

Acceptance criterion: Pattern doc: "events.AllTypes must mirror the EventEnvelope.type enum exactly." Test enforces enum↔AllTypes equality but not the `oneOf` payload schemas + discriminator mapping consistency.

## Gap type
complementary coverage

## Suggested test
Extend `TestEventTypeConstants_MatchOpenAPIYAML` to also assert every enum
value has a `discriminator.mapping` entry and a matching `oneOf` schema.

## Test location (suggested)
`internal/portal/events/spec_drift_test.go`
