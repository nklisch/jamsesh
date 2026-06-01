---
id: story-playground-anon-access-file-tree-403
kind: story
stage: drafting
tags: [playground, portal, auth, bug]
parent: feature-playground-anon-session-access
depends_on: []
release_binding: null
gate_origin: null
created: 2026-06-01
updated: 2026-06-01
---

# Anonymous playground file tree renders nothing (org-gated endpoints 403)

## Brief

An anonymous playground participant sees an empty file panel even after a
successful push. The data is present — the bare repo has
`refs/heads/jam/<id>/{base,draft}` at the pushed commit and the session row has
`base_sha` set — but the endpoints that populate the file UI reject the
anonymous bearer with `403 "not a member of this org"`.

Root cause: anon participants are `session_members` but never `org_members` of
`org_playground` (playground join calls `AddSessionMember` only —
`internal/portal/playground/handler.go:188,322`), while the org-scoped session
handlers check `GetOrgMember` first:

- `ListSessionRefs` — `internal/portal/sessions/state.go:109`
- `ListSessionFiles` — `internal/portal/sessions/files.go:46`
- also `refmodes.go:43`, `listing.go:38`, `invites.go:45`, `state.go:236`
  (digest)

Observed live 2026-06-01 (session `01KT0M1JPAMMSEXAQQBSTZFD7D`, v0.5.0): on the
canonical `/playground/s/<id>/resume` URL the playground session GET returns 200
but the follow-up `/api/orgs/org_playground/.../refs` returns 403.

Fix direction (org_members vs. per-endpoint session-membership fallback) is the
feature-level strategic decision — see the parent feature. This story owns the
server-side authorization change and its tests, whichever direction is chosen.

## Acceptance criteria

- An anonymous playground participant can list refs and files for their own
  session; the file panel renders the pushed tree.
- The fix does not widen access for non-members or across orgs (a non-member
  anonymous account still cannot read another session's refs/files).
- Regression test covers the playground anon path through the chosen
  authorization point.
