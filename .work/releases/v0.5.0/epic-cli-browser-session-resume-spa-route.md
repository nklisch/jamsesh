---
id: epic-cli-browser-session-resume-spa-route
kind: feature
stage: done
tags: [ui]
parent: epic-cli-browser-session-resume
depends_on: [epic-cli-browser-session-resume-portal-contract]
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
---

# SPA resume route

## Brief

The browser consumer side: a `/…/resume` route that reads the single-use resume
token from `location.hash`, immediately strips it via `history.replaceState`,
exchanges it at the portal for a browser bearer, stores that bearer in the
correct auth state, and lands the user in the live session as their CLI
identity. Handles the playground vs durable credential difference on the receive
side, plus error/expiry UX (expired/used/invalid token).

Does NOT implement the portal endpoints (sibling `…-portal-contract`) or the CLI
mint/open (sibling `…-cli-handoff`).

## Epic context

- Parent epic: `epic-cli-browser-session-resume`
- Position in epic: consumer of `…-portal-contract`. Independent of
  `…-cli-handoff` — parallelizable once the contract lands.

## Foundation references

- `frontend/src/lib/screens/MagicLinkExchange.svelte` — **direct precedent**: a
  transitional screen that reads a token from the URL, exchanges it, and sets
  auth state. The resume route mirrors this shape.
- `frontend/src/lib/auth.svelte.ts` — `setPlaygroundContext(ctx)` (playground
  in-memory rune) and `setTokens(access, refresh)` (durable) are the two state
  setters the exchange result feeds, depending on session kind.
- `frontend/src/lib/router.svelte.ts` — route registration (the resume route is
  public/unauthenticated until the exchange succeeds, like `playground-join`).
- `frontend/src/lib/screens/JoinerOutcome.svelte` — precedent for the
  error/expiry outcome view.

## Mockups / UI alignment

**No net-new mockup.** This is a transitional exchange screen that reuses the
existing `MagicLinkExchange` / `JoinerOutcome` patterns and the established
design system (`.mockups/design-system/tokens.css`). Per the ux-ui-principles
decision matrix, minor composition reusing existing patterns skips mocking. If
the feature-design pass finds the error/expiry UX genuinely novel, it may fall
back to `/ux-ui-design:screens` then.

## Design decisions

(Captured in the questions-only alignment pass, 2026-05-30.)

- **Account-mismatch handling**: if the browser is already authenticated as a
  *different* account when a resume link is opened, **confirm the switch** with
  the user before clearing the existing session and adopting the resumed
  identity. The exchange response carries identity metadata (`account_id` +
  `display_name`, added to the contract in the final review) so the SPA detects
  the mismatch WITHOUT a second `/me` probe.
- **Durable adoption is access-only** [final-review]: the contract returns an
  `IssueShortLived` access token with NO refresh, but the current durable setter
  `auth.setTokens(access, refresh)` expects both. The full design must add an
  access-only adoption path that clears any stale refresh / cached current-user
  and handles the 1h access expiry cleanly (re-resume / re-login), rather than
  reusing `setTokens` as-is.
