---
id: story-sessionviewshell-test-playground-branch-coverage
kind: story
stage: review
tags: [test, ui, playground]
parent: null
depends_on: [story-playground-ws-protocol-mismatch-session-view-extensions]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
---

# SessionViewShell.test.ts misses the playground branch entirely

`frontend/src/lib/screens/SessionViewShell.test.ts` only exercises the
durable session render path. After
`story-epic-ephemeral-playground-portal-ui-session-view-extensions`
landed, the new playground branch (`isPlayground` true →
PlaygroundChip + CountdownBadge + DestructionWarningBanner +
playground WS subscriptions + post-destruction navigate) has no
coverage at the shell level. PlaygroundChip / CountdownBadge /
DestructionWarningBanner have their own isolated tests, but the
integration — "does SessionViewShell mount them with the right
data when org_id is org_playground, subscribe to the right events,
and navigate correctly on destroy" — is uncovered.

This gap is what allowed the protocol-mismatch bug
(`story-playground-ws-protocol-mismatch-session-view-extensions`) to
slip past a green test suite.

Once the protocol mismatch is resolved, extend
SessionViewShell.test.ts with:

- Playground render path: mock `client.GET` to return a
  playground-shaped Session, assert PlaygroundChip and
  CountdownBadge mount.
- WS subscribe path: assert that the `subscribe(sessionId,
  '<destruction event>', ...)` and `subscribe(sessionId,
  '<activity event>', ...)` calls are registered (existing tests
  already use the `ws.svelte` module-mock pattern from
  `spa-test-module-mock-barrel`).
- Post-destruction navigation: dispatch the destruction envelope
  through the subscribed handler and assert `navigate` is called
  with `/playground/s/:id/ended`.
- Regression guard for the durable path: ensure none of the
  playground UI renders when `org_id !== 'org_playground'`.

## Implementation discovery — fixed via sibling story

All acceptance criteria were already satisfied by sibling story
`story-playground-ws-protocol-mismatch-session-view-extensions`
(commit `d50e575`). The playground describe block in
`frontend/src/lib/screens/SessionViewShell.test.ts` (lines 278–396)
covers every criterion:

- **Correct endpoint called** (not orgs endpoint): test at line 297
  asserts `GET /api/playground/sessions/{id}` is called and the
  org-scoped endpoint is NOT called.
- **PlaygroundChip present**: test at line 313 asserts
  `aria-label="Playground session"` renders; regression guard at line
  400 confirms it is absent for durable sessions.
- **subscribe() with correct event type names**: test at line 321
  asserts `playground.destruction_warning` and `session.ended` are
  subscribed, and NOT the legacy broken names `playground.activity_reset`
  or `session.destroyed`.
- **session.ended fires navigation**: test at line 340 dispatches the
  `session.ended` envelope and asserts `navigate` is called with
  `/playground/s/:id/ended`.
- **playground.destruction_warning updates idle timer / CountdownBadge
  urgent**: test at line 355 fires the warning event with
  `reason=idle_timeout` and 3 minutes remaining, then asserts the badge
  gains the `urgent` CSS class.

Verification: `npm run check` — 0 errors, `npm run test` — 635/635 passed.
