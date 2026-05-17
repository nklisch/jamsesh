---
id: spa-websocket-reconnect-logic
kind: feature
stage: drafting
tags: [ui]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# SPA WebSocket has no reconnect logic

## Finding

Discovered during e2e implementation of
`epic-e2e-tests-failure-mode-spa-error-states`. The Svelte SPA's
WebSocket layer at `frontend/src/lib/ws.svelte.ts` has no reconnect
behavior. The `close` event handler removes the socket from the map
without retry; users see no UI indicator when the connection drops.

## Why it matters

The portal's WebSocket gateway is the SPA's live-update channel
(commit.arrived, comment.added, merge.succeeded, conflict.detected,
etc.). When network conditions cause a drop:
- The user's session view silently stops updating
- No "reconnecting…" indicator surfaces
- The user might think the app is frozen or "done"

For a collaboration tool where peer activity is the headline feature,
silent WS death is a real UX failure.

## Suggested implementation

Two layers:

1. **Reconnect with backoff**: when the WS closes unexpectedly,
   retry with exponential backoff (1s → 2s → 4s → 8s, capped at 30s).
   On reconnect, replay events from the last-seen cursor (the portal
   supports this via `replay_from` per the gateway docs).

2. **UI status indicator**: surface a visible "reconnecting" state
   when the WS isn't connected. Use `role="status"` with text like
   "Reconnecting..." so screen readers and Playwright tests can
   target it stably.

## Acceptance criteria

- [ ] `ws.svelte.ts` retries with exponential backoff on unexpected
      close
- [ ] On reconnect, the client passes `replay_from: <last-seen-seq>`
      so missed events are delivered
- [ ] A visible UI indicator (text + role="status") shows the
      reconnecting state
- [ ] The Playwright test
      `network_loss_state_shows_reconnecting_indicator_in_session_view`
      in `tests/e2e/playwright/error-states.spec.ts` is un-skipped
      and passes
- [ ] `tests/e2e/fixtures/wsclient/wsclient.go` gets a
      `ConnectFromSeq` helper so the e2e Go layer can drive
      reconnect scenarios too

## Notes

The Playwright test is skipped with `test.skip` and a comment
pointing at this story. Re-enable when reconnect + status UI lands.
