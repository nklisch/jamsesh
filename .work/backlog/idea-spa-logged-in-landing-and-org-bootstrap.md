---
id: idea-spa-logged-in-landing-and-org-bootstrap
created: 2026-05-19
tags: [frontend, ux]
---

The SPA has no UI for a successfully authenticated user with no specific
destination and no orgs yet. Three concrete gaps observed in v0.2.0:
(1) `OAuthCallback.svelte:49` falls back to `navigate(returnTo ?? '/login')`,
sending freshly-authed users back to the login page instead of a logged-in
landing; (2) `Login.svelte:47` only redirects authed users when `returnTo`
is set, so a logged-in user hitting `/login` clean just sees the form again;
(3) the router only knows org-scoped routes (`/orgs/{orgId}/sessions`,
`/orgs/{orgId}/settings`, etc.) — there is no `/`, no "list my orgs" screen,
and no SPA-side first-org creation flow. `OrgSettings.svelte` and
`SessionList.svelte` exist but nothing onboards a fresh user into their
first org. Needs a logged-in landing route, an org-list/picker view, and
a create-first-org flow before this hole closes.
