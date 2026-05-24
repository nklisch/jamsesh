---
id: story-playground-foundation-docs-rollup-protocol-destruction-warning
kind: story
stage: implementing
tags: [documentation, playground, protocol]
parent: feature-playground-foundation-docs-rollup
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Add `playground.destruction_warning` to PROTOCOL.md event-type taxonomy

## Origin

Filed by review of
`story-epic-ephemeral-playground-plugin-skills-destruction-warning`.

## Problem

`docs/PROTOCOL.md` § "WebSocket event types" (around lines 370-382)
enumerates every event type the system emits. The
`session-lifecycle` and `plugin-skills` features added a new event
type — `playground.destruction_warning` — and registered it in
`docs/openapi.yaml` (EventEnvelopeType enum, payload schema, digest
`urgent_events` field). The OpenAPI schema is the canonical
machine-readable contract, but PROTOCOL.md is the human-readable
taxonomy and is currently stale on this addition.

Per the rolling-foundation principle (CLAUDE.md, agile-workflow
rules), foundation docs must describe the system as it is NOW. The
feature-epic-ephemeral-playground-plugin-skills feature body
explicitly called this out:

> `docs/PROTOCOL.md` — addressing convention for anonymous handles
> and the "destruction-warning event" digest extension are rolled
> into PROTOCOL.md by this feature's design pass

The OpenAPI rollup happened; the PROTOCOL.md rollup did not.

## Scope

Append the new event to PROTOCOL.md:

1. In § "WebSocket event types" event-list, add:
   ```
   - `playground.destruction_warning` — payload: `{reason: "idle_timeout" | "hard_cap", ends_at, remaining_seconds, session_id}`
   ```
2. In the digest section (around line 178 / `pre_turn_digest`), note
   that the digest may surface an `urgent_events` field for
   time-sensitive events that the binary renders in a prominent
   "Urgent" section above regular digest text.
3. Optionally: cross-link to the openapi.yaml schemas for full payload
   field definitions.

Also worth considering: a brief mention in the addressing-convention
section that anonymous handles (`amber-otter`) participate in
`@<handle>` mentions identically to durable handles — that was the
*other* PROTOCOL.md update the feature design promised.

## Acceptance criteria

- [ ] PROTOCOL.md § "WebSocket event types" includes
      `playground.destruction_warning` with its payload shape
- [ ] PROTOCOL.md digest section references the `urgent_events` field
- [ ] (Optional but desirable) Addressing convention note for
      anonymous handles is present in the addressing section
- [ ] No drift between PROTOCOL.md and `docs/openapi.yaml` for this
      event

## Notes

This is documentation-only. No code change. Small enough to drain in
one stride.
