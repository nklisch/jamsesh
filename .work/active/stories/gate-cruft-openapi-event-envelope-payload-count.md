---
id: gate-cruft-openapi-event-envelope-payload-count
kind: story
stage: implementing
tags: [cleanup, documentation]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: cruft
created: 2026-05-31
updated: 2026-05-31
---

# OpenAPI event-envelope comment has stale payload count

## Confidence
Medium

## Category
stale comment

## Location
`docs/openapi.yaml:132`

## Evidence
```yaml
# Every event pushed over the portal WebSocket shares the EventEnvelope
# wrapper. The `type` field is the discriminator; the `payload` field is a
# oneOf over the 12 payload schemas below.
```

The adjacent enum and `payload.oneOf` list 15 event payloads.

## Removal
Replace the fixed count with the current count, or remove the count so the
comment cannot drift again.


## Autopilot scope/design (2026-05-31)

Promoted by the gate-drain autopilot run. Implement the remediation direction above as a focused single-stride story, keep edits limited to the named surface, and verify with the targeted test or check that covers the changed file. For older backlog gate items, this run binds the work to `v0.5.0` because the user explicitly requested all gate-related work be scoped, designed, and implemented before release.
