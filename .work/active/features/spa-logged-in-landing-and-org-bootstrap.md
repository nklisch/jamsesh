---
id: spa-logged-in-landing-and-org-bootstrap
kind: feature
stage: drafting
tags: [frontend, ui]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-19
updated: 2026-05-19
---

# SPA logged-in landing and org bootstrap

## Brief

The SPA in v0.2.0 has no path from "freshly-signed-up GitHub user" to any
useful screen. A user who finishes the OAuth handshake with no specific
`returnTo` and no org memberships gets bounced back to the login form they
just succeeded against. Every existing SPA route requires an `orgId` in the
URL, and there is no UI anywhere that creates a first org — even though the
backend endpoint to do so already exists.

This feature closes the four-way gap: it adds a logged-in landing route
(the SPA's first non-org-scoped page), wires authed-user redirects on the
login and OAuth-callback paths so freshly-authenticated users land there,
lists the user's org memberships on that landing, and ships a
create-first-org flow for users with zero memberships.

Scope is purely SPA wiring + two net-new screens. The backend already exposes
everything needed:

- `GET /api/me` returns the authenticated account's profile and the `orgs[]`
  array of memberships (operationId `getMe`, `docs/openapi.yaml:1634`).
- `POST /api/orgs` creates a new org with the caller as creator (operationId
  `createOrg`, `docs/openapi.yaml:1650`).

No foundation-doc roll-forward is required; this is a UX gap in the SPA,
not a contract or architectural shift.

## Strategic decisions

Locked at scope time so feature-design inherits the framing.

- **Home route path: `/`** — the SPA already treats `/` as a sentinel
  "default destination" string in `OAuthCallback.svelte` and `Login.svelte`,
  but no actual route exists there. Materializing `/` as the logged-in
  landing matches the existing fallback intent and avoids inventing a new
  path (`/home`, `/dashboard`) that would need its own redirects.
- **Listing source: `GET /api/me`, not a new endpoint** — the backend
  already returns `orgs[]` nested under the `MeResponse` schema. The SPA
  consumes the same `/api/me` call that `auth` already makes; no new API
  surface is needed. Feature-design can decide whether to reuse the
  existing auth store's `me` payload or refetch.
- **Org creation is in the SPA, not deferred to ops tooling** — the
  `POST /api/orgs` endpoint exists explicitly so end-users can bootstrap
  their first org without operator intervention. v1 of this feature ships
  the in-SPA flow rather than punting users to a CLI or admin UI.
- **Unauthenticated visitors to `/` keep going to login** — the new home
  route is auth-gated. The router's existing auth-guard middleware
  redirects unauthenticated callers to `/login?returnTo=/`. Out of scope
  to design a public marketing landing here.

## UI surface alignment

Net-new surfaces (logged-in landing, create-first-org). Tagged `[ui]`;
feature-design will invoke `/ux-ui-design:screens` per the
`ux-ui-principles` tier rule. Surfaces to mock:

- **Logged-in landing** (`/`) — variants for "has >=1 org" (list +
  create-another) and "has 0 orgs" (empty state with create CTA).
- **Create org** — inline form, modal, or its own route is a
  feature-design call; the screens skill should explore options.

## Open design questions (deferred to feature-design)

These are feature-design Phase 4.5 decisions, not strategic framing:

- When a user has exactly one org membership, does `/` auto-route to that
  org's session list, or always show the picker?
- Is the create-org form inline on `/`, a modal over `/`, or its own
  route (`/orgs/new`)?
- After creating an org, does the SPA navigate straight to that org's
  session list, or back to `/` with the new org highlighted?
- Does the org picker show role badges (owner/member/etc.) or just names?

## Known touchpoints

Anchor points for feature-design to design against — not a prescribed diff.

- `frontend/src/App.svelte` — main router; needs a `/` route and an
  authed-user redirect mirror for `/login`.
- `frontend/src/lib/screens/Login.svelte:47` — currently only redirects
  authed users when `returnTo` is set; needs to redirect to `/`
  unconditionally when authed.
- `frontend/src/lib/screens/OAuthCallback.svelte:49` — fallback target
  `navigate(returnTo ?? '/login')` should become
  `navigate(returnTo ?? '/')`.
- `frontend/src/lib/screens/` — home (`/`) renderer is net-new;
  create-org renderer is net-new.
- `frontend/src/lib/` — wherever the typed API client lives; consume the
  generated `getMe` and `createOrg` operations.

## Acceptance criteria (feature-level)

- [ ] A freshly-authenticated user with zero orgs lands on a screen that
      offers org creation, not the login form.
- [ ] A freshly-authenticated user with >=1 org lands on a screen that
      lists their memberships and lets them pick one (or auto-routes per
      the deferred design question).
- [ ] Hitting `/login` while already authenticated redirects to `/`
      regardless of `returnTo`.
- [ ] Hitting `/` while unauthenticated redirects to `/login?returnTo=/`.
- [ ] `OAuthCallback`'s fallback target is the new home route, not the
      login form.
- [ ] Org creation via the SPA actually calls `POST /api/orgs` and the
      created org appears in the user's list on next render.

<!-- Design and Implementation Notes accumulate as feature-design and
     downstream stories run. -->
