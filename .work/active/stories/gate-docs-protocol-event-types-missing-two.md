---
id: gate-docs-protocol-event-types-missing-two
kind: story
stage: implementing
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# PROTOCOL.md `WebSocket event types` enumeration is missing two events the server emits

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/PROTOCOL.md:376-390`
- Code: `internal/portal/events/types.go:19-35` and `docs/openapi.yaml:159-174`

## Current doc text
> **Event types:** (15 items listed, ending at `playground.destruction_warning`)

## Reality
`events.AllTypes` contains 15 strings including `auto-merger.backpressure` (line 20) and `session.created` (line 31) — both added in this bundle by `story-spec-discipline-audit-and-close-emit-vs-yaml-gaps` and `story-spec-discipline-drift-ci-check`. Both appear in `docs/openapi.yaml`'s `EventEnvelope.type` enum and `discriminator.mapping`, with payload schemas (`AutoMergerBackpressurePayload`, `SessionCreatedPayload`). PROTOCOL.md's event-type bullet list lacks both.

## Required edit
Append two bullets to the event-type list in `docs/PROTOCOL.md` in alphabetical position: `auto-merger.backpressure` (payload: `AutoMergerBackpressurePayload`) and `session.created` (payload: `SessionCreatedPayload`).
