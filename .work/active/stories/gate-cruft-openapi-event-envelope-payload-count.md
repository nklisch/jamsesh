---
id: gate-cruft-openapi-event-envelope-payload-count
kind: story
stage: drafting
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

