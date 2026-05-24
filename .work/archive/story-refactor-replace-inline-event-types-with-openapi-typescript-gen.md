---
id: story-refactor-replace-inline-event-types-with-openapi-typescript-gen
kind: story
stage: done
tags: [ui, refactor, cleanup]
parent: feature-spec-discipline
depends_on: [story-spec-discipline-audit-and-close-emit-vs-yaml-gaps]
release_binding: null
gate_origin: refactor-design
created: 2026-05-23
updated: 2026-05-24
---

# Replace inline WS event-type annotations with openapi-typescript-generated types

## Brief

Two frontend files carry inline event-type annotations with TODOs
explicitly pointing at the openapi-typescript regeneration as the
mechanical follow-up. The note in
`feature-epic-ephemeral-playground-portal-ui.md` already calls this
out as deferred-mechanical work:

> "Session-view-extensions WS event payload types defined inline with
> TODO pointing at openapi-typescript regeneration (since
> session-lifecycle landed earlier in the same wave, the integration
> fix is mechanical)"

Surfaced by a discovery-mode `/agile-workflow:refactor-design` scan.

## Targets

- `frontend/src/lib/components/CountdownBadge.svelte:23` — `// TODO:
  replace inline event type annotation with openapi-typescript generated`
- `frontend/src/lib/screens/SessionViewShell.svelte:30` — `// TODO:
  replace with openapi-typescript generated types once`

## Target state

In each file, drop the inline-typed event payload shape and switch to
the appropriate generated type from
`frontend/src/lib/api/types.gen.ts`. The relevant generated symbols
will already exist because `docs/openapi.yaml` is the single source of
truth and the session-lifecycle WS payloads are part of the spec.

```ts
// Before — inline type
type IdleResetEvent = { type: 'idle.reset', payload: { ... } };

// After — generated
import type { components } from '$lib/api/types.gen';
type EventEnvelope = components['schemas']['EventEnvelope'];
```

## Acceptance criteria

- Both files import `components` from `$lib/api/types.gen` and use
  the generated schema names for event payloads.
- The inline TODO comments are removed.
- `npm run check` clean.
- `npm run test` passes.
- No visible behavior change — same fields read at the same times.

## Notes

If the generated types do not yet cover the payload shape used by
the inline annotation, surface that as a docs/openapi.yaml drift
story rather than papering over it inline. The intent here is
**only** to switch to existing generated types, not to patch the spec.

Behavior-preserving — pure type-import change.

## Implementation notes

**Spec gap found — story returned to `drafting`.**

Investigation on 2026-05-23 confirmed that both target event types are absent from
`docs/openapi.yaml` and from the generated `frontend/src/lib/api/types.gen.ts`:

- `playground.activity_reset` — used in `SessionViewShell.svelte` line ~194.
  Fields: `last_substantive_activity_at: string`, `idle_timeout_at: string`.
  Not present in the `EventEnvelope` discriminator enum or payload `oneOf` in
  `docs/openapi.yaml`.
- `session.destroyed` — used in `SessionViewShell.svelte` line ~207.
  Not present in `docs/openapi.yaml`. (The spec has `session.ended` for durable
  sessions; playground uses a different event name.)

Additionally, `playground.destruction_warning` and `PlaygroundDestructionWarningPayload`
exist in `docs/openapi.yaml` (added by the playground epic) but the generated
`types.gen.ts` does not include them, indicating codegen has not been re-run since
that schema was added.

**CountdownBadge.svelte**: confirmed no inline event-payload type in the file body.
The TODO was informational only. Refreshed it to point at the parked id.

**SessionViewShell.svelte**: inline `PlaygroundActivityResetEvent` and
`SessionDestroyedEvent` types remain in place. The TODO comments were updated to
reference `idea-playground-ws-event-types-missing-from-openapi`.

**Parked spec gap**: `idea-playground-ws-event-types-missing-from-openapi`
(`.work/backlog/idea-playground-ws-event-types-missing-from-openapi.md`)

**Unblocking path**: once the spec gap backlog item is scoped and worked
(add the two payload schemas + `EventEnvelope` discriminator entries to
`docs/openapi.yaml`, re-run codegen), re-open this story at `implementing`
and it becomes a straightforward `import type { components }` swap.

**2026-05-24 — Implementation complete (dep story closed the spec gap).**

`story-spec-discipline-audit-and-close-emit-vs-yaml-gaps` is done; codegen
re-ran and `types.gen.ts` now includes `PlaygroundDestructionWarningPayload`,
`SessionEndedPayload`, and both event types in the `EventEnvelope` discriminator.

**TODOs removed:**
- `frontend/src/lib/components/CountdownBadge.svelte` — stale informational
  `TODO(idea-playground-ws-event-types-missing-from-openapi)` comment block
  removed (5 lines). File has no inline event-payload type.
- `frontend/src/lib/session/usePlaygroundCountdown.svelte.ts` — entire
  `TODO(...)` comment and both inline type definitions removed.

**Types swapped in `usePlaygroundCountdown.svelte.ts`:**
- `PlaygroundDestructionWarningEvent` (inline) →
  `components['schemas']['PlaygroundDestructionWarningPayload']`
- `SessionEndedEvent` (inline) →
  `components['schemas']['SessionEndedPayload']`
- Handler casts updated to match: `env as unknown as PlaygroundDestructionWarningPayload`
  and `_env as SessionEndedPayload`.

**`SessionViewShell.svelte`** — already clean; `import type { components }` was
present with no inline event-payload types remaining.

**Verification:** `npm run check` (0 errors), `npm run test` (635/635 pass),
`npm run build` — all clean.

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Implementation matches design. Drift-ci-check caught a bonus gap (`auto-merger.backpressure`) and closed it inline — proves the test's value immediately. Pattern doc indexed in both rules and SKILL.md; SPEC.md cross-reference added. Replace-inline-event-types swapped 2 inline payload types to generated imports; the third file (CountdownBadge) had no inline type and only the stale TODO removed.
