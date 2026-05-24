---
id: gate-docs-ux-playground-flow-not-documented
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

# UX.md does not describe the playground UX flow that shipped in this bundle

## Drift category
foundation-doc-assertion

## Location
- Doc: `docs/UX.md:91-96` (stale "Forward reference" note) and missing playground entries in "Portal UI surfaces" `docs/UX.md:281-310`
- Code: `frontend/src/lib/screens/PlaygroundLanding.svelte`, `JoinerPicker.svelte`, `SessionTombstone.svelte`, `frontend/src/lib/components/{PlaygroundChip,CountdownBadge,DestructionWarningBanner,JoinerForm,JoinerOutcome}.svelte`, `frontend/src/lib/router.svelte.ts:23-27`

## Current doc text
> ### Forward reference
>
> `jamsesh new --playground` (non-durable, anonymous-bearer sessions) is shipped in the `session-lifecycle` sibling feature (wave 2 of this epic). The flag and all durable-session mechanics are identical; the difference is the org binding and the session lifetime indicator shown in the output.

## Reality
The playground epic shipped in v0.4.0 with a full anonymous-entry UX flow: public `/playground` landing page, open-join page at `/playground/s/{id}/join` (nickname → anonymous bearer exchange via `JoinerForm`/`JoinerOutcome`), post-destruction tombstone view at `/playground/s/{id}/ended`, always-visible `CountdownBadge` showing remaining session time, `DestructionWarningBanner` that fires inside the warning window, and a `PlaygroundChip` marker on session-list rows. None of these surfaces are documented in UX.md. The "Forward reference" framing is itself stale — this is no longer forward-looking.

## Required edit
Replace the "Forward reference" subsection with new "Flow: spinning up a playground" (creation from landing page, anonymous-bearer exchange, share URL) and "Flow: joining a playground" (URL → joiner picker → nickname-and-bearer flow → playground session view). Add three playground entries to "Portal UI surfaces" (lines 281-310): playground landing, joiner picker, session tombstone. Document `CountdownBadge`, `DestructionWarningBanner`, and `PlaygroundChip` as session-view chrome components when a session has playground identity.
