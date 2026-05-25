---
id: idea-playground-ws-event-types-missing-from-openapi
kind: story
stage: drafting
tags: [ui, refactor, openapi, playground]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-25
---

`playground.activity_reset` and `session.destroyed` are used by `SessionViewShell.svelte`'s WS subscriptions but are absent from `docs/openapi.yaml` and therefore from the generated `types.gen.ts`. This is a spec gap that blocks `story-refactor-replace-inline-event-types-with-openapi-typescript-gen` from switching the inline type annotations to generated types. The fix requires adding `PlaygroundActivityResetPayload` (fields: `last_substantive_activity_at: string`, `idle_timeout_at: string`) and `SessionDestroyedPayload` (empty payload) to the `EventEnvelope` discriminator in `docs/openapi.yaml`, re-running codegen, and then updating `SessionViewShell.svelte` to import the generated types. Additionally, `playground.destruction_warning` / `PlaygroundDestructionWarningPayload` already exists in the YAML but is absent from the generated file, indicating codegen has not been re-run since that schema was added.
