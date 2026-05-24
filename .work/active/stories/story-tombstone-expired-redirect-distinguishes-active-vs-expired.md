---
id: story-tombstone-expired-redirect-distinguishes-active-vs-expired
kind: story
stage: implementing
tags: [ui, playground, ux]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Tombstone 404 should distinguish "session still active" vs "tombstone expired"

## Origin

Review of `story-epic-ephemeral-playground-portal-ui-anonymous-entry`
(SessionTombstone.svelte). The OpenAPI spec for
`GET /api/playground/sessions/{id}/tombstone` says 404 fires in two
distinct cases:

1. The session is **still active** (no tombstone written yet)
2. The tombstone **TTL has elapsed** (default 30 days)

The SPA currently redirects every 404 to the live session view at
`/orgs/org_playground/sessions/:id`. That works for case 1, but for
case 2 the user will hit a dead live-session route, get redirected by
the auth gate / receive a 401/404, and end up in a confusing loop.

## Suggested fix

Two options:

- **Server-side discriminator** — extend the 404 response envelope with
  an `error` code (`playground.session_active` vs
  `playground.tombstone_expired`) so the SPA can branch. This is the
  cleaner of the two and matches the project's typed-error-envelope
  pattern (`deperr-translate-pipeline`).
- **Client-side probe** — on 404, fire a HEAD/GET against
  `/api/playground/sessions/:id` to discriminate active vs gone. Cheap
  enough but adds a request to a rare path.

When the tombstone is expired, render a different terminal state — a
"This session existed but its summary has expired" page with a
"Try another playground" CTA — instead of redirecting.

## Scope hint

Small story. UI plus an OpenAPI tweak if the server-side discriminator
is chosen. Coordinate with the playground-session-lifecycle endpoint
owner.

## Priority

Low — only affects users visiting a 30+ day old tombstone URL. The
playground itself is ephemeral, so most users will never hit this.
File now so it's not lost.
