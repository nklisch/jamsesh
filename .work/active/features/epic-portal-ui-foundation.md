---
id: epic-portal-ui-foundation
kind: feature
stage: implementing
tags: [ui]
parent: epic-portal-ui
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal UI — Foundation

## Brief

The plumbing every other portal-UI feature depends on. Establishes the Svelte 5
+ Vite project, routing, embedded-static-assets integration into the portal Go
binary, the OAuth + magic-link login UI (the visual surface only — the
backend handlers live in `epic-portal-foundation`), browser-side token
persistence, and the WebSocket client wrapper + reactive state primitives
(Svelte runes) that every consuming feature uses for live updates.

This feature delivers the login screen as its first concrete UI surface and
ships the empty session-list shell (without sessions populated — that lands
in `epic-portal-ui-session-list`). Once it's done, a user can sign in via
OAuth or magic-link and reach an empty post-login state.

Does NOT cover: the design system (`epic-portal-ui-design-system`), any
session-view surfaces (`epic-portal-ui-session-view-shell` and downstream).

## Epic context

- Parent epic: `epic-portal-ui`
- Position in epic: foundation feature — every other portal-UI feature
  depends on this for routing, the WebSocket primitive, and the post-login
  shell.

## Foundation references

- `docs/SPEC.md` — Stack > Portal frontend (Svelte 5 + Vite locked, embedded
  in Go binary)
- `docs/ARCHITECTURE.md` — Portal UI WebSocket gateway, session_id in MCP
  calls
- `docs/PROTOCOL.md` — REST API > Auth section, WebSocket event envelope
- `docs/SECURITY.md` — User authentication, Token lifetime and renewal
- `docs/UX.md` — Flow: joining a session
- `.mockups/design-system/tokens.css` — locked palette + typography
- `.mockups/flows/onboarding/02-sign-in.html` — locked login screen design

## Decomposition risks (carried from epic pre-mortem)

- Routing-library choice is locked here (svelte-spa-router / hand-rolled /
  hash-based) and affects every other feature. Pin during design.
- The WebSocket client wrapper here becomes the cross-cutting subscription
  pattern; every other feature uses it. Feature-design must lock the
  subscription API shape to avoid drift.

## Mockups

The login screen is the only UI surface in this feature; it's locked by
the onboarding flow rather than a per-feature `/screens` pass.

- Login screen: `.mockups/flows/onboarding/02-sign-in.html`
  - Centered card layout, OAuth button + magic-link inline form equally
    prominent (per the epic-design Phase 4.7 auth-UX lock)
  - "Resume strip" callout reminding the user which session they'll land
    in post-auth (when arriving via an invite link)
- Design tokens: `.mockups/design-system/tokens.css`
- Theme toggle behavior: `prefers-color-scheme` default, `[data-theme]`
  on `<html>` for explicit override (tokens.css already implements this)
- App chrome consistency: see the chrome treatment in the session-view
  options at `.mockups/screens/epic-portal-ui-session-view-shell/option-5.html`
  (wordmark, breadcrumb-like org chip, theme chip, avatar) — foundation
  must implement these as base components for downstream features to reuse.

## Generated-contracts scope

Per the SPEC.md generated-contracts decision, this feature establishes
the typed-client wrapper used by every other UI feature:

- A Vite-time codegen step (or pre-build script invoked by `make
  generate`) runs `openapi-typescript` against `docs/openapi.yaml` to
  produce `frontend/src/lib/api/types.gen.ts` (committed). This file is
  imported but never edited.
- A thin REST client wrapper at `frontend/src/lib/api/client.ts` uses
  `openapi-fetch` with the generated types to provide typed
  `client.GET`, `client.POST`, `client.PATCH`, etc. — endpoint paths
  and request/response bodies are checked against the spec at TS
  compile time.
- The WebSocket primitive (the cross-cutting subscription pattern this
  feature already owns) types incoming events against the
  `EventEnvelope` discriminated union from the generated types. A
  consumer subscribes to events filtered by `type` and receives the
  correctly-narrowed payload shape.

