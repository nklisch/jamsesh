---
id: epic-cli-browser-session-resume-spa-route
kind: feature
stage: drafting
tags: [ui]
parent: epic-cli-browser-session-resume
depends_on: [epic-cli-browser-session-resume-portal-contract]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
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

## Foundation-doc roll-forward (at implementation)

`docs/UX.md` (the resume landing in the create/join journeys); `docs/SECURITY.md`
note on the fragment-strip / referrer-policy / same-origin-exchange client-side
safeguards (alongside the contract feature's server-side threat model).
