---
id: gate-tests-event-discriminator-triad-completeness
kind: story
stage: implementing
tags: [testing, portal, spec]
parent: feature-test-spec-drift-and-coverage
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-24
updated: 2026-05-25
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

## Implementation

Add `TestEventDiscriminatorTriad_Completeness` to
`internal/portal/events/spec_drift_test.go`.

The test loads the YAML using the existing `runtime.Caller(0)`-based path
(already established in the file — no cwd dependency). It then extracts:

1. `enumTypes` — from `EventEnvelope.properties.type.enum` (existing helper
   `extractEventEnvelopeTypeEnum` covers this)
2. `mappingKeys` — from `EventEnvelope.discriminator.mapping` (keys of the
   mapping object)
3. `mappingValues` — from `EventEnvelope.discriminator.mapping` (values,
   e.g. `#/components/schemas/CommitArrivedPayload`)
4. `oneOfRefs` — from `EventEnvelope.payload.oneOf[*].$ref`

Assertions:
- `sort(enumTypes) == sort(mappingKeys)` — every enum type has a mapping entry
- `sort(mappingValues) == sort(oneOfRefs)` — every mapping target appears in
  oneOf (and vice versa)

Reuse the existing `sortedCopy` and `difference` helpers from the same file.
Emit a structured diagnostic on failure (same style as the existing test).
