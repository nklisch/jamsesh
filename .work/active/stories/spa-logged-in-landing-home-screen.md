---
id: spa-logged-in-landing-home-screen
kind: story
stage: review
tags: [frontend, ui]
parent: spa-logged-in-landing-and-org-bootstrap
depends_on: [spa-logged-in-landing-auth-store-orgs-cache]
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
---

# Home screen + router wiring

## Scope

Add the `/` route and the new `Home.svelte` screen that renders the user's
org picker, the empty state for users with no orgs, and the inline
create-org form (POST /api/orgs). Auto-route to the single org when the
user has exactly one membership.

Visual reference: `.mockups/screens/spa-logged-in-landing-and-org-bootstrap/option-1.html`
(selected variant — centered card, quiet & literal). Match the layout,
spacing, and component composition.

See parent feature `## Implementation Units > Unit 2` for the full
specification (component shape, snippet structure, edge cases).

## Files

- `frontend/src/lib/screens/Home.svelte` (new)
- `frontend/src/lib/screens/Home.test.ts` (new)
- `frontend/src/lib/router.svelte.ts` (edit — add `/` route as the FIRST entry)
- `frontend/src/App.svelte` (edit — add `{:else if current.name === 'home'} <Home />` to the render chain)

## Acceptance Criteria

- [ ] Navigating to `/` triggers `current.name === 'home'` and renders
      `Home.svelte`.
- [ ] When `auth.orgs === null`, the screen renders a loading state
      with `aria-busy="true"` containing the text "Loading your workspaces".
- [ ] When `auth.orgs.length === 0`, the screen renders the empty-state
      heading + welcome paragraph + create-form, NO org list.
- [ ] When `auth.orgs.length === 1`, the screen navigates to
      `/orgs/{onlyId}/sessions` via a `$effect` and does not render the
      picker.
- [ ] When `auth.orgs.length >= 2`, the screen renders one `<li>` per
      org, each containing avatar + name + slug + role-badge, each
      clickable (and middle-clickable via real `<a href>`) to the org's
      session list.
- [ ] Role badges title-case the role string. The `creator` role uses
      a distinct class (`role-creator`) styled with the accent-muted
      color; other roles fall through to the neutral pill.
- [ ] Submitting a non-empty name calls `POST /api/orgs` with
      `{ name: <trimmed> }` exactly once per submit.
- [ ] On 201 response: `auth.addOrg` is called with the response data
      (id/name/slug, role inferred as `'creator'`), then `navigate` is
      called with `/orgs/{newId}/sessions`.
- [ ] On non-2xx or network error: `createError` is set, `createState`
      becomes `'create-error'`, the Create button is enabled again,
      the error message is rendered in an element with `role="alert"`.
- [ ] Empty / whitespace-only org names are rejected client-side — no
      fetch fires; `createState` stays `'idle'`.
- [ ] The Sign-out button calls `auth.signOut()`.
- [ ] `npm run check` and `npm run test` pass.

## Notes

- The auto-route latch `_autoRouted` is a plain `let`, NOT `$state` — see
  parent feature `Implementation Notes` under Unit 2 for rationale.
- The create-form is rendered as a `{#snippet}` block and used in both
  empty and picker states — same handler, same input binding.
- `<a href>` is used for org rows (not just `<button>`) for middle-click,
  open-in-new-tab, and accessibility. Click events `preventDefault` and
  call `navigate()` to keep SPA routing for normal clicks.
- The topbar is custom (mirroring the mock); do NOT use the existing
  `Chrome` component — `Chrome` is for org-scoped surfaces with a
  breadcrumb, and the Home screen has no org context yet. `Login` and
  `OAuthCallback` set the precedent of auth-area screens with custom
  chrome.
- Component reuse: `Card`, `Button`, `Input` from `frontend/src/lib/components/`
  per the existing patterns. The Login screen is the closest precedent.
- Match the tokens.css design tokens exactly — no hardcoded colors,
  spacings, or font sizes outside what tokens.css provides.

## Out of scope for this story

- No changes to `OAuthCallback.svelte`, `Login.svelte`, or the existing
  App.svelte auth-gate `$effect`. Unit 3 handles those.
- No e2e / playwright tests; unit tests in `Home.test.ts` only.
- No analytics, no telemetry, no toasts on create success — direct
  navigation is the feedback.

## Implementation notes

### Input.svelte extension (path chosen: extend the component)

Added `id?: string` to `Input.svelte`'s `$props()` and passed it through to
the underlying `<input>`. This is the preferred path: the `<label for="new-org-name">
<Input id="new-org-name">` association is a hard accessibility requirement, and
the three-line change is pattern-consistent with how `type`, `placeholder`, etc.
are already forwarded. Added two new test cases to `Input.test.ts` covering the
`id` attribute set/absent scenarios.

### Single-org auto-route template fix

The feature spec has the `{:else}` branch rendering the picker for `>= 1` orgs
(relying on the `$effect` to navigate away for the length === 1 case). During
testing, this caused a brief flash of "Pick a workspace" before the effect fired,
and made the AC assertion "does not render the picker" technically false at mount
time. The template was changed to `{:else if auth.orgs.length >= 2}` so the
single-org branch renders nothing (empty card) rather than a transient picker
heading. The `$effect` still fires and navigates immediately.

### `{#snippet}` cross-branch sharing

The `{#snippet createForm()}` block is declared after the `{/if}` and before
`</style>` (outside the main conditional tree). Both the empty-state and picker
branches call `{@render createForm()}`. Svelte 5 snippets are scoped to the
component, so cross-branch sharing works correctly — no design flaw here.

### Verification

- `npm run check`: 0 errors, 2 pre-existing warnings (unrelated files).
- `npm run test`: 447/447 passed (32 new Home tests + 2 new Input tests).
