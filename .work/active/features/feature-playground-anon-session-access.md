---
id: feature-playground-anon-session-access
kind: feature
stage: drafting
tags: [playground, auth]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-06-01
updated: 2026-06-01
---

# Anonymous playground participants as first-class session members

## Brief

Anonymous playground participants are added as `session_members` at join time
(`internal/portal/playground/handler.go:188,322` call `AddSessionMember`) but
are **never** `org_members` of the reserved `org_playground` org. As a result
they are second-class on every org-scoped session surface: the endpoints that
populate the session UI gate on org membership and reject them, and the SPA
doesn't reliably carry the anonymous bearer across navigations. The net effect
is a broken first impression for the playground — the exact funnel the
visitor-entry landing drives traffic into.

Three concrete defects, all symptoms of the same gap, observed live on v0.5.0
(session `01KT0M1JPAMMSEXAQQBSTZFD7D`, 2026-06-01):

1. **Empty file tree** — `GET /api/orgs/{orgID}/sessions/{id}/refs` and
   `/files` return `403 "not a member of this org"` because
   `ListSessionRefs` (`internal/portal/sessions/state.go:109`) and
   `ListSessionFiles` (`internal/portal/sessions/files.go:46`) call
   `GetOrgMember` before the session-membership check. The push lands and
   `base_sha` is set, but the file panel renders nothing. (Same gating in
   `refmodes.go:43`, `listing.go:38`, `invites.go:45`, `state.go:236`.)
2. **Refresh → login bounce** — on a fresh page load at the org-scoped URL
   `/orgs/org_playground/sessions/<id>` the `anonymous_session_bearer` isn't
   rehydrated, so `GET /api/playground/sessions/{id}` returns 401 and the SPA
   redirects to login. The bearer IS attached on the canonical
   `/playground/s/<id>/...` URL (that route returns 200).
3. **No live WebSocket updates** — the WS layer guards on `auth.token` and the
   `POST /api/auth/ws-ticket` request isn't playground-scoped, so the anonymous
   bearer never reaches the upgrade and `/ws/sessions/<id>` 403s.

## Strategic decisions

- **Fix direction for the server-side 403s** (org_members-vs-session-membership):
  deferred to feature-design. Two candidates, to be resolved with full code
  context during the design pass:
  - *Add anon as org_members* — at playground join also
    `AddOrgMember(org_playground, role=member)`. One change clears the refs,
    files, and ws-ticket 403s simultaneously, but grants anon broader
    org-scoped reach (listing, invites) that may need separate guarding.
  - *Endpoints accept session membership* — make the org-gated session
    endpoints fall back to session membership when `org_id` is the reserved
    playground org. More surgical but touches every handler and adds a
    playground special-case.
  Whichever is chosen determines how much collapses into a single change — the
  org_members route could resolve stories 1 and 3 server-side together, leaving
  only the frontend rehydration (story 2) and the WS-ticket scoping as separate
  work.

## Decomposition

Three child stories already exist (created at scope time); feature-design should
refine their bodies and sequencing rather than spawn new ones:

- `story-playground-anon-access-file-tree-403` — server-side authz (portal)
- `story-playground-anon-access-refresh-bounce` — SPA bearer rehydration (ui)
- `story-playground-anon-access-ws-live-updates` — playground-scoped WS ticket
  + bearer (ui), migrated from backlog
