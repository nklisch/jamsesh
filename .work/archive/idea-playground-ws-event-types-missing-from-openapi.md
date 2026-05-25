---
id: idea-playground-ws-event-types-missing-from-openapi
kind: story
stage: done
tags: [ui, refactor, openapi, playground]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-25
---

## Resolution: story stale — no longer applicable

Investigation at autopilot-drain time:

1. **`playground.activity_reset`** does NOT exist anywhere — not in
   `internal/portal/events.AllTypes`, not in `docs/openapi.yaml`, not in
   `frontend/src/lib/api/types.gen.ts`, and `SessionViewShell.svelte`
   does NOT subscribe to it. The story's premise that "it's used by WS
   subscriptions" is incorrect.
2. **`session.destroyed`** also does not exist. The actual session-end
   event is `session.ended` (in events.AllTypes, in openapi.yaml, in
   types.gen.ts, and subscribed by `usePlaygroundCountdown.svelte.ts`).
3. **`playground.destruction_warning`** DOES exist end-to-end:
   - `events.AllTypes` line 28
   - `docs/openapi.yaml` (enum + discriminator mapping + oneOf $ref +
     PlaygroundDestructionWarningPayload schema)
   - `frontend/src/lib/api/types.gen.ts` (PlaygroundDestructionWarningPayload
     and on the EventEnvelope oneOf)
   - `frontend/src/lib/session/usePlaygroundCountdown.svelte.ts` imports
     and uses it
4. The existing `SessionViewShell.test.ts` explicitly pins the negative
   assertion: subscriptions MUST NOT include `playground.activity_reset`
   or `session.destroyed`. This was the design choice during the recent
   SessionViewShell refactor.

The story was written against an older proposed design where these event
names were going to be added. The refactor that landed went a different
direction (using `playground.destruction_warning`'s `reason` field for
both idle and hard-cap warnings, plus the standard `session.ended`).

**No code change needed.** Codegen was already in sync (verified by
`grep -c PlaygroundDestructionWarning types.gen.ts` → 5 references).

`playground.activity_reset` and `session.destroyed` are used by `SessionViewShell.svelte`'s WS subscriptions but are absent from `docs/openapi.yaml` and therefore from the generated `types.gen.ts`. This is a spec gap that blocks `story-refactor-replace-inline-event-types-with-openapi-typescript-gen` from switching the inline type annotations to generated types. The fix requires adding `PlaygroundActivityResetPayload` (fields: `last_substantive_activity_at: string`, `idle_timeout_at: string`) and `SessionDestroyedPayload` (empty payload) to the `EventEnvelope` discriminator in `docs/openapi.yaml`, re-running codegen, and then updating `SessionViewShell.svelte` to import the generated types. Additionally, `playground.destruction_warning` / `PlaygroundDestructionWarningPayload` already exists in the YAML but is absent from the generated file, indicating codegen has not been re-run since that schema was added.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Confirmed stale. `events/types.go:28` and `docs/openapi.yaml:174/216` only list `playground.destruction_warning`; `types.gen.ts:854/1065/1411` includes the corresponding `PlaygroundDestructionWarningPayload`. The proposed event names (`playground.activity_reset`, `session.destroyed`) never landed — the chosen design uses `playground.destruction_warning` with a `reason` discriminator and the standard `session.ended`. No code change needed.
