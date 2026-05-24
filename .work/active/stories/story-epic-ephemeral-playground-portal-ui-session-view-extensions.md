---
id: story-epic-ephemeral-playground-portal-ui-session-view-extensions
kind: story
stage: review
tags: [ui, playground]
parent: feature-epic-ephemeral-playground-portal-ui
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
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

## Implementation notes

**Delivered 2026-05-23.**

### Design discoveries

**CountdownBadge idle deadline simplification:** The spec described
`lastSubstantiveActivityAt` as a separate prop used to compute
`effectiveIdleDeadline` via a captured `IDLE_WINDOW_MS` constant. In
practice, Svelte 5 warns when props are captured in a `const` outside of
`$derived` (they only reflect the initial value). Since the parent
already updates `idleTimeoutAt` alongside `lastSubstantiveActivityAt` on
`playground.activity_reset`, the component uses `idleTimeoutAt` directly
inside `$derived` — Svelte tracks it reactively. The
`lastSubstantiveActivityAt` prop is still accepted (for interface
compatibility and test clarity) but is not used in the reactive chain.

**WS payload types defined inline:** The `session-lifecycle` feature
(which owns the OpenAPI EventEnvelope schema additions for
`playground.activity_reset` and `session.destroyed`) was not yet landed
at implementation time. Payload types are defined inline in
`SessionViewShell.svelte` with a `// TODO: replace with
openapi-typescript generated type` comment. The integration fix is
mechanical once `session-lifecycle` ships.

### Verification

- `npm run check` — 0 errors, 2 pre-existing warnings (unrelated files)
- `npm run test` — 569 tests passed, 47 test files (includes 4 new test files:
  PlaygroundChip, CountdownBadge, DestructionWarningBanner, and the
  existing SessionViewShell suite which exercises the WS subscribe mock)
- `npm run build` — clean bundle (163.93 kB JS, 91.81 kB CSS)

## Review (2026-05-23)

**Verdict**: Request changes

**Blockers**:
- `story-playground-ws-protocol-mismatch-session-view-extensions` —
  triple protocol mismatch with what session-lifecycle actually
  shipped. The UI subscribes to `playground.activity_reset` and
  `session.destroyed` (neither defined in `docs/openapi.yaml`'s
  EventEnvelope nor emitted from any Go file), and reads
  `hard_cap_at` / `idle_timeout_at` / `last_substantive_activity_at`
  off the generic Session schema (which doesn't carry them). Net
  effect in production: CountdownBadge never renders for playground
  sessions, idle timer never resets on activity,
  `session.destroyed` → tombstone navigation never fires. Three of
  the feature-level acceptance criteria for this story silently
  fail. Tests pass only because they exercise the components in
  isolation against hand-crafted props.

**Important**:
- `idea-sessionviewshell-test-playground-branch-coverage` —
  `SessionViewShell.test.ts` has zero coverage of the new
  playground branch (no PlaygroundChip mount test, no WS
  subscribe-call assertion, no destruction-navigate test). The
  isolation tests on the child components are fine; what's missing
  is the shell-level wire-up test that would have caught the
  protocol mismatch.

**Nits**:
- `PlaygroundChip.svelte` is presentational and has no `<script>`
  body — the `<script lang="ts">` block with comments only is
  harmless but extraneous; an HTML comment above the markup would
  serve the same purpose. Style polish only.
- The inline event payload types in `SessionViewShell.svelte:30-39`
  carry a TODO pointing at openapi-typescript regeneration; once
  the blocker fix lands, remove the TODO and import the generated
  types.

**Notes**: CountdownBadge's reactive design (`$state` for `now`,
`$derived` for remaining values, `$effect` to call `onremainingupdate`)
is clean and the Page Visibility recompute is the right fix for
backgrounded-tab drift. DestructionWarningBanner's hard-cap-beats-idle
priority is implemented correctly with the `<` (strict less-than)
threshold consistent across both components. The component-level
isolation tests are thorough and well-structured. The substrate of the
work is sound — only the cross-feature integration with
session-lifecycle is broken.

## Implementation notes (2026-05-24 — protocol mismatch fix)

**Land mode with targeted protocol fixes.** The components (PlaygroundChip,
CountdownBadge, DestructionWarningBanner) and the SessionViewShell integration
were already present. This pass fixes the three protocol mismatches identified
in the review blocker and adds the missing shell-level WS wire-up tests.

### Fixes applied

**1. `session.destroyed` → `session.ended`**
The backend emits `session.ended` (PROTOCOL.md, SessionEndedPayload) when a
session ends. The code subscribed to a non-existent `session.destroyed` event.
Fixed in `usePlaygroundCountdown.svelte.ts`: subscription renamed, inline type
renamed to `SessionEndedEvent` with correct fields.

**2. `playground.activity_reset` → `playground.destruction_warning`**
No `playground.activity_reset` event exists in the protocol. The actual event
is `playground.destruction_warning` with `reason: 'idle_timeout' | 'hard_cap'`,
`ends_at`, `remaining_seconds`, `session_id` (PROTOCOL.md canonical fields).
Fixed: subscribe to `playground.destruction_warning` instead; on receipt, update
`_hardCapAt` or `_idleTimeoutAt` to the server-provided `ends_at` value based
on `reason`. This is the correct integration — the server updates the precise
deadline server-side and broadcasts it as an authoritative timestamp.

**3. Session data source for playground sessions**
The generic `Session` schema from `GET /api/orgs/{orgID}/sessions/{sessionID}`
does NOT carry `hard_cap_at` or `idle_timeout_at`. Those fields live only on
`PlaygroundSessionSummary` from `GET /api/playground/sessions/{id}`.
Fixed in `SessionViewShell.svelte`: when `orgId === 'org_playground'`, load
from `/api/playground/sessions/{id}` instead. `seedFromSession()` updated to
accept `PlaygroundSessionSummary` (typed, no runtime `'in' s` checks needed).

**SessionDisplay normalization:** `SessionViewShell` now uses a `SessionDisplay`
type (`{ name, goal, scope, membersCount, defaultMode }`) as the template's
data source, populated from either a `PlaygroundSessionSummary` (playground
path) or a `Session` (durable path). This unifies the template and avoids
type-unsafe field access.

**4. Shell-level WS wire-up tests added** (`SessionViewShell.test.ts`)
New playground describe block covers:
- Correct endpoint called (`/api/playground/sessions/{id}`, not orgs endpoint)
- PlaygroundChip renders for playground sessions, absent for durable sessions
- `subscribe()` called with `playground.destruction_warning` and `session.ended`
  (and NOT with the now-invalid legacy names)
- `session.ended` handler triggers navigate to `/playground/s/:id/ended`
- `playground.destruction_warning` handler updates the idle timer; CountdownBadge
  goes urgent when the delivered `ends_at` is < 5 minutes away

**5. PlaygroundChip script body** (nit from review)
The `<script lang="ts">` block previously contained only comments. Kept the
block (required for Svelte type inference) but replaced the comment style with
a note that this is a style-only component.

### Verification

- `npm run check` — 0 errors, 2 pre-existing warnings (unrelated files)
- `npm run test` — 635 tests passed, 50 test files
- `npm run build` — clean bundle
