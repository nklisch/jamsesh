---
id: feature-spec-discipline
kind: feature
stage: done
tags: [portal, ui, documentation, infra]
parent: null
depends_on: []
release_binding: v0.4.0
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
| `story-spec-discipline-audit-and-close-emit-vs-yaml-gaps` | drafting | — |
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

## Design Discovery (feature-design 2026-05-24)

The Phase 3 audit revealed the original child story's framing was wrong
in detail but right in spirit:

- The events `playground.activity_reset` and `session.destroyed` (the
  original spec-gap claim) **do not exist** — they were wrong frontend
  assumptions. The actual events are `session.ended` and
  `playground.destruction_warning`, both already in the YAML enum.
  The portal-ui session-view-extensions story (`d50e575`) corrected
  the frontend to use the real names.

- The **real** gap class is server-emits-not-specced. Concrete example
  found: `session.created` is emitted by `sessions/handler.go:155`,
  used by `SessionList.svelte`, but absent from the YAML enum.

- A second gap class: codegen-stale. `playground.destruction_warning`
  is in the YAML but missing from `types.gen.ts` — `make generate`
  hasn't been re-run since that schema added.

The child story is rescoped accordingly:

- `story-spec-discipline-add-playground-event-payloads` →
  **renamed** to `story-spec-discipline-audit-and-close-emit-vs-yaml-gaps`.
  Scope: audit Go emit vs YAML, close all observed gaps, run codegen,
  re-enable frontend type imports. Now at `stage: implementing` since
  the audit-and-fix work is concrete.

Sibling stories unchanged:
- `story-spec-discipline-drift-ci-check` — CI test catches the
  gap-class going forward. Still depends on the (renamed) audit story.
- `story-spec-discipline-pattern-doc` — documents the rule. Still
  depends on drift-ci-check.

## Implementation Order

Serial chain:
1. `story-spec-discipline-audit-and-close-emit-vs-yaml-gaps` —
   closes the current gaps + runs codegen
2. `story-spec-discipline-drift-ci-check` — prevents recurrence
3. `story-spec-discipline-pattern-doc` — codifies the rule
4. `story-refactor-replace-inline-event-types-with-openapi-typescript-gen` —
   unblocked once codegen refreshes the generated types

## Review (2026-05-24)

**Verdict**: Approve — feature delivered as briefed.

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: All 4 child stories at done. Aggregate review: spec-discipline closed end-to-end. The audit closed 2 real gaps (`session.created`, `auto-merger.backpressure`) plus the codegen-stale issue (`PlaygroundDestructionWarningPayload`). The CI test (`TestEventTypeConstants_MatchOpenAPIYAML`) enforces the rule going forward and explicitly caught the `auto-merger.backpressure` gap during the drift-ci-check implementation. The pattern doc + foundation-doc cross-reference make the rule discoverable. Verification: `go build`, `go test ./...`, `npm run check/test/build` all clean.

**What's now possible**: a future developer who adds a new event-emit string without updating `docs/openapi.yaml` gets a clear CI failure pointing at the resolution flow. The frontend's WS event payload types are now spec-driven end-to-end — no inline workarounds remain.
