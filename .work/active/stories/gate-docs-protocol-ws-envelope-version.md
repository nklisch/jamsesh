---
id: gate-docs-protocol-ws-envelope-version
kind: story
stage: implementing
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: docs
created: 2026-05-18
updated: 2026-05-18
---

# PROTOCOL.md WebSocket envelope sample omits the `version` field that the schema requires

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/PROTOCOL.md:319-330`
- Code: `docs/openapi.yaml:143-170` (`EventEnvelope` has
  `required: [seq, version, type, payload, timestamp, session_id]` with
  `version: enum: [1]`)

## Current doc text
> ```
> {
>   "seq": <int>,
>   "session_id": "<session-id>",
>   "type": "<event-type>",
>   "payload": { ... },
>   "timestamp": "<iso-8601>"
> }
> ```

## Reality
The live `EventEnvelope` schema requires a `version: 1` field on every
envelope.

## Required edit
Add `"version": 1` to the envelope sample and a short note that the
field is the envelope-schema version (currently always `1`).