Auth-flow surfaces (login screen handlers) call the typed `client` for
the `/api/auth/*` endpoints. Token persistence and reactive state
primitives (Svelte runes wrapping the WebSocket subscription and the
typed REST client) sit on top of this generated foundation.

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Vite + Svelte 5**: locked versions are `svelte@^5.0.0`,
  `vite@^5.0.0`, `@sveltejs/vite-plugin-svelte@^4.0.0`. TypeScript via
  `svelte-check@^4.0.0`. No SvelteKit per `docs/SPEC.md` (the portal
  UI is an authenticated SPA, no SSR / SEO surface).
- **Routing**: hand-rolled History-API router (~50 LoC). Routes are:
  `/login`, `/orgs/:orgId/sessions` (post-login landing — empty shell
  in this feature), `/orgs/:orgId/sessions/:sessionId` (placeholder,
  consumed later). No external routing dep — the surface is small
  enough that a single `route.svelte.ts` rune-based store is cleaner
  than a library.
- **API client**: `openapi-fetch@^0.13.0` (latest of the 0.x line,
  matching the `openapi-typescript@~7.13.0` pin in the existing
  `frontend/package.json`). Generated types from
  `frontend/src/lib/api/types.gen.ts` (already produced by
  `http-skeleton-openapi-bootstrap`).
- **WebSocket primitive**: a typed wrapper using the discriminated-
  union `EventEnvelope` from `types.gen.ts`. Reactive over a single
  global `$state` map keyed by `session_id`. Subscriptions filter by
  event `type` so consumers narrow payloads correctly.
- **Token persistence**: localStorage under `jamsesh.token` (access
  token), `jamsesh.refresh` (refresh token, longer TTL). A small
  `auth.svelte.ts` rune-based store exposes `currentUser`, `token`,
  `signIn`, `signOut`. Token is sent as `Authorization: Bearer <token>`
  on every REST call; the WebSocket client sends it via
  `Sec-WebSocket-Protocol: jamsesh.bearer.<token>` per the locked
  protocol decision in research docs.
- **Static-asset embedding**: Vite builds to `frontend/dist/`; the
  Go portal embeds via `//go:embed frontend/dist/*` in a small
  `internal/portal/assets/assets.go` and serves at `/` (catch-all
  route after the API/git/mcp/ws mounts). The router falls through
  to `index.html` for SPA history-API routing.
- **App chrome**: a top-bar with wordmark "jamsesh" (left), breadcrumb
  org chip + session chip (center), theme toggle + avatar (right).
  Implemented as a `<Chrome>` Svelte component composed of design-
  system primitives.
- **Login screen content**: matches `.mockups/flows/onboarding/02-sign-in.html`.
  OAuth and magic-link forms equally prominent on one card. The
  OAuth path opens a popup window (or full redirect) to
  `/api/auth/oauth/github/start`; the magic-link path posts to
  `/api/auth/magic-link/request` then displays a "check your inbox"
  state.
- **Story decomposition**: 3 stories with chained deps:
  1. `vite-svelte-routing` — Vite/Svelte/TS toolchain + History
     router + theme bootstrap call + app entry + Go embed wiring
  2. `api-ws-token` — openapi-fetch client + WebSocket primitive +
     auth rune store + token persistence
  3. `login-and-chrome` — Login screen + app chrome + empty
     session-list landing
  Order: 1 lands first (toolchain), 2 lands second (clients),
  3 lands third (consumer surfaces).

## Architectural choice

**Layered: Vite/Svelte 5 toolchain → typed clients → consumer
surfaces. Runes (`$state`, `$derived`, `$effect`) carry all reactive
state. No external state library (no Redux, no Zustand) — runes are
sufficient.**

Alternatives considered:

- **SvelteKit**: ruled out by `docs/SPEC.md` — the portal UI is an
  authenticated SPA with no SSR / SEO needs; SvelteKit adds adapter
  + server-side machinery for nothing.
