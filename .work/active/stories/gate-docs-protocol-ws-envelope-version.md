---
id: gate-docs-protocol-ws-envelope-version
kind: story
stage: review
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

## Implementation notes

- Added `"version": 1` to the WebSocket envelope JSON sample in `docs/PROTOCOL.md` (lines 340-348), placed adjacent to `seq` as the envelope-level metadata fields group naturally together.
- Added a one-sentence prose note after the sample explaining that `version` is the envelope-schema version (currently always `1`) and that it allows future envelope format changes without breaking existing clients.
- Schema cross-checked against `docs/openapi.yaml:143-170`: `EventEnvelope` has `required: [seq, version, type, payload, timestamp, session_id]` with `version: enum: [1]` — the sample now matches.
