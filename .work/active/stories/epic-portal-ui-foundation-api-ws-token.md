---
id: epic-portal-ui-foundation-api-ws-token
kind: story
stage: review
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

## Implementation notes

### Files delivered

- `frontend/src/lib/api/client.ts` — `openapi-fetch` client with Bearer
  middleware reading `auth.token` at request time. Uses a `lateFetch`
  wrapper (`(...args) => globalThis.fetch(...args)`) so `vi.spyOn` works
  in tests (openapi-fetch captures `baseFetch` at `createClient()` time;
  the wrapper defers the lookup to call time). `baseUrl` is derived from
  `window.location.origin` so relative paths work in both production
  (same-origin) and jsdom tests (`http://localhost:3000`).
- `frontend/src/lib/auth.svelte.ts` — rune-based auth store using the
  wrapper-object pattern (Svelte 5 prohibits `export const x = $derived(…)`).
  Persists to localStorage under `jamsesh.token` / `jamsesh.refresh`.
  `loadCurrentUser` is a no-op TODO pending `epic-portal-foundation-accounts`.
- `frontend/src/lib/ws.svelte.ts` — WebSocket subscription manager. One
  socket per sessionId; subsequent subscribes reuse the connection.
  Authenticates via `Sec-WebSocket-Protocol: jamsesh.bearer.<token>`.
  `EventEnvelope` typed as `{ type: string; [key: string]: unknown }` locally;
  when `EventEnvelope` lands in the OpenAPI spec, swap the local alias for
  `components['schemas']['EventEnvelope']` — no API surface changes needed.

### Test notes

- 3 tests for `client.ts`, 7 for `auth.svelte.ts`, 11 for `ws.svelte.ts` = 21
  total, all green.
- `ws.test.ts` uses `vi.stubGlobal('WebSocket', MockWebSocket)` with a
  synchronous mock class that lets tests trigger `message`/`close` events
  directly.
- `auth.test.ts` uses `vi.doMock` + `vi.resetModules()` per test so rune
  state starts fresh; `navigate` from the router is mocked per test.

### Pre-existing issues (not in scope)

- `svelte-check` reports 21 errors in `src/lib/components/*.test.ts` (Badge,
  Button, Card, InlineCode) — all passing `children: () => 'string'` where
  Svelte 5 requires a `Snippet` value. These are sibling design-system story
  bugs. The `epic-portal-ui-design-system-tokens-and-components` review should
  address them before those tests are promoted to CI gates.
