---
id: feature-spec-discipline
kind: feature
stage: drafting
tags: [portal, ui, documentation, infra]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Spec-first discipline: complete the playground event payloads, prevent drift

## Brief

`docs/openapi.yaml` is the authoritative source of truth for portal REST
routes and WebSocket event envelopes. Both Go (`oapi-codegen`) and
TypeScript (`openapi-typescript`) regenerate from it via `make generate`.

The autopilot refactor-design discovery surfaced a real spec-drift case:
the Go server emits `playground.activity_reset` and `session.destroyed`
WebSocket events that are NOT declared in the YAML. The frontend handled
this with inline type annotations + a TODO. Same scan found
`PlaygroundDestructionWarningPayload` *is* in the YAML but absent from
`frontend/src/lib/api/types.gen.ts` — meaning `make generate` hadn't
been re-run since that schema was added.

This feature closes the gap and installs guardrails so it cannot
silently recur.

## Scope

Three concerns, decomposed into the child stories below:

1. **Add the missing payload schemas** to `docs/openapi.yaml`, run
   codegen, update the two frontend files to import from
   `types.gen.ts`. Unblocks the existing drafting story
   `story-refactor-replace-inline-event-types-with-openapi-typescript-gen`.

2. **CI drift check**: a small Go test that asserts every server-emit
   event-type string constant appears in the YAML's `EventEnvelope.type`
   enum. Prevents the original bug class from recurring.

3. **Pattern documentation**: codify the spec-first event-type
   discipline as a new pattern entry, so this convention is
   discoverable and rolling-foundation rules apply if it ever drifts.

## Children (declared up front; design pass refines)

| Child | Stage | Depends on |
|---|---|---|
| `story-spec-discipline-add-playground-event-payloads` | drafting | — |
| `story-spec-discipline-drift-ci-check` | implementing | add-playground-event-payloads |
| `story-spec-discipline-pattern-doc` | implementing | drift-ci-check |
| `story-refactor-replace-inline-event-types-with-openapi-typescript-gen` (existing, re-parented) | drafting | add-playground-event-payloads |

## Design notes

- The spec discipline IS the project's existing convention per
  `docs/SPEC.md` §Stack — this feature does not invent it, it just
  closes the one observed leak and adds the missing CI guardrail.
- The drift check is server-side (Go test) because the Go server is
  where the event-type strings originate. A frontend variant
  (asserting `types.gen.ts` matches the YAML) is unnecessary —
  `openapi-typescript` generates from the YAML directly, so they
  cannot diverge.
- The pattern doc, once written, should be referenced from
  `docs/SPEC.md`'s "Generated contracts" section and from
  `.claude/rules/patterns.md`.

## Acceptance criteria (feature-level)

- `playground.activity_reset` and `session.destroyed` are full citizens
  of the `EventEnvelope` discriminator in `docs/openapi.yaml`.
- `frontend/src/lib/api/types.gen.ts` includes both new payload types
  AND the previously-missing `PlaygroundDestructionWarningPayload`.
- `SessionViewShell.svelte` and `CountdownBadge.svelte` import the
  generated types; their inline TODOs are removed.
- A CI test fails if a Go-emitted event type is missing from the YAML
  enum, OR vice-versa.
- The pattern is documented at
  `.claude/skills/patterns/spec-driven-event-types.md` and indexed in
  `.claude/rules/patterns.md`.

## Out of scope

- Restructuring the spec authoring workflow generally. The convention
  already exists; this feature just enforces and closes one leak.
- Anything code-first about OpenAPI. The direction stays spec-first.