- **External router (svelte-spa-router)**: the small route set (3
  routes for v0, growing slowly) doesn't justify a dep when ~50 LoC
  of History-API code handles it.
- **External state (Zustand / Pinia-style)**: Svelte 5 runes are
  the state layer; an external lib duplicates the surface.

## Implementation Units

### Unit 1: Vite + Svelte 5 toolchain

**Story**: `epic-portal-ui-foundation-vite-svelte-routing`

**Files**:
- `frontend/vite.config.ts` — Svelte plugin, TS path resolution,
  `build.outDir: 'dist'`, dev server proxy for `/api`, `/ws`,
  `/git`, `/mcp` to `http://localhost:8443` (the portal dev port)
- `frontend/svelte.config.js` — `vitePreprocess` for TS in Svelte
  files
- `frontend/tsconfig.json` — extend the existing minimal config
  with Svelte-specific `types` + `include` paths
- `frontend/package.json` — add `svelte`, `@sveltejs/vite-plugin-svelte`,
  `vite`, `svelte-check`, `vitest`, `@testing-library/svelte`,
  `@testing-library/jest-dom`, `openapi-fetch`
- `frontend/index.html` — root HTML with `<script src="/src/main.ts">`
- `frontend/src/main.ts` — entry point: imports `theme-bootstrap.ts`
  (from design-system) FIRST, then imports app.css (tokens), then
  mounts `App.svelte` to `#app`
- `frontend/src/App.svelte` — top-level component reading the
  router store and rendering `<Login>`, `<SessionsLanding>`, etc.
- `frontend/src/lib/router.svelte.ts` — History-API router as a
  Svelte 5 rune store. ~50 LoC.

### Unit 2: Router

**File**: `frontend/src/lib/router.svelte.ts`

```ts
// History-API router as a rune store. Routes are matched eagerly
// (first match wins). Programmatic navigation via `navigate()`.

type Route = { pattern: RegExp; name: string; params: string[] };

const routes: Route[] = [
  { pattern: /^\/login$/,                                   name: 'login',         params: [] },
  { pattern: /^\/orgs\/([^/]+)\/sessions$/,                 name: 'sessions',      params: ['orgId'] },
  { pattern: /^\/orgs\/([^/]+)\/sessions\/([^/]+)$/,        name: 'session-view',  params: ['orgId', 'sessionId'] },
];

function match(path: string) {
  for (const r of routes) {
    const m = r.pattern.exec(path);
    if (m) {
      const params: Record<string, string> = {};
      r.params.forEach((p, i) => { params[p] = decodeURIComponent(m[i + 1]); });
      return { name: r.name, params };
    }
  }
  return { name: 'not-found', params: {} };
}

let path = $state(typeof window !== 'undefined' ? window.location.pathname : '/');
export const current = $derived(match(path));

export function navigate(to: string) {
  if (typeof window === 'undefined') return;
  window.history.pushState({}, '', to);
  path = to;
}

if (typeof window !== 'undefined') {
  window.addEventListener('popstate', () => { path = window.location.pathname; });
}
```

### Unit 3: Go embed for static assets

**Files**:
- `internal/portal/assets/assets.go` — `//go:embed dist/*` and
  helper `Handler() http.Handler` returning an `http.FileServer`
  scoped to the embedded FS. Falls back to `index.html` for
  unmatched paths (SPA history-API).
- `cmd/portal/main.go` (edit) — wire `MountUI` hook on `router.Deps`
  to mount the assets handler at `/`. (The router currently has no
  catch-all; adding one means extending `router.Deps` with a
  `MountUI http.Handler` field — coordinate with story.)

```go
package assets

import (
    "embed"
    "io/fs"
    "net/http"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the embedded SPA. Falls back to index.html on
// not-found so the History API can resolve routes client-side.
func Handler() (http.Handler, error) {
    sub, err := fs.Sub(dist, "dist")
    if err != nil {
        return nil, err
    }
    fs := http.FileServer(http.FS(sub))
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Try the literal path first.
        if _, err := fs.Open(r.URL.Path); err == nil {
            fs.ServeHTTP(w, r)
            return
        }
        // Fall back to index.html for SPA routing.
        r.URL.Path = "/"
        fs.ServeHTTP(w, r)
    }), nil
}
```

