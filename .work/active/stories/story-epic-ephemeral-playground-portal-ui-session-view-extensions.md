---
id: story-epic-ephemeral-playground-portal-ui-session-view-extensions
kind: story
stage: implementing
tags: [ui, playground]
parent: feature-epic-ephemeral-playground-portal-ui
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# SessionViewShell playground extensions

## Scope

Story 3 of the parent feature. Extends the existing SessionViewShell
component with playground-mode UI:
- PlaygroundChip in the header chrome (mockup step 03)
- CountdownBadge (client-side ticker, mockup step 03)
- DestructionWarningBanner (idle + hard-cap, mockup steps 07a/07b)
- Post-destruction transition (WS event → navigate to tombstone page)

Full design in the parent feature body's "Story 3" section.

## Files delivered

- `frontend/src/lib/screens/SessionViewShell.svelte` (modify) — render
  playground branch when `session.orgId === 'org_playground'`
- `frontend/src/lib/components/PlaygroundChip.svelte` (new) + test
- `frontend/src/lib/components/CountdownBadge.svelte` (new) + test
- `frontend/src/lib/components/DestructionWarningBanner.svelte` (new) + test

## Acceptance criteria

See the parent feature body's "Story 3 acceptance criteria" section.

## Notes for the implementing agent

- The CountdownBadge uses `$state` for "now" + `$derived` for the two
  remaining-time values. Update "now" via 1-second `setInterval` in
  onMount; clear in onDestroy.
- Page Visibility API: on `visibilitychange` -> visible, recompute
  "now" from `Date.now()` to correct backgrounded-tab drift before
  resuming the ticker.
- WS event handling: subscribe to `playground.activity_reset` (replaces
  `lastSubstantiveActivityAt` prop) and `session.destroyed` (triggers
  navigate to `/playground/s/:id/ended`). Both events flow through the
  existing WS infrastructure — the SessionViewShell already subscribes
  to per-session events; just add handlers for the new envelope kinds.
- Import WS event payload types from the openapi-typescript generated
  client — don't redefine inline. The session-lifecycle feature owns
  the OpenAPI schema additions for these envelopes.
- Warning banner priority: if both idle and hard-cap timers are within
  5 minutes, render the hard-cap warning (it's more urgent — no way to
  reset). One banner shown at a time.
- Durable session render path is unchanged — the playground branch is
  guarded by the org_id check. Regression test the durable path.
