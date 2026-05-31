---
id: epic-cli-browser-session-resume-spa-route-route-screen
kind: story
stage: done
tags: [ui]
parent: epic-cli-browser-session-resume-spa-route
depends_on: [epic-cli-browser-session-resume-spa-route-auth-access-only]
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
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

## Implementation notes

- **Router**: added `playground-resume` (`/playground/s/{sessionId}/resume`,
  `requiresAuth:false`) and `session-resume` (`/orgs/{orgId}/sessions/{sessionId}/resume`,
  `requiresAuth:false`) routes, both inserted BEFORE `session-view` to win
  under first-match semantics.
- **App.svelte**: imports `ResumeExchange` and maps both `playground-resume` and
  `session-resume` → `<ResumeExchange />` (no props needed; reads route from
  exchange response).
- **ResumeExchange.svelte**: union state `exchanging | confirming | error`.
  `onMount` reads `#rt`, strips via `history.replaceState(null,'',pathname+search)`,
  then fires bare `fetch` with `credentials:'omit'` and no Authorization header.
  Success with no existing identity → adopt directly. Existing differing/unknown
  identity → `confirming` (only `display_name` in `$state`; bearer kept in plain
  non-reactive local). Accept → adopt; decline → wipe bearer + transition to
  `error`. Playground adoption mirrors `JoinerPicker` success path exactly
  (`auth.setPlaygroundContext({sessionId, bearer, nickname})`). Durable adoption
  uses `auth.setAccessOnly(bearer)`. Navigation derived from the exchange response.
  Error/missing token shows generic retry hint only — no oracle.
- **Docs**: `docs/UX.md` — "Flow: resuming a session from the CLI" added before
  the "joining a session" flow. `docs/SECURITY.md` — "CLI resume-exchange
  client-side safeguards" section added (fragment strip, bare fetch
  `credentials:omit`, token/bearer never in reactive state).
- **Tests** (18 passing): bare-fetch-no-Authorization, `#rt` strip via
  `replaceState`, durable adopt+navigate, playground adopt+navigate (via
  `setPlaygroundContext`), URI segment encoding, confirm accept/decline,
  all three confirming triggers (differing id, authenticated+null user,
  existing playground context), error on non-ok + network failure, token/bearer
  never rendered in DOM. `vitest` 18/18 green; `svelte-check` 0 errors.
