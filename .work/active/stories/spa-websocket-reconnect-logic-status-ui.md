---
id: spa-websocket-reconnect-logic-status-ui
kind: story
stage: implementing
tags: [ui]
parent: spa-websocket-reconnect-logic
depends_on: [spa-websocket-reconnect-logic-backoff]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# SPA WS — role=status reconnecting banner + un-skip Playwright test

## Scope

Build a small `WsStatusBanner.svelte` component that subscribes to
`wsStatus` (landed in `spa-websocket-reconnect-logic-backoff`),
renders a `role="status"` text indicator while the session's WS is
reconnecting, and is absent (not just hidden) when the WS is open.
Mount it in `SessionViewShell.svelte` just below the session header.
Then un-skip the Playwright test
`network_loss_state_shows_reconnecting_indicator_in_session_view` in
`tests/e2e/playwright/error-states.spec.ts`.

## Files touched

- `frontend/src/lib/components/WsStatusBanner.svelte` (new) — the
  banner component.
- `frontend/src/lib/components/WsStatusBanner.test.ts` (new) — vitest
  component tests covering visibility, accessibility, and prop
  reactivity.
- `frontend/src/lib/screens/SessionViewShell.svelte` (edit) — import
  and mount the banner just under the `.session-header` div, before
  the `.top` grid.
- `tests/e2e/playwright/error-states.spec.ts` (edit) — change
  `test.skip(...)` to `test(...)` on the network-loss test; assert
  the banner role+text appears within the existing 10 s timeout.

## Specification

### `WsStatusBanner.svelte`

```svelte
<script lang="ts">
  import { wsStatus } from '$lib/ws.svelte';

  let { sessionId }: { sessionId: string } = $props();

  let status = $derived(wsStatus.for(sessionId));
</script>

{#if status === 'reconnecting'}
  <div class="ws-status" role="status" aria-live="polite">
    <span class="dot" aria-hidden="true"></span>
    <span>Reconnecting…</span>
  </div>
{/if}

<style>
  .ws-status {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 20px;
    background: var(--color-warning-muted);
    color: var(--color-warning);
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-sans);
    border-bottom: 1px solid var(--color-border);
  }

  .dot {
    display: inline-block;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--color-warning);
    animation: ws-pulse 1.5s ease infinite;
    flex-shrink: 0;
  }

  @keyframes ws-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
</style>
```

The banner is **absent** from the DOM when `status !== 'reconnecting'`.
This is deliberate: a `role="status"` element that's `visibility: hidden`
or empty would announce nothing to screen readers AND would still be
findable by `getByRole('status')` in Playwright — which would make the
test pass on every page load, defeating its purpose.

### Mount point in `SessionViewShell.svelte`

Mount once, between the session header and the top grid:

```svelte
<div class="session-header">
  <!-- existing content -->
</div>

<WsStatusBanner {sessionId} />

<!-- Main body: tree rail | artifact -->
<div class="top" ...>
```

### Un-skip the Playwright test

In `tests/e2e/playwright/error-states.spec.ts`, change:

```ts
test.skip(
  "network-loss state shows reconnecting indicator in session view",
  async ({ page, context }) => { ... }
);
```

to a live test. Drop the "Pending" comment. The route abort already
exists; assertion on `page.getByRole("status", { name: /reconnecting/i })`
stays as-is.

The seeded fake token (`"valid-enough-token"`) won't satisfy the
portal, so the WS upgrade will 401 before the route abort fires.
**Adjust** by intercepting the session-load API call to fake a
successful session payload so the session view actually renders, then
abort the WS route. The shape:

```ts
await page.route(/\/api\/orgs\/[^/]+\/sessions\/[^/]+$/, (route) =>
  route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify({
      id: "test-session",
      org_id: "test-org",
      name: "Test session",
      goal: "test",
      scope: "[]",
      default_mode: "sync",
      status: "active",
      members: [],
      created_at: "2026-05-17T00:00:00Z",
    }),
  }),
);
await page.route("**/ws/**", (route) =>
  route.abort("connectionrefused"),
);
```

Verify the exact shape against
`frontend/src/lib/api/types.gen.ts` → `Session` during implementation.
If extra fields are required, add them; if the load shape drifts
before this lands, treat it as a stale-fixture repair (test debt).

## Acceptance criteria

- [ ] `WsStatusBanner.svelte` renders a `role="status"` element with
      text matching `/reconnecting/i` when `wsStatus.for(sessionId)`
      is `'reconnecting'`.
- [ ] The banner is absent from the DOM when status is `'open'`,
      `'connecting'`, or `null`.
- [ ] The banner uses only existing tokens from
      `.mockups/design-system/tokens.css` (no new CSS custom
      properties).
- [ ] `SessionViewShell.svelte` mounts the banner exactly once,
      between the session header and the top grid.
- [ ] The Playwright test
      `network_loss_state_shows_reconnecting_indicator_in_session_view`
      is un-skipped and passes.

## Test approach

Component test (vitest + @testing-library/svelte): set the rune
status to each of `null`, `'open'`, `'connecting'`, `'reconnecting'`
in turn and assert the banner's presence/absence and role/text.

Playwright test: full happy path described above. The route abort
forces the WebSocket connection to fail, the SPA's reconnect loop
flips status to `'reconnecting'`, and the banner appears.

## Notes

- The banner's design fits within the existing palette — D6 in the
  feature design justifies skipping a separate mockup.
- The status rune is not specific to any single component; future
  consumers (e.g. a network-health indicator in the chrome) can
  subscribe to the same rune.