Note: `frontend/dist/` won't exist until the first build, so the
`go:embed all:dist` directive needs the directory to exist. The
story's build step runs `npm run build` to produce it before `go
build`. CI does the same.

### Unit 4: openapi-fetch typed client

**File**: `frontend/src/lib/api/client.ts`
**Story**: `epic-portal-ui-foundation-api-ws-token`

```ts
import createClient from 'openapi-fetch';
import type { paths } from './types.gen';
import { auth } from '../auth.svelte';

export const client = createClient<paths>({
  baseUrl: '',  // same-origin
});

// Request interceptor: attach Bearer token from the auth store.
client.use({
  onRequest({ request }) {
    const token = auth.token;
    if (token) request.headers.set('Authorization', `Bearer ${token}`);
    return request;
  },
});
```

### Unit 5: WebSocket primitive

**File**: `frontend/src/lib/ws.svelte.ts`

```ts
// Reactive WebSocket subscription manager keyed by sessionId.
// Each subscription receives events of a specific type with the
// payload narrowed via the generated EventEnvelope union.

import type { components } from './api/types.gen';
import { auth } from './auth.svelte';

type EventEnvelope = components['schemas']['EventEnvelope'];
// `EventEnvelope` from types.gen.ts is a discriminated union; once
// types are populated by sibling features, narrowing on `event.type`
// gives correctly typed payloads. For now (empty paths), the union
// resolves to `never`, and type narrowing is a no-op.

type Handler<T extends EventEnvelope['type']> = (
  e: Extract<EventEnvelope, { type: T }>
) => void;

const sockets = new Map<string, WebSocket>();
const handlers = new Map<string, Map<string, Set<Handler<EventEnvelope['type']>>>>();

function open(sessionId: string) {
  if (sockets.has(sessionId)) return sockets.get(sessionId)!;
  const token = auth.token;
  if (!token) throw new Error('ws.svelte: no auth token');
  const proto = `jamsesh.bearer.${token}`;
  const ws = new WebSocket(`/ws/sessions/${sessionId}`, proto);
  ws.addEventListener('message', (ev) => {
    let env: EventEnvelope;
    try { env = JSON.parse(ev.data); } catch { return; }
    const byType = handlers.get(sessionId);
    const set = byType?.get(env.type);
    if (set) for (const h of set) h(env);
  });
  ws.addEventListener('close', () => { sockets.delete(sessionId); });
  sockets.set(sessionId, ws);
  return ws;
}

export function subscribe<T extends EventEnvelope['type']>(
  sessionId: string,
  type: T,
  handler: Handler<T>,
): () => void {
  open(sessionId);
  if (!handlers.has(sessionId)) handlers.set(sessionId, new Map());
  const byType = handlers.get(sessionId)!;
  if (!byType.has(type)) byType.set(type, new Set());
  byType.get(type)!.add(handler as Handler<EventEnvelope['type']>);
  return () => {
    byType.get(type)?.delete(handler as Handler<EventEnvelope['type']>);
  };
}

export function close(sessionId: string) {
  sockets.get(sessionId)?.close();
  sockets.delete(sessionId);
  handlers.delete(sessionId);
}
```

### Unit 6: Auth store

**File**: `frontend/src/lib/auth.svelte.ts`

```ts
import { client } from './api/client';
import { navigate } from './router.svelte';

const TOKEN = 'jamsesh.token';
const REFRESH = 'jamsesh.refresh';

let token = $state<string | null>(localStorage.getItem(TOKEN));
let refresh = $state<string | null>(localStorage.getItem(REFRESH));
let currentUser = $state<{ id: string; email: string; displayName: string } | null>(null);

