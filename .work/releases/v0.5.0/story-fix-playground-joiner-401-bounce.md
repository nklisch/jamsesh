---
id: story-fix-playground-joiner-401-bounce
kind: story
stage: done
tags: [bug]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
---

# Playground joiner gets 401-bounced after a successful /join

Fixes GitHub issue nklisch/jamsesh#1.

## Symptom

A visitor opens a shared playground link (`/playground/s/<id>/join`), submits a
nickname, and `POST /api/playground/sessions/<id>/join` returns 200 with a fresh
anonymous bearer. The SPA navigates to `/orgs/org_playground/sessions/<id>` and
is immediately bounced back out — to `/login` (signed-out) or to the user's own
org dashboard (signed-in). The next API call (`GET /api/playground/sessions/<id>`)
returns 401. The CLI creator is unaffected because the creator drives the session
over the API/git directly, never through the browser SPA.

## Root cause

The SPA conflates "authenticated" with "holds an account access token" and never
consults `auth.playgroundContext` (the anonymous session bearer that
`JoinerPicker` stores after a successful join). Two independent bounce paths:

1. **`frontend/src/App.svelte` auth gate.** The `session-view` route
   (`/orgs/{orgId}/sessions/{sessionId}`) is `requiresAuth: true`. A signed-out
   joiner has `auth.isAuthenticated === false` (only a `playgroundContext`), so
   the gate's `$effect` navigates them to `/login` on arrival — before any API
   call fires.
2. **`frontend/src/lib/api/client.ts` `bearerMiddleware`.** It only ever attaches
   `auth.token`. For a signed-in user who joined a playground, the wrong account
   token is sent on `GET /api/playground/sessions/<id>`, the server returns 401
   `auth.not_a_member`, and `unauthorizedMiddleware` (which signs out on any
   `auth.*` 401) clears the session and redirects. For a signed-out joiner no
   token is sent at all.

Server-side enforcement (`internal/portal/playground/handler.go`) is correct;
this is purely a client identity-selection bug.

## Fix approach

- `client.ts`: add `bearerForRequest(pathname)` — when `auth.playgroundContext`
  is set and the request path is playground-scoped (`/api/playground/` or
  `/api/orgs/org_playground/`), attach the playground bearer; otherwise the
  account token. Keeps signed-in + playground states coexisting cleanly.
- `App.svelte`: the auth gate now treats a matching `playgroundContext`
  (`session-view` + `orgId === org_playground` + `playgroundContext.sessionId ===
  current.params.sessionId`) as satisfying the gate, so the joiner is not bounced.

## Regression test

- `frontend/src/lib/api/client.test.ts` — new `client — playground bearer
  selection` describe: playground-scoped requests carry the playground bearer
  (even when an account token coexists); non-playground and non-playground-org
  requests still carry the account token.
- `frontend/src/App.test.ts` — anonymous participant on their own playground
  session-view is not redirected; a participant whose `playgroundContext` is for
  a different session is still redirected to `/login`.

## Implementation notes

Files changed:
- `frontend/src/lib/api/client.ts` — added `PLAYGROUND_ORG_ID` const and
  `bearerForRequest(pathname)`; `bearerMiddleware` now selects the playground
  bearer for `/api/playground/*` and `/api/orgs/org_playground/*` paths, else
  the account token.
- `frontend/src/App.svelte` — added `PLAYGROUND_ORG_ID` const; the auth-gate
  `$effect` now exempts a matching playground session (`inOwnPlaygroundSession`)
  from the unauthenticated→/login bounce.

Tests added:
- `frontend/src/lib/api/client.test.ts` — `client — playground bearer selection`
  describe (5 cases).
- `frontend/src/App.test.ts` — 2 auth-gate cases; extended the auth mock with
  `playgroundContext`.

Verification: ran the full frontend suite in a `node:22` container (host has no
node runtime; this is the reference clone) — 788 tests passed (56 files),
`svelte-check` 0 errors / 0 warnings. Live browser reproduction was not run (no
runtime here); the two bounce paths are covered by the unit tests above, which
fail against the pre-fix code.

Adjacent issue parked (NOT bundled into the behavior fix): anonymous playground
participants still get no live WebSocket updates —
`.work/backlog/playground-anonymous-websocket-live-updates.md`.

Note: `npm ci` in the container created a spurious empty `frontend/.git`
(nested repo, no commits); removed it so the outer repo tracks the edits.

## Known gaps (from Opus review)

The bounce itself is fixed and the requests made through the shared
openapi-fetch `client` (tree, comments, activity, mode-switch, finalize plan)
now carry the playground bearer. NOT yet covered: two components issue raw
`fetch()` with `auth.token` directly and so still fail for anonymous joiners —
`ArtifactPane.svelte` (file-content load) and `ForkDialog.svelte` (refs probe +
`/mcp`). So a joiner reaches and stays in the session but cannot yet view file
artifacts or fork. Tracked in
`.work/backlog/playground-rawfetch-components-bypass-bearer.md`. The earlier
phrasing claiming "every playground-scoped request" is corrected by this note —
coverage is "every request that goes through the shared client".

## Review (2026-05-31)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Substrate fast-lane story review. Implementation notes include green
frontend verification (`788` tests passed across `56` files) plus `svelte-check`
with `0` errors / `0` warnings. Lens walk skipped per fast-lane story policy.
