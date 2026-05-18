---
id: epic-portal-ui-foundation-login-and-chrome
kind: story
stage: done
tags: [ui]
parent: epic-portal-ui-foundation
depends_on:
  - epic-portal-ui-foundation-vite-svelte-routing
  - epic-portal-ui-foundation-api-ws-token
  - epic-portal-ui-design-system-tokens-and-components
release_binding: v0.1.0
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

## Implementation notes

### Files created

- `frontend/src/lib/screens/Login.svelte` — matches
  `.mockups/flows/onboarding/02-sign-in.html` pixel-faithfully.
  Centered card; `method-block` sections for OAuth (GitHub button with
  SVG) and magic-link (inline form); "or" divider using flexbox
  pseudo-elements; resume-strip callout when `?resume=<name>` query
  param is present. Three states: `choose`, `magic-link-sent`,
  `magic-link-error`. Uses raw `fetch` for magic-link POST (defer
  comment to `epic-portal-foundation-auth-flows`).
- `frontend/src/lib/components/Chrome.svelte` — top-bar layout from
  option-5 mock. Wordmark with accent dot, breadcrumb chip(s) with
  "/" separator, ThemeToggle always, AuthorDot conditional on
  `auth.currentUser`. Body rendered via `{@render children()}`.
  `ChromeTestHarness.svelte` companion provides the snippet for tests.
- `frontend/src/lib/screens/SessionsLanding.svelte` — wraps Chrome
  with `orgChip="default-org"`, placeholder text, and a sign-out Button.
- `frontend/src/lib/screens/NotFound.svelte` — centered 404 card with
  link to `/login` using the router's `navigate()`.
- `frontend/src/App.svelte` (edited) — full route→screen mapping,
  auth gate via `$effect` redirecting unauthenticated non-login routes
  to `/login`.

### Tests

- All 28 new tests green (`vitest run`).
- Snippet tests use `ChromeTestHarness.svelte` companion pattern —
  NOT the `children: () => 'string'` sibling bug (those are pre-existing
  failures in Badge, Card, Button, InlineCode tests, filed as finding for
  design-system story review).
- `svelte-check` clean on new files (0 warnings, 0 errors from our code).
- `npm run build` green — dist built in ~1s.

### Deferred

- `/api/auth/magic-link/request` typed client call (raw fetch for now).
  Activates when `epic-portal-foundation-auth-flows` adds the endpoint
  to `openapi.yaml`.
- `?resume=<name>` query-param extraction in Login: works at load time;
  reactive routing support for in-flight invite links lands in
  `epic-portal-ui-session-view-shell`.

### Pre-existing failures (not introduced here)

21 `svelte-check` type errors and corresponding test failures in
`Badge.test.ts`, `Button.test.ts`, `Card.test.ts`, `InlineCode.test.ts`
— all use `children: () => 'string'` (incompatible with Svelte 5
Snippet type). Filed as a finding for the design-system story's review.

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Pixel-faithful Login translation from the mock. Chrome composition via design-system primitives is clean. Auth gate via \$effect redirects unauthenticated routes correctly. ChromeTestHarness.svelte demonstrates the correct Svelte 5 Snippet testing pattern — the bug-fix follow-up should use this same pattern.
