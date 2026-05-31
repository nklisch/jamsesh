---
id: epic-cli-browser-session-resume-spa-route-route-screen
kind: story
stage: implementing
tags: [ui]
parent: epic-cli-browser-session-resume-spa-route
depends_on: [epic-cli-browser-session-resume-spa-route-auth-access-only]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Resume routes + `ResumeExchange.svelte`

Implements **Unit 2** of `epic-cli-browser-session-resume-spa-route`. Mirrors
`MagicLinkExchange.svelte`. Uses `auth.setAccessOnly` from Unit 1. See feature
body (Other agent review + Unit 2) for the full rules.

## Scope

- `frontend/src/lib/router.svelte.ts`: add public (`requiresAuth:false`) routes
  `/playground/s/{sessionId}/resume` and `/orgs/{orgId}/sessions/{sessionId}/resume`,
  mounted BEFORE `session-view`.
- `frontend/src/lib/App.svelte`: map the routes → `ResumeExchange`.
- `frontend/src/lib/screens/ResumeExchange.svelte` (mirror MagicLinkExchange,
  union state `exchanging | confirming | error`):
  - `onMount`: read `#rt` from `location.hash` (missing → error);
    `history.replaceState(null,'',pathname+search)` to strip immediately.
  - Exchange via a BARE `fetch('/api/session-resumes/exchange', {method:'POST',
    credentials:'omit', headers:{'content-type':'application/json'},
    body: JSON.stringify({resume_token})})` — NOT the shared `client`.
  - Confirm logic: adopt directly when no existing identity; else `confirming`
    (currentUser differs from response `account_id`, or unconfirmable, or an
    existing playground context) → accept adopts / decline shows generic retry
    hint and does NOT persist the bearer. Keep only `display_name` in `$state`.
  - Adopt: playground (`kind==="playground"`) → MIRROR `JoinerPicker.svelte`'s
    successful-join handling (do not reinvent the playground-bearer wiring);
    durable → `auth.setAccessOnly(bearer)`.
  - Navigate from the response: playground → `/orgs/org_playground/sessions/{session_id}`;
    durable → `/orgs/{org_id}/sessions/{session_id}` (encode segments).
  - Failure/expired/used → generic error + retry hint (no oracle).
- Roll forward `docs/UX.md` (resume landing in create/join journeys) +
  `docs/SECURITY.md` (client-side fragment-strip / bare-fetch / no-token-logging).

## Acceptance criteria

- [ ] Exchange sends NO `Authorization` header; `#rt` stripped before any other
      nav/asset; resume token + bearer never logged or rendered.
- [ ] No existing identity → adopt directly + navigate (playground via
      JoinerPicker's path; durable via `setAccessOnly`).
- [ ] Existing differing/unconfirmable identity → `confirming`; accept adopts,
      decline → retry hint + bearer NOT persisted.
- [ ] Expired/used/invalid → generic error + retry hint (same shape, no oracle).
- [ ] Public route flags; nav paths derived from the response.
- [ ] vitest + typecheck pass.

## Notes

Test patterns: `spa-test-module-mock-barrel`, `window-location-defineproperty-stub`,
`view-state-union-machine`. Read `JoinerPicker.svelte` to replicate the working
playground adoption.