- **Error / expiry UX**: an expired / already-used / invalid resume token shows
  a **generic message + retry hint** ("this resume link expired or was already
  used — run the command again from your terminal"), with NO detail that
  distinguishes the cases (no oracle). Stays on the resume route (no redirect).

The full design pass settles: the confirm-switch UI, the exact copy, and how the
generic-error state composes with the existing `JoinerOutcome` pattern.

## Decomposition-review findings (Codex, accepted — fold into this feature's design)

- **Guard against ambient browser auth.** [important] The shared API client
  (`frontend/src/lib/api/client.ts`) attaches `auth.token` to every request. A
  user already logged in as another account would send an unrelated
  `Authorization` header to the public exchange. The exchange call must use a
  bare/unauthenticated fetch (the resume token is the sole credential), and the
  route must define behavior on account mismatch (e.g. clear existing auth
  before adopting the resumed identity, or surface a mismatch error).
- **Use the contract-owned route path + `rt` fragment key** (defined in
  `…-portal-contract`) for the route registration — single source of truth with
  the CLI.
- **Branch the store-into-auth-state by session kind**: playground →
  `auth.setPlaygroundContext` (in-memory rune); durable → the SPA post-login
  state (access-only browser session — the SPA must not expect the CLI's refresh
  token from the exchange).

## Foundation-doc roll-forward (at implementation)

`docs/UX.md` (the resume landing in the create/join journeys); `docs/SECURITY.md`
note on the fragment-strip / referrer-policy / same-origin-exchange client-side
safeguards (alongside the contract feature's server-side threat model).

## Other agent review (Codex xhigh advisory, 2026-05-30)

Accepted points folded into the design below:
- **Playground bearer gap [verify first].** `auth.setPlaygroundContext` stores
  `bearer`, but `bearerMiddleware` only sends `auth.token`, and `session-view`
  needs auth. The resume screen's playground adoption must MIRROR exactly what
  the working `JoinerPicker.svelte` does on a successful join (it works in
  production) — do NOT reinvent the playground-bearer wiring. Read JoinerPicker's
  success path and replicate.
- **Confirm-switch is post-exchange** (the single-use token is already consumed
  when we learn the identity). On decline → show the generic retry hint (run the
  command again); do NOT persist the returned bearer until confirmed; keep only
  display-safe fields (`display_name`) in reactive `$state`, never the bearer/token.
- **Confirm only when an existing identity is present.** No durable token AND no
  playground context → adopt directly (the common case). `auth.currentUser?.id`
  exists and differs from response `account_id` → confirm. `isAuthenticated` but
  `currentUser` null → confirm conservatively (no `/me` probe). Existing
  playground context has no `account_id` → treat as unknown → confirm
  conservatively.
- **Bare fetch** for the exchange: hand-rolled `fetch('/api/session-resumes/
  exchange', {method:'POST', credentials:'omit', headers JSON, body})` — NOT the
  shared `client` (its `bearerMiddleware` would attach another account's bearer).
- **Navigate from the response**, not route params: playground →
  `/orgs/org_playground/sessions/{session_id}`, durable →
  `/orgs/{org_id}/sessions/{session_id}` (encode segments; reuse `session-view`).
- **Strip `#rt` immediately** via `history.replaceState(null,'',pathname+search)`;
  never log/render the token or bearer; don't put the full exchange response in
  reactive state.
- Svelte footguns: `onMount` (not `$effect`) for the exchange; union state
  `exchanging | confirming | error` (`view-state-union-machine`); add the auth
  method via the `wrapper-object-rune-store`; clear `_loadingMe`/cached user/orgs
  when replacing auth state. V1 durable expiry stays simple (no 1h warning; 401
  later signs out).

## Architectural choice

Mirror `MagicLinkExchange.svelte` (the shipped transitional token-exchange
screen) for the resume route, with a `confirming` state added for the
account-mismatch guard and a bare (un-authed) fetch for the exchange. A new
`auth.setAccessOnly` method handles durable adoption (access-only, no refresh).
Two stories split by surface: the shared auth method (Story A, focused tests on
auth semantics) and the route+screen (Story B).

## Implementation Units

### Unit 1: `auth.setAccessOnly` (durable access-only adoption)
**Story**: `epic-cli-browser-session-resume-spa-route-auth-access-only`
**File**: `frontend/src/lib/auth.svelte.ts` (+ `auth.test.ts`)

```ts
// On the wrapper-object rune store `auth`:
// setAccessOnly adopts a durable browser session that has NO refresh token
// (resume exchange returns access-only). Sets _token; CLEARS _refresh +
// localStorage refresh; clears cached current-user/orgs + _loadingMe so the
// next /me runs fresh as the adopted account.
setAccessOnly(access: string): void
```

**Acceptance**:
- [ ] `setAccessOnly` sets `auth.token`, and `auth.refresh` is null + the
      `jamsesh.refresh` localStorage key is removed.
- [ ] Any cached current-user/orgs + `_loadingMe` are cleared (next `/me` is fresh).
- [ ] `auth.token` persists to `jamsesh.token` (consistent with `setTokens`).

### Unit 2: resume routes + `ResumeExchange.svelte`
**Story**: `epic-cli-browser-session-resume-spa-route-route-screen`
**Files**: `frontend/src/lib/router.svelte.ts`, `frontend/src/lib/screens/ResumeExchange.svelte`,
`frontend/src/lib/App.svelte` (route→screen switch), `docs/UX.md` + `docs/SECURITY.md` (roll-forward)

- Routes (public, `requiresAuth:false`, mounted before `session-view`):
  `/playground/s/{sessionId}/resume` and `/orgs/{orgId}/sessions/{sessionId}/resume`.
- `ResumeExchange.svelte` (mirror MagicLinkExchange): `onMount` → read `#rt` from
  `location.hash`; missing → error. `history.replaceState(null,'',pathname+search)`
  to strip. Bare `fetch` POST `/api/session-resumes/exchange` `{resume_token}`,
  `credentials:'omit'`. On success: decide confirm (per the rules above);
  `confirming` → user accepts → adopt; declines → generic retry hint. Adopt:
  playground → mirror JoinerPicker's success handling; durable →
  `auth.setAccessOnly(bearer)`. Then `navigate` from the response. On
  failure/expired/used → generic error + retry hint (no oracle).
- Roll forward `docs/UX.md` (resume landing) + `docs/SECURITY.md` (client-side
  fragment-strip / bare-fetch / no-token-logging safeguards).

**Acceptance**:
- [ ] Bare fetch sends NO `Authorization` header; `#rt` stripped before any other
      nav/asset; token/bearer never logged or rendered.
- [ ] Success (no existing identity) adopts directly + navigates (playground via
      JoinerPicker's path; durable via `setAccessOnly`).
- [ ] Existing differing/unconfirmable identity → `confirming`; accept adopts,
      decline shows retry hint and does NOT persist the bearer.
- [ ] Expired/used/invalid token → generic error + retry hint (no oracle).
- [ ] Public route flags; navigation paths derived from the response.

## Implementation Order

1. Unit 1 (`…-auth-access-only`) — depends on: `[]`
2. Unit 2 (`…-route-screen`) — depends on: `[…-auth-access-only]`

## Testing

- Unit 1: `auth.test.ts` — setAccessOnly sets token, clears refresh +
  localStorage refresh + cached user/orgs/_loadingMe; token persisted.
- Unit 2: `ResumeExchange.test.ts` (jsdom; `spa-test-module-mock-barrel` +
  `window-location-defineproperty-stub`): bare-fetch-no-Authorization; `#rt`
  strip; success adopt+navigate (playground + durable); confirm accept/decline
  (decline → no bearer persisted + retry hint); generic error on bad token.

## Risks

- **Playground bearer wiring** — the single biggest integration risk; mitigated
  by mirroring the shipped JoinerPicker success path rather than reinventing.
- **Post-exchange confirm consumes the token** — decline can't be undone; the UX
  must make "run the command again" clear. Acceptable (single-use is the point).
