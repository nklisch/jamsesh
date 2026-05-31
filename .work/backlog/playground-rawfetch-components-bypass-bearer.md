---
id: playground-rawfetch-components-bypass-bearer
kind: story
stage: backlog
tags: [frontend, playground, bug, auth]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Anonymous playground: ArtifactPane and ForkDialog bypass the bearer middleware

## Idea
Found by the Opus review of `story-fix-playground-joiner-401-bounce` (issue #1).
The bounce fix routes the shared openapi-fetch `client` through
`bearerForRequest`, which covers TreeDag, CommentsTab, CommentComposer,
ModeSwitchDialog, and the finalize plan (all call `/api/orgs/org_playground/*`
via the client). But two components use a **raw `fetch()`** with a hardcoded
`Authorization: Bearer ${auth.token}` and so never see the playground bearer:

- `frontend/src/lib/components/ArtifactPane.svelte:35-41` — file-content load
  (`/api/orgs/{orgId}/sessions/{sessionId}/files`). Rendered unconditionally in
  `SessionViewShell`, so a joiner who clears the bounce still gets a 401 the
  moment they select a commit/file.
- `frontend/src/lib/components/ForkDialog.svelte:49-51,78-86` — the refs probe
  and the `/mcp` POST. Fork is unusable for an anonymous joiner. (Separately,
  `orgIdFromRef` appears to yield `''`, producing `/api/orgs//sessions/...` — a
  pre-existing bug worth checking while here.)

Fix: route both through the shared `client` (typed paths exist, e.g.
`GET /api/orgs/{orgID}/sessions/{sessionID}/files`) so `bearerForRequest`
selects the playground bearer — or apply the same selection inline. Add tests
mirroring the playground-bearer cases in `client.test.ts`. Same root class as
the WS gap (`playground-anonymous-websocket-live-updates`); could be tackled
together as "make every playground-scoped request use the anon bearer."
