---
id: story-playground-foundation-docs-rollup-protocol-destruction-warning
kind: story
stage: review
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

Per feature-design (`feature-playground-foundation-docs-rollup`), this
story owns three PROTOCOL.md edits plus one verification:

1. **§ "WebSocket event types"** (~lines 369–382) — add:
   ```
   - `playground.destruction_warning` — payload: `{reason: "idle_timeout" | "hard_cap", ends_at, remaining_seconds, session_id}`
   ```
   Place adjacent to the existing `session.*` lifecycle entries (it shares
   that semantic family).

2. **Cross-link openapi.yaml schemas across the event-type list** —
   alongside EVERY event-type entry in the bullet list (not just the new
   one), add a parenthetical link of the form
   `(schema: [SchemaName](./openapi.yaml#/components/schemas/SchemaName))`
   pointing at the canonical payload definition in `docs/openapi.yaml`.
   The set of schemas to map:
   - `commit.arrived` → `CommitArrivedPayload`
   - `merge.succeeded` → `MergeSucceededPayload`
   - `conflict.detected` → conflict event schema (verify name)
   - `conflict.resolved` → `ConflictResolvedPayload`
   - `comment.added` → comment schema (verify name)
   - `comment.resolved` → `CommentResolvedPayload`
   - `ref.forked` → `RefForkedPayload`
   - `mode.changed` → `ModeChangedPayload`
   - `turn.ended` → `TurnEndedPayload`
   - `presence.updated` → `PresenceUpdatedPayload`
   - `session.finalizing` → `SessionFinalizingPayload`
   - `session.ended` → `SessionEndedPayload`
   - `playground.destruction_warning` → `PlaygroundDestructionWarningPayload`

   Verify each anchor target exists by `grep -n "^    SchemaName:" docs/openapi.yaml`.
   If a schema isn't yet defined for an existing event, link to the
   nearest available section and leave a TODO comment in the PR — do NOT
   block this story on missing openapi schemas (file a follow-up).

3. **§ `pre_turn_digest`** (~line 178) — note that the digest may
   surface an `urgent_events` field for time-sensitive events that the
   binary renders in a prominent "Urgent" section above regular digest
   text. Mention `playground.destruction_warning` as the current member
   of that class.

4. **§ Addressing syntax — VERIFY-ONLY (no edit)** — lines 302–307
   already contain the anonymous-handles addressing note ("Anonymous
   session participants use the same `@<nickname>` form…", with the
   `@amber-otter` example). Re-read the section; confirm the prose
   matches the Design-decisions intent. Record the verification in the
   PR description so the reviewer doesn't flag the missing edit as a
   gap. If the section has somehow regressed since 2026-05-23, restore
   it; otherwise leave it untouched.

## Acceptance criteria

- [ ] PROTOCOL.md § "WebSocket event types" includes
      `playground.destruction_warning` with its payload shape
- [ ] Every event-type bullet in the list carries a `(schema: ...)`
      cross-link to its openapi.yaml schema anchor (or a TODO marker if
      the schema is missing — with a follow-up filed)
- [ ] PROTOCOL.md `pre_turn_digest` section references the
      `urgent_events` field and cites `playground.destruction_warning`
- [ ] Addressing-convention section verified present (no edit needed);
      verification recorded in PR description
- [ ] No drift between PROTOCOL.md and `docs/openapi.yaml` for the
      destruction-warning event — payload field names match exactly
- [ ] No "previously" / "newly added" framing — present-tense throughout

## Notes

This is documentation-only. No code change. Small enough to drain in
one stride.

## Implementation notes (2026-05-23)

All four PROTOCOL.md edits applied:

1. **Event-type bullet list (~line 369)** — added `playground.destruction_warning`
   with payload summary `{reason, ends_at, remaining_seconds, session_id}` at
   the end of the list (after `session.ended`), preserving the session-lifecycle
   grouping. Per OpenAPI cross-reference (`docs/openapi.yaml:537`) the schema
   is `PlaygroundDestructionWarningPayload`.

2. **Cross-link openapi schemas** — every event-type bullet now carries a
   `(schema: [Name](./openapi.yaml#/components/schemas/Name))` parenthetical
   pointing at its canonical payload definition. All 13 schema anchors verified
   to exist in `docs/openapi.yaml` via `grep -nE '^    .*Payload:'`. Used the
   `#/components/schemas/Name` JSON-pointer fragment rather than line numbers
   so the links survive openapi.yaml renumbering.

3. **`pre_turn_digest` section (~line 187)** — added a paragraph after the
   Outputs bullet list explaining the `urgent_events` array and naming
   `playground.destruction_warning` as the current sole member of that class.

4. **Addressing-convention section (lines 302-307)** — VERIFIED unchanged.
   The anonymous-handles addressing note is present and matches Design-decisions
   intent (mentions `@amber-otter`, explains durable-vs-anonymous identity-kind
   equivalence). No edit needed; no regression detected.

Verification:
- `grep -niE "previously|newly added|note: in|used to be" docs/PROTOCOL.md` → 0 hits
- `grep -n "playground.destruction_warning" docs/PROTOCOL.md docs/openapi.yaml` →
  hits in both files; payload field names (`reason`, `ends_at`, `remaining_seconds`,
  `session_id`) match the openapi schema exactly.
- All `(schema: ...)` links use `#/components/schemas/<Name>` fragment form;
  every schema name resolves to an existing definition in openapi.yaml.
