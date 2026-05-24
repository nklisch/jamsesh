---
id: story-tombstone-expired-redirect-distinguishes-active-vs-expired
kind: story
stage: done
tags: [ui, playground, ux]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-24
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

## Implementation discovery

**Design decision: client-side probe over server-side discriminator.**

The story offered two options. The server-side discriminator (adding a typed
error code to the 404 envelope) is the cleaner long-term approach and aligns
with the project's `deperr-translate-pipeline` pattern, but it requires
touching `docs/openapi.yaml` and coordinating with concurrent in-flight spec
work. Choosing it here would create merge conflicts and widen the blast radius
of a low-priority story.

The client-side probe is frontend-only and fully atomic: when the tombstone
endpoint returns 404, fire `HEAD /api/playground/sessions/{id}`. A 2xx response
means the session is still active and we redirect as before; anything else
(404, 401, or network error) means the record is gone and we render a terminal
"session summary has expired" page with a "Try another playground" CTA. The
probe is cheap because stale tombstone URLs are rare (only visited 30+ days
after a session ends).

**Files changed:**

- `frontend/src/lib/screens/SessionTombstone.svelte` — added `'expired'`
  ViewState, client-side HEAD probe on 404, and the terminal expired page UI
  with matching CSS.
- `frontend/src/lib/screens/SessionTombstone.test.ts` — split the existing
  404-redirect test into three cases (probe 200 → redirect, probe 404 →
  expired page, probe network error → expired page) and added a CTA navigation
  test for the expired page. 22 tests total, all passing.

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Implementation matches the design; verification passes (Go: `go build` + `go test ./...` clean; frontend: `npm run check` 0 errors, `npm run test` 635/635, `npm run build` clean). Implementation notes accurately document what landed, including any agent decisions or land-mode confirmations.
