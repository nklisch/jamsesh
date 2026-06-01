---
id: story-playground-anon-access-refresh-bounce
kind: story
stage: drafting
tags: [playground, ui, auth, bug]
parent: feature-playground-anon-session-access
depends_on: []
release_binding: null
gate_origin: null
created: 2026-06-01
updated: 2026-06-01
---

# Refreshing an anonymous playground session bounces to login

## Brief

Reloading the page during an anonymous playground session — or opening the
org-scoped URL `/orgs/org_playground/sessions/<id>` directly — drops the user to
the login screen, losing their live session. Terrible UX for a funnel whose
whole point is zero-friction try-it-now.

Root cause is client-side: the `anonymous_session_bearer` is not rehydrated on a
fresh page load at the `/orgs/...` route, so `GET /api/playground/sessions/{id}`
returns 401 and the SPA redirects to login. The bearer IS attached on the
canonical `/playground/s/<id>/...` URL (that route returns 200) — so the session
identity exists, the SPA just doesn't reattach it after a reload / on the
org-scoped path.

Observed live 2026-06-01 (session `01KT0M1JPAMMSEXAQQBSTZFD7D`, v0.5.0).

## Acceptance criteria

- Reloading a playground session page keeps the anonymous participant in the
  session (no login bounce) as long as the bearer is still valid.
- Loading the org-scoped session URL for a playground session rehydrates the
  anonymous bearer (or redirects to the canonical `/playground/s/<id>` URL)
  rather than dropping to login.
- When the bearer is genuinely expired/revoked, the user sees an appropriate
  "session ended/expired" state, not a generic login bounce.
