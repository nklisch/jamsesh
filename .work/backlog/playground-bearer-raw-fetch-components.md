---
id: playground-bearer-raw-fetch-components
kind: story
stage: backlog
tags: [ui, bug, playground]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Session-view child components drop the anonymous playground bearer (raw fetch)

Discovered during the consumer-milestone review of
`epic-cli-browser-session-resume` (Codex xhigh). Pre-existing; affects the
playground JOIN flow too, not just resume.

`bearerMiddleware` (`frontend/src/lib/api/client.ts`) and the WS path
(`ws.svelte.ts`) now send `auth.playgroundContext.bearer` when there's no
durable `auth.token` (fixed in the resume epic — commit `1f25c353` + the
non-overwrite follow-up). But some session-view CHILD components issue **raw
`fetch` with `auth.token`** and bypass the shared client, so they still send NO
Authorization header for anonymous playground participants → 401:

- `frontend/src/lib/components/ArtifactPane.svelte` (~line 39)
- `frontend/src/lib/components/ForkDialog.svelte` (~line 89)

So a playground participant lands in the session view (core GET + WS now work),
but artifact loading / fork actions fail with 401.

Fix direction: route these through the shared `client` (so they inherit the
bearer fallback), or have them resolve the credential via
`auth.token ?? auth.playgroundContext?.bearer`. Audit ALL `fetch(`/`auth.token`
usages under `frontend/src/lib/{components,screens}/` for the same pattern and
fix comprehensively. Add tests asserting playground requests carry the bearer.

This is broader playground-SPA bearer-coverage debt, deliberately scoped OUT of
the resume epic (whose core path — gate + session GET + WS — is fixed). It
should be its own focused story.
