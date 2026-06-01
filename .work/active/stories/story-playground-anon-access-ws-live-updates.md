---
id: story-playground-anon-access-ws-live-updates
kind: story
stage: review
tags: [playground, ui, auth, websocket, bug]
parent: feature-playground-anon-session-access
depends_on: [story-playground-anon-access-refresh-bounce]
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-06-01
---

# Anonymous playground participants get no live WebSocket updates

## Idea
Surfaced while fixing the playground joiner 401 bounce
(`story-fix-playground-joiner-401-bounce`, GitHub #1). The bounce fix lets an
anonymous joiner reach and stay on the session view, but the WebSocket live
feed still does not connect for them, so tree/activity/comment events don't
stream in real time. Two causes in `frontend/src/lib/ws.svelte.ts`: `open()`
and `reopen()` both guard on `if (!auth.token)` and bail when there's no account
token, and the `POST /api/auth/ws-ticket` request is not playground-scoped, so
`bearerMiddleware` won't attach the anonymous `playgroundContext` bearer to it —
the ticket fetch fails / is issued for the wrong identity and the
`/ws/sessions/<id>` upgrade 403s. Fix needs the WS layer to use the playground
bearer (gate on `auth.token || auth.playgroundContext`, and make the ws-ticket
request carry the playground bearer for the active playground session). Larger
than the bounce fix; deferred deliberately to keep that fix minimal.

## Design

The WS layer should choose a bearer for the requested session, not for the
browser globally. If the active playground context matches the session ID, use
that anonymous bearer; otherwise fall back to the durable account token.

**Files**:
- `frontend/src/lib/ws.svelte.ts`
- `frontend/src/lib/session/usePlaygroundCountdown.svelte.ts`

```ts
function bearerForSession(sessionId: string): string | null {
  return auth.playgroundContext?.sessionId === sessionId
    ? auth.playgroundContext.bearer
    : auth.token;
}

async function fetchTicket(sessionId: string): Promise<string | null> {
  const bearer = bearerForSession(sessionId);
  if (!bearer) return null;

  const { data } = await client.POST('/api/auth/ws-ticket', {
    headers: { Authorization: `Bearer ${bearer}` },
  });
  return data?.ticket ?? null;
}
```

Implementation details:
- Change `fetchTicket()` to accept `sessionId`; update both `open()` and
  `reopen()` to call `fetchTicket(sessionId)`.
- Replace the `if (!auth.token)` guards with `if (!bearerForSession(sessionId))`
  so playground-only users can connect.
- Keep the existing ticket and subprotocol model. Do not put raw bearers in the
  `Sec-WebSocket-Protocol` header.
- No server change is required for issuing tickets: `POST /api/auth/ws-ticket`
  already issues a ticket for whichever account bearer middleware authenticated,
  and `/ws/sessions/:sessionID` already verifies session membership during the
  upgrade.

## Tests

- `frontend/src/lib/ws.test.ts`: playground-only context opens a socket and
  POSTs `/api/auth/ws-ticket` with `Authorization: Bearer <playground-bearer>`.
- `frontend/src/lib/ws.test.ts`: when a durable token and playground context
  coexist, a playground session uses the playground bearer and a durable session
  uses the account bearer.
- `frontend/src/lib/ws.test.ts`: reconnect obtains a fresh ticket with the same
  session-specific bearer choice.
- Existing replay cursor, teardown, reconnect suppression, and handler routing
  tests must remain green.

## Acceptance criteria

- Anonymous playground participants receive live commit/tree/comment/activity
  events without requiring a durable account token.
- The WS ticket request is playground-scoped by bearer choice, while the socket
  URL remains `/ws/sessions/:sessionID`.
- Expired/revoked playground bearers fail without redirecting to durable login.

## Implementation Notes

- Added `bearerForSession(sessionId)` to choose the matching playground bearer
  before falling back to the durable account token.
- Changed WS ticket fetches to pass an explicit `Authorization` header and
  updated both initial open and reconnect paths to use the session-specific
  bearer choice.
- Preserved the existing ticket/subprotocol model; raw bearers still never go
  into `Sec-WebSocket-Protocol`.
- Added playground-only WS coverage and updated durable ticket expectations.

## Verification

- `npm test -- --run src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/components/ArtifactPane.test.ts src/App.test.ts src/lib/screens/SessionViewShell.test.ts src/lib/ws.test.ts`
- `npm run check` (0 errors, 1 pre-existing Svelte warning in `ModeSwitchDialog.svelte`)
