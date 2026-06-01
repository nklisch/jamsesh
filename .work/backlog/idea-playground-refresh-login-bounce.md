---
id: idea-playground-refresh-login-bounce
created: 2026-06-01
tags: [playground]
---

Refreshing the page in an anonymous playground session (or loading the
org-scoped URL `/orgs/org_playground/sessions/<id>`) bounces the user back to
the login screen — terrible UX, the participant loses their live session on a
reload. Root cause: the `anonymous_session_bearer` token is not rehydrated on a
fresh page load at the `/orgs/...` route, so `GET /api/playground/sessions/{id}`
returns 401 and the SPA redirects to login. The bearer IS attached on the
canonical `/playground/s/<id>/...` URL (that route returns 200), so the fix is
to persist/rehydrate the anonymous bearer across reloads and on the org-scoped
session URL. Observed live 2026-06-01 on session 01KT0M1JPAMMSEXAQQBSTZFD7D
(image v0.5.0). Related: the empty-file-tree bug where `refs`/`files` endpoints
return 403 for anonymous playground members because they gate on org
membership.
