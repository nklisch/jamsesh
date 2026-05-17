---
id: epic-portal-ui-foundation-login-and-chrome
kind: story
stage: implementing
tags: [ui]
parent: epic-portal-ui-foundation
depends_on:
  - epic-portal-ui-foundation-vite-svelte-routing
  - epic-portal-ui-foundation-api-ws-token
  - epic-portal-ui-design-system-tokens-and-components
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# UI Foundation — Login Screen and App Chrome

## Scope

Ship the first concrete UI surfaces: the login screen (OAuth +
magic-link), the app Chrome (wordmark + breadcrumb chips + theme
toggle + avatar), and the empty SessionsLanding placeholder that
the chrome wraps.

## Units delivered

- `frontend/src/lib/screens/Login.svelte` — login card matching
  `.mockups/flows/onboarding/02-sign-in.html`; OAuth button + magic
  link form; "check your inbox" state
- `frontend/src/lib/components/Chrome.svelte` — top-bar + body
  layout consumed by post-login screens
- `frontend/src/lib/screens/SessionsLanding.svelte` — empty stub
  wrapped in Chrome (real listing lands in
  `epic-portal-ui-session-list`)
- `frontend/src/App.svelte` (edit) — wire route → screen mapping:
  `/login` → Login, `/orgs/:orgId/sessions` → SessionsLanding,
  others → Not Found placeholder
- Tests: Login mode transitions, Chrome renders auth user,
  SessionsLanding wraps Chrome

## Acceptance Criteria

- [ ] Login screen renders correctly against the mock at
      `.mockups/flows/onboarding/02-sign-in.html` — centered card,
      OAuth + magic-link equally prominent, "or" divider
- [ ] OAuth button triggers `window.location.assign('/api/auth/oauth/github/start')`
- [ ] Magic-link form POSTs to `/api/auth/magic-link/request` and
      transitions to the "check your inbox" state on 200
- [ ] On non-2xx response, login transitions to error state with
      a "Try again" affordance
- [ ] Chrome renders wordmark + theme toggle always; org chip +
      session chip when provided; avatar (via AuthorDot from the
      design system) when `auth.currentUser` is set
- [ ] SessionsLanding wraps in Chrome with `orgChip="default-org"`
      and shows the placeholder text + sign-out button
- [ ] Sign-out clears auth and navigates to `/login`
- [ ] `vitest` green for all components

## Notes

- The `/api/auth/oauth/github/start` and
  `/api/auth/magic-link/request` endpoints don't exist yet — they
  land with `epic-portal-foundation-auth-flows`. Login's
  client-side logic is correct; the backend just isn't there to
  answer until the auth-flows feature ships. This is the expected
  late-binding sequencing.
- Design-system components used: `Button`, `Input`, `Card`,
  `AuthorDot`, `ThemeToggle`. Verify they're available at the
  expected paths before implementation.
- The "Not Found" route handler should render a simple "Page not
  found" message with a link back to the appropriate landing — keep
  it tiny; full UX comes later.
