---
id: epic-portal-ui-foundation
kind: feature
stage: drafting
tags: [ui]
parent: epic-portal-ui
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal UI — Foundation

## Brief

The plumbing every other portal-UI feature depends on. Establishes the Svelte 5
+ Vite project, routing, embedded-static-assets integration into the portal Go
binary, the OAuth + magic-link login UI (the visual surface only — the
backend handlers live in `epic-portal-foundation`), browser-side token
persistence, and the WebSocket client wrapper + reactive state primitives
(Svelte runes) that every consuming feature uses for live updates.

This feature delivers the login screen as its first concrete UI surface and
ships the empty session-list shell (without sessions populated — that lands
in `epic-portal-ui-session-list`). Once it's done, a user can sign in via
OAuth or magic-link and reach an empty post-login state.

Does NOT cover: the design system (`epic-portal-ui-design-system`), any
session-view surfaces (`epic-portal-ui-session-view-shell` and downstream).

## Epic context

- Parent epic: `epic-portal-ui`
- Position in epic: foundation feature — every other portal-UI feature
  depends on this for routing, the WebSocket primitive, and the post-login
  shell.

## Foundation references

- `docs/SPEC.md` — Stack > Portal frontend (Svelte 5 + Vite locked, embedded
  in Go binary)
- `docs/ARCHITECTURE.md` — Portal UI WebSocket gateway, session_id in MCP
  calls
- `docs/PROTOCOL.md` — REST API > Auth section, WebSocket event envelope
- `docs/SECURITY.md` — User authentication, Token lifetime and renewal
- `docs/UX.md` — Flow: joining a session
- `.mockups/design-system/tokens.css` — locked palette + typography
- `.mockups/flows/onboarding/02-sign-in.html` — locked login screen design

## Decomposition risks (carried from epic pre-mortem)

- Routing-library choice is locked here (svelte-spa-router / hand-rolled /
  hash-based) and affects every other feature. Pin during design.
- The WebSocket client wrapper here becomes the cross-cutting subscription
  pattern; every other feature uses it. Feature-design must lock the
  subscription API shape to avoid drift.

## Mockups

The login screen is the only UI surface in this feature; it's locked by
the onboarding flow rather than a per-feature `/screens` pass.

- Login screen: `.mockups/flows/onboarding/02-sign-in.html`
  - Centered card layout, OAuth button + magic-link inline form equally
    prominent (per the epic-design Phase 4.7 auth-UX lock)
  - "Resume strip" callout reminding the user which session they'll land
    in post-auth (when arriving via an invite link)
- Design tokens: `.mockups/design-system/tokens.css`
- Theme toggle behavior: `prefers-color-scheme` default, `[data-theme]`
  on `<html>` for explicit override (tokens.css already implements this)
- App chrome consistency: see the chrome treatment in the session-view
  options at `.mockups/screens/epic-portal-ui-session-view-shell/option-5.html`
  (wordmark, breadcrumb-like org chip, theme chip, avatar) — foundation
  must implement these as base components for downstream features to reuse.

## Generated-contracts scope

Per the SPEC.md generated-contracts decision, this feature establishes
the typed-client wrapper used by every other UI feature:

- A Vite-time codegen step (or pre-build script invoked by `make
  generate`) runs `openapi-typescript` against `docs/openapi.yaml` to
  produce `frontend/src/lib/api/types.gen.ts` (committed). This file is
  imported but never edited.
- A thin REST client wrapper at `frontend/src/lib/api/client.ts` uses
  `openapi-fetch` with the generated types to provide typed
  `client.GET`, `client.POST`, `client.PATCH`, etc. — endpoint paths
  and request/response bodies are checked against the spec at TS
  compile time.
- The WebSocket primitive (the cross-cutting subscription pattern this
  feature already owns) types incoming events against the
  `EventEnvelope` discriminated union from the generated types. A
  consumer subscribes to events filtered by `type` and receives the
  correctly-narrowed payload shape.

Auth-flow surfaces (login screen handlers) call the typed `client` for
the `/api/auth/*` endpoints. Token persistence and reactive state
primitives (Svelte runes wrapping the WebSocket subscription and the
typed REST client) sit on top of this generated foundation.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. Feature stays at
stage: drafting per --mocks-only pass. -->
