---
id: epic-portal-ui-foundation-api-ws-token
kind: story
stage: implementing
tags: [ui]
parent: epic-portal-ui-foundation
depends_on: [epic-portal-ui-foundation-vite-svelte-routing]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# UI Foundation — API, WebSocket, Token Store

## Scope

Add the typed REST client (`openapi-fetch` over the generated
`types.gen.ts`), the WebSocket subscription primitive keyed by
session id, and the auth rune store that holds the OAuth token and
persists it to localStorage.

## Units delivered

- `frontend/src/lib/api/client.ts` — `openapi-fetch` client with
  Bearer-token middleware reading from the auth store
- `frontend/src/lib/ws.svelte.ts` — WebSocket subscription manager
  + typed `subscribe(sessionId, type, handler)` API; payload
  narrowing via `Extract<EventEnvelope, { type: T }>`
- `frontend/src/lib/auth.svelte.ts` — rune-based store with
  `token`, `refresh`, `currentUser`, `setTokens`, `signOut`,
  `loadCurrentUser`; persists to localStorage under `jamsesh.token`
  / `jamsesh.refresh`
- Unit tests for each

## Acceptance Criteria

- [ ] `client.GET('/api/whatever')` is type-checked at compile time
      against `paths` from `types.gen.ts` — currently empty paths
      means `client.GET` accepts any path string with `unknown`
      response; once paths land, calls narrow correctly
- [ ] Token-loading middleware attaches `Authorization: Bearer <token>`
      when `auth.token` is non-null; omits the header when null
- [ ] `auth.setTokens(access, refresh)` persists both to
      localStorage and updates the rune-derived `isAuthenticated`
- [ ] `auth.signOut()` clears localStorage and runes, then
      navigates to `/login`
- [ ] `ws.subscribe(sessionId, type, handler)` opens a single
      WebSocket per session (subsequent subscribes for the same
      sessionId reuse the connection), routes messages to handlers
      filtered by `type`, and returns an unsubscribe closure
- [ ] WebSocket connects with `Sec-WebSocket-Protocol: jamsesh.bearer.<token>`
      (the protocol the portal expects per `docs/research/core-go-server-stack.md`)
- [ ] Vitest green for all three modules (use jsdom env; mock
      `WebSocket` for ws.svelte.ts tests)

## Notes

- Until sibling REST features land paths in `docs/openapi.yaml`,
  `paths` and `EventEnvelope` resolve to mostly-empty types. The
  clients still compile cleanly — `client.GET<unknown>` accepts
  any string until paths populate.
- `auth.loadCurrentUser` calls `/api/me` which doesn't exist yet
  (lands in `epic-portal-foundation-accounts`). For v0 it's a
  no-op with a TODO comment pointing at the future endpoint; the
  Login screen and Chrome can still operate.
- Token refresh flow (using `auth.refresh` to fetch a new access
  token on 401) is OUT OF SCOPE for this story — it's a follow-up
  triage item once `/api/auth/refresh` exists.
