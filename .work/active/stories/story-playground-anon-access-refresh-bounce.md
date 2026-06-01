---
id: story-playground-anon-access-refresh-bounce
kind: story
stage: review
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

## Design

Persist the playground anonymous context in browser-scoped `localStorage` keyed by
session ID. This changes the previous in-memory-only contract: a participant who
returns in the same browser while the playground session is still live should
land directly back in the session.

### Auth store unit

**File**: `frontend/src/lib/auth.svelte.ts`

```ts
export type PlaygroundContext = {
  sessionId: string;
  bearer: string;
  nickname: string;
  expiresAt: string;
};

type StoredPlaygroundContext = PlaygroundContext & {
  storedAt: string;
};

const PLAYGROUND_CONTEXT_KEY_PREFIX = 'jamsesh.playground.';

function playgroundContextKey(sessionId: string): string;
function loadStoredPlaygroundContext(sessionId: string): PlaygroundContext | null;
function persistPlaygroundContext(ctx: PlaygroundContext): void;
function clearStoredPlaygroundContext(sessionId?: string): void;
```

Add these methods to the wrapper-object rune store:

```ts
setPlaygroundContext(ctx: PlaygroundContext | null): void;
restorePlaygroundContext(sessionId: string): boolean;
clearPlaygroundContext(sessionId?: string): void;
```

Implementation details:
- `setPlaygroundContext(ctx)` writes both `_playgroundContext` and the
  browser-scoped record; `setPlaygroundContext(null)` clears the in-memory
  context and the matching stored record.
- `restorePlaygroundContext(sessionId)` synchronously reads the stored record,
  drops it if `expiresAt <= Date.now()`, updates `_playgroundContext`, and
  returns true only when a usable record was restored.
- Playground storage must not reuse `jamsesh.token` or `jamsesh.refresh`; those
  keys remain durable account auth only.

### Adoption call sites

**Files**:
- `frontend/src/lib/screens/JoinerPicker.svelte`
- `frontend/src/lib/screens/ResumeExchange.svelte`

Both successful playground adoption paths must pass `expiresAt`:

```ts
auth.setPlaygroundContext({
  sessionId: data.session.id,
  bearer: data.bearer,
  nickname: data.nickname,
  expiresAt: data.expires_at,
});
```

For resume exchange, use `body.expires_at` and `body.display_name`.

### Routing and load behavior

**Files**:
- `frontend/src/App.svelte`
- `frontend/src/lib/screens/SessionViewShell.svelte`
- `frontend/src/lib/api/client.ts`

`App.svelte` should restore before the protected-route decision:

```ts
const isPlaygroundSessionView =
  current.name === 'session-view' &&
  current.params.orgId === PLAYGROUND_ORG_ID;

if (isPlaygroundSessionView) {
  auth.restorePlaygroundContext(current.params.sessionId);
}
```

If the org-scoped playground route has no restored context, navigate to
`/playground/s/:sessionID/join` instead of `/login`. If a restored bearer is
later rejected by `GET /api/playground/sessions/{id}`, clear the stored context
and navigate to `/playground/s/:sessionID/ended`.

`client.ts` should attach the playground bearer only for the active playground
session:

```ts
function bearerForRequest(pathname: string): string | null {
  const pg = auth.playgroundContext;
  if (pg && isPathForPlaygroundSession(pathname, pg.sessionId)) {
    return pg.bearer;
  }
  return auth.token;
}
```

The 401 interceptor must not call `auth.signOut()` for requests that used the
playground bearer. It should clear playground context and let the view route to
join/ended states.

### Foundation docs

**Files**:
- `docs/SPEC.md`
- `docs/ARCHITECTURE.md`

Update the playground frontend auth contract from "in-memory only" to
"browser-scoped, session-keyed persistence until bearer expiry/session end".
The docs should still state that anonymous identities are separate from durable
OAuth accounts and never appear in `org_members`.

## Tests

- `frontend/src/lib/auth.test.ts`: stored context round-trip, expiry pruning,
  `restorePlaygroundContext`, `clearPlaygroundContext`, and coexistence with a
  durable token.
- `frontend/src/App.test.ts`: cold load of
  `/orgs/org_playground/sessions/:sessionID` restores the stored context before
  the auth gate; missing context routes to `/playground/s/:sessionID/join`, not
  `/login`.
- `frontend/src/lib/screens/SessionViewShell.test.ts`: playground 401 clears
  context and routes to ended state.
- `frontend/src/lib/api/client.test.ts`: active-session playground requests use
  the playground bearer; cross-session playground requests do not; playground
  401 does not trigger durable sign-out.

## Implementation Notes

- Added `expiresAt` to `PlaygroundContext` and browser-scoped localStorage
  persistence under `jamsesh.playground.<sessionId>`, with synchronous restore
  and expiry pruning.
- Updated join and resume adoption paths to persist the server `expires_at`.
- Restored playground context before the auth gate on org-scoped playground
  session routes; missing context now routes to `/playground/s/:id/join`.
- Playground summary 401s clear the stored context and route to
  `/playground/s/:id/ended` without durable account sign-out.
- Tightened client bearer selection to the active playground session and
  updated foundation docs from in-memory-only to browser-scoped persistence.

## Verification

- `npm test -- --run src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/components/ArtifactPane.test.ts src/App.test.ts src/lib/screens/SessionViewShell.test.ts src/lib/ws.test.ts`
- `npm run check` (0 errors, 1 pre-existing Svelte warning in `ModeSwitchDialog.svelte`)