export const auth = {
  get token() { return token; },
  get refresh() { return refresh; },
  get currentUser() { return currentUser; },
  get isAuthenticated() { return token !== null; },

  setTokens(access: string, refreshTok: string) {
    token = access;
    refresh = refreshTok;
    localStorage.setItem(TOKEN, access);
    localStorage.setItem(REFRESH, refreshTok);
  },

  signOut() {
    token = null;
    refresh = null;
    currentUser = null;
    localStorage.removeItem(TOKEN);
    localStorage.removeItem(REFRESH);
    navigate('/login');
  },

  async loadCurrentUser() {
    // Call /api/me — added by sibling features. Once paths.gen has
    // an entry for /api/me, this typed call narrows correctly. For
    // now this is a no-op.
    // const { data, error } = await client.GET('/api/me');
    // if (data) currentUser = data;
  },
};
```

### Unit 7: Login screen

**File**: `frontend/src/lib/screens/Login.svelte`
**Story**: `epic-portal-ui-foundation-login-and-chrome`

Matches `.mockups/flows/onboarding/02-sign-in.html`. Layout:
centered card; OAuth button + magic-link inline form equally
prominent. On OAuth click, opens
`/api/auth/oauth/github/start` (full redirect for v0; popup-flow
is a refinement). On magic-link submit, POSTs
`/api/auth/magic-link/request` with `{ email }`, then transitions
the card to a "check your inbox" state.

```svelte
<script lang="ts">
  import Button from '$lib/components/Button.svelte';
  import Input from '$lib/components/Input.svelte';
  import Card from '$lib/components/Card.svelte';
  import { client } from '$lib/api/client';

  let email = $state('');
  let mode = $state<'choose' | 'magic-link-sent' | 'magic-link-error'>('choose');
  let errorMsg = $state<string | null>(null);

  function signInWithGitHub() {
    window.location.assign('/api/auth/oauth/github/start');
  }

  async function requestMagicLink(e: Event) {
    e.preventDefault();
    errorMsg = null;
    // Once `/api/auth/magic-link/request` is in openapi.yaml, this
    // is typed end-to-end. Until then, a raw fetch.
    const res = await fetch('/api/auth/magic-link/request', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email }),
    });
    if (res.ok) {
      mode = 'magic-link-sent';
    } else {
      mode = 'magic-link-error';
      errorMsg = 'Could not send magic link. Try again.';
    }
  }
</script>

<div class="login-wrap">
  <Card padding="lg">
    {#if mode === 'choose'}
      <h1>Sign in to jamsesh</h1>
      <Button variant="primary" onclick={signInWithGitHub}>
        {#snippet children()}Continue with GitHub{/snippet}
      </Button>
      <div class="divider">or</div>
      <form onsubmit={requestMagicLink}>
        <Input type="email" bind:value={email} placeholder="you@example.com" />
        <Button variant="accent" type="submit">
          {#snippet children()}Email me a magic link{/snippet}
        </Button>
      </form>
    {:else if mode === 'magic-link-sent'}
      <h1>Check your inbox</h1>
      <p>We sent a link to {email}. Click it to finish signing in.</p>
    {:else}
      <h1>Something went wrong</h1>
      <p>{errorMsg}</p>
      <Button variant="ghost" onclick={() => mode = 'choose'}>
        {#snippet children()}Try again{/snippet}
      </Button>
    {/if}
  </Card>
</div>

<style>
  .login-wrap {
    min-height: 100vh;
    display: grid;
    place-items: center;
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
  }
  h1 { font-size: var(--font-size-2xl); margin: 0 0 var(--space-4); }
  .divider {
    text-align: center;
    color: var(--color-text-tertiary);
    margin: var(--space-3) 0;
    font-size: var(--font-size-sm);
  }
  form { display: flex; flex-direction: column; gap: var(--space-3); }
</style>
```

### Unit 8: App chrome

**File**: `frontend/src/lib/components/Chrome.svelte`

Top-bar: wordmark, org+session breadcrumb chips, theme toggle,
avatar. Receives `children` snippet for the page body.

```svelte
<script lang="ts">
  import ThemeToggle from './ThemeToggle.svelte';
  import AuthorDot from './AuthorDot.svelte';
  import { auth } from '$lib/auth.svelte';

  let { orgChip, sessionChip, children }: {
    orgChip?: string;
    sessionChip?: string;
    children: import('svelte').Snippet;
  } = $props();
</script>

<div class="chrome">
  <header class="topbar">
    <div class="left">
      <span class="wordmark">jamsesh</span>
      {#if orgChip}<span class="chip">{orgChip}</span>{/if}
      {#if sessionChip}<span class="chip">{sessionChip}</span>{/if}
    </div>
    <div class="right">
      <ThemeToggle />
      {#if auth.currentUser}
        <AuthorDot authorId={auth.currentUser.id} size={20} title={auth.currentUser.displayName} />
      {/if}
    </div>
  </header>
  <main>{@render children()}</main>
</div>

<style>
  .chrome { min-height: 100vh; display: flex; flex-direction: column; background: var(--color-bg-primary); color: var(--color-text-primary); }
  .topbar { display: flex; justify-content: space-between; align-items: center; padding: var(--space-3) var(--space-6); border-bottom: 1px solid var(--color-border); }
  .left, .right { display: flex; align-items: center; gap: var(--space-3); }
  .wordmark { font-family: var(--font-mono); font-weight: var(--font-weight-semibold); }
  .chip { font-family: var(--font-mono); font-size: var(--font-size-xs); padding: 2px var(--space-2); background: var(--color-bg-tertiary); border-radius: var(--radius-full); }
  main { flex: 1; padding: var(--space-6); }
</style>
```

### Unit 9: Empty session-list shell

**File**: `frontend/src/lib/screens/SessionsLanding.svelte`

The empty post-login state. Real session listing lands in
`epic-portal-ui-session-list`.

```svelte
<script lang="ts">
  import Chrome from '$lib/components/Chrome.svelte';
  import { auth } from '$lib/auth.svelte';
</script>

<Chrome orgChip="default-org">
  {#snippet children()}
    <h1>Sessions</h1>
    <p>No sessions yet. (Listing lands in epic-portal-ui-session-list.)</p>
    <button onclick={() => auth.signOut()}>Sign out</button>
  {/snippet}
</Chrome>
```

## Implementation Order

1. **vite-svelte-routing** — toolchain + router + entry + Go embed
   wiring. depends_on: []
2. **api-ws-token** — typed clients + auth store. depends_on:
   [vite-svelte-routing]
3. **login-and-chrome** — Login + Chrome + SessionsLanding +
   App.svelte. depends_on: [vite-svelte-routing, api-ws-token,
   epic-portal-ui-design-system-tokens-and-components]

## Testing

- Unit tests via Vitest for `router.svelte.ts`, `ws.svelte.ts`,
  `auth.svelte.ts`
- Component tests via `@testing-library/svelte` for Login,
  Chrome, SessionsLanding
- A smoke test that builds the full SPA (`npm run build`) and
  inspects `frontend/dist/index.html` exists

## Risks

- **Empty openapi.yaml means generated `EventEnvelope` and `paths`
  are empty / `never`.** The typed clients are scaffolded but
  effectively un-typed until sibling features populate the spec.
  Mitigation: ws.svelte.ts and client.ts compile fine against
  `never`; consumers code defensively. Once paths land, type
  narrowing activates with no API changes.
- **Vite + Svelte 5 ecosystem drift.** Svelte 5 stable shipped
  recently; some libraries lag. Mitigation: only `vite-plugin-svelte`
  (official), `vitest`, `@testing-library/svelte` (Svelte-5 path),
  and `openapi-fetch` are pinned. All have Svelte 5 support.
- **`go:embed` requires `frontend/dist/` to exist at build time.**
  Mitigation: the Makefile / CI orders `npm run build` before
  `go build ./cmd/portal`. The story bodies make this explicit;
  the release workflow already runs `go build` after checkout —
  add a pre-step.
