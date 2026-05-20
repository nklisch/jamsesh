---
id: spa-logged-in-landing-and-org-bootstrap
kind: feature
stage: implementing
tags: [frontend, ui]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-19
updated: 2026-05-20
---

# SPA logged-in landing and org bootstrap

## Brief

The SPA in v0.2.0 has no path from "freshly-signed-up GitHub user" to any
useful screen. A user who finishes the OAuth handshake with no specific
`returnTo` and no org memberships gets bounced back to the login form they
just succeeded against. Every existing SPA route requires an `orgId` in the
URL, and there is no UI anywhere that creates a first org — even though the
backend endpoint to do so already exists.

This feature closes the four-way gap: it adds a logged-in landing route
(the SPA's first non-org-scoped page), wires authed-user redirects on the
login and OAuth-callback paths so freshly-authenticated users land there,
lists the user's org memberships on that landing, and ships a
create-first-org flow for users with zero memberships.

Scope is purely SPA wiring + two net-new screens. The backend already exposes
everything needed:

- `GET /api/me` returns the authenticated account's profile and the `orgs[]`
  array of memberships (operationId `getMe`, `docs/openapi.yaml:1634`).
- `POST /api/orgs` creates a new org with the caller as creator (operationId
  `createOrg`, `docs/openapi.yaml:1650`).

No foundation-doc roll-forward is required; this is a UX gap in the SPA,
not a contract or architectural shift.

## Strategic decisions

Locked at scope time so feature-design inherits the framing.

- **Home route path: `/`** — the SPA already treats `/` as a sentinel
  "default destination" string in `OAuthCallback.svelte` and `Login.svelte`,
  but no actual route exists there. Materializing `/` as the logged-in
  landing matches the existing fallback intent and avoids inventing a new
  path (`/home`, `/dashboard`) that would need its own redirects.
- **Listing source: `GET /api/me`, not a new endpoint** — the backend
  already returns `orgs[]` nested under the `MeResponse` schema. The SPA
  consumes the same `/api/me` call that `auth` already makes; no new API
  surface is needed. Feature-design can decide whether to reuse the
  existing auth store's `me` payload or refetch.
- **Org creation is in the SPA, not deferred to ops tooling** — the
  `POST /api/orgs` endpoint exists explicitly so end-users can bootstrap
  their first org without operator intervention. v1 of this feature ships
  the in-SPA flow rather than punting users to a CLI or admin UI.
- **Unauthenticated visitors to `/` keep going to login** — the new home
  route is auth-gated. The router's existing auth-guard middleware
  redirects unauthenticated callers to `/login?returnTo=/`. Out of scope
  to design a public marketing landing here.

## UI surface alignment

Net-new surface: one screen at `/` that renders both empty (0 orgs) and
picker (>=1 org) states. Create-org lives inline on the same screen — see
`## Design decisions` below.

## Mockups

- Screens: [`.mockups/screens/spa-logged-in-landing-and-org-bootstrap/index.html`](../../../.mockups/screens/spa-logged-in-landing-and-org-bootstrap/index.html)
- **Selected**: `option-1` (Centered card, quiet & literal) — 2026-05-20
- Rationale: mirrors the `Login.svelte` centered-card pattern so the
  logged-in landing feels like a natural continuation of sign-in; smallest
  surface to ship; one card holds both states.

## Design decisions

Resolved at feature-design time.

- **Single-org UX: auto-route**. When the user navigates to `/` and
  `orgs.length === 1`, the Home screen calls `navigate('/orgs/{onlyOrgId}/sessions')`
  immediately on data-load. For 0 orgs render the empty state; for 2+
  render the picker. The "show picker after creating another" nuance from
  the user's answer is naturally satisfied because creating an org goes
  from N to N+1 and post-create lands the user directly in the new org —
  they never bounce back through `/` to be auto-routed away.
- **Create-org placement: inline on the home screen**. Both empty state
  (0 orgs) and picker state (>=1 org) render the create-form in the same
  card. No modal, no `/orgs/new` route. The form is a single `name` input
  + Create button; slug is derived server-side per `CreateOrgBody`
  (`maxLength: 200`, no client-side slug preview needed).
- **Post-create nav: direct into the new org's session list**. The
  Home screen's create handler calls `client.POST('/api/orgs', ...)`,
  on `201` reads `OrgRef.id`, appends the new org to `auth.orgs`,
  navigates to `/orgs/{newOrgId}/sessions`.
- **Role badges in the picker: yes**. The mock includes them; `MeOrgMembership.role`
  is required by the schema so the data is always present. Visual hierarchy:
  `creator` uses the accent-muted pill style; non-creator roles use the
  neutral pill. The label is the raw role string title-cased
  (`'creator'` -> `'Creator'`).
- **Org-cache source-of-truth: extend the `auth` store**. `auth.svelte.ts`
  already calls `GET /api/me` via `loadCurrentUser()` but discards the
  `orgs[]` array. We extend it to cache the full `MeResponse` shape
  including `orgs`, plus an `addOrg(org)` mutator for the create flow
  to push a new entry without a second `/api/me` round-trip. Rationale:
  the chrome's existing `auth.currentUser` consumer and the new Home
  screen are reading the same backend payload — one store, one fetch.
- **Loading state: tri-state `orgs: MeOrgMembership[] | null`**.
  `null` means "not yet loaded"; the Home screen renders a spinner
  while null. After `loadCurrentUser()` resolves, `orgs` is always an
  array (possibly empty). No separate `error` state in v1 — the existing
  `unauthorizedMiddleware` already covers 401 by signing the user out;
  other transient failures leave `orgs` null and the spinner stays.
  Acceptable v1 trade — failures here are rare.
- **Where `loadCurrentUser()` is triggered**. Two callers, both idempotent:
  1. `App.svelte` `$effect` — runs when `auth.isAuthenticated` flips to
     true AND `auth.orgs === null`. Covers cold-load (user opens the SPA
     with tokens already in localStorage) and any future client-side
     token-issue path.
  2. `OAuthCallback.svelte` — `await auth.loadCurrentUser()` BEFORE
     navigating, so the "Signing you in..." view encompasses the
     `/api/me` round-trip. Eliminates a spinner-flash on the Home screen
     after fresh OAuth. The guard inside `loadCurrentUser()` makes the
     subsequent App.svelte effect a no-op.
- **`loadCurrentUser()` idempotency**. Add a private `_loading: boolean`
  guard and a `currentUser !== null` short-circuit. Concurrent callers
  await one in-flight promise; already-loaded state returns immediately.

## Architectural choice

**Auth-store-as-orgs-cache (Approach B)** — extend `auth.svelte.ts`
to cache the full `MeResponse` (currently partially captured) and
expose `orgs` reactively. The Home screen reads from `auth.orgs`;
the create-flow mutates via `auth.addOrg(org)`.

Considered and rejected:

- **Approach A: Home does its own `/api/me` fetch on mount.** Smallest
  diff, but produces a redundant `/api/me` call (one from auth, one
  from Home) and provides no path to share orgs with the existing
  chrome consumers reading `auth.currentUser`. Splits ownership of
  one payload across two screens.
- **Approach C: New `account.svelte.ts` store.** Cleanest separation
  (auth = tokens, account = identity + orgs), but introduces a third
  module for data that's fetched in one call from one endpoint. Premature
  splitting; if `auth.svelte.ts` grows past comfort later, refactor then.

The choice keeps the store shape close to the `MeResponse` schema and
gives downstream consumers (Chrome's AuthorDot, future org-switcher in
session views) one place to read user-scoped data.

## Implementation Units

### Unit 1: Auth store extension + bootstrap effect

**Story**: `spa-logged-in-landing-auth-store-orgs-cache`
**Files**:
- `frontend/src/lib/auth.svelte.ts` (edit)
- `frontend/src/lib/auth.test.ts` (edit — extend coverage)
- `frontend/src/App.svelte` (edit — add bootstrap `$effect`)

**`auth.svelte.ts` shape:**

```typescript
import type { components } from '$lib/api/types.gen';
import { navigate } from '$lib/router.svelte';
import { client } from '$lib/api/client';

type MeOrgMembership = components['schemas']['MeOrgMembership'];

const TOKEN_KEY = 'jamsesh.token';
const REFRESH_KEY = 'jamsesh.refresh';

let _token = $state<string | null>(/* unchanged */);
let _refresh = $state<string | null>(/* unchanged */);
let _currentUser = $state<{ id: string; email: string; displayName: string } | null>(null);
let _orgs = $state<MeOrgMembership[] | null>(null);

// Guards a single in-flight /api/me call. Concurrent callers await the
// same promise; resolved-state callers return immediately.
let _loadingMe: Promise<void> | null = null;

export const auth = {
  get token(): string | null { return _token; },
  get refresh(): string | null { return _refresh; },
  get currentUser() { return _currentUser; },
  get orgs(): MeOrgMembership[] | null { return _orgs; },
  get isAuthenticated(): boolean { return _token !== null; },

  setTokens(access: string, refreshTok: string): void { /* unchanged */ },

  signOut(): void {
    _token = null;
    _refresh = null;
    _currentUser = null;
    _orgs = null;            // new
    _loadingMe = null;       // new — clear any in-flight guard
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(REFRESH_KEY);
    navigate('/login');
  },

  async loadCurrentUser(): Promise<void> {
    if (_currentUser !== null && _orgs !== null) return;
    if (_loadingMe !== null) return _loadingMe;

    _loadingMe = (async () => {
      try {
        const { data } = await client.GET('/api/me');
        if (data) {
          _currentUser = {
            id: data.id,
            email: data.email,
            displayName: data.display_name,
          };
          _orgs = data.orgs;
        }
      } catch {
        // Leave state as-is; the App.svelte effect will retry on next
        // isAuthenticated flip if any.
      } finally {
        _loadingMe = null;
      }
    })();

    return _loadingMe;
  },

  // Append a freshly-created org to the local cache. Assigns a new array
  // (not push-in-place) so Svelte 5 $state reactivity fires.
  addOrg(org: MeOrgMembership): void {
    if (_orgs === null) _orgs = [org];
    else _orgs = [..._orgs, org];
  },
};
```

**`App.svelte` bootstrap effect** (added alongside the existing auth-gate effect):

```svelte
// Existing: redirect unauthed users to /login on protected routes.
$effect(() => { /* unchanged */ });

// New: when the user is authenticated but we have not yet loaded /api/me,
// fetch it. Covers cold-load with persisted tokens; the OAuthCallback path
// awaits loadCurrentUser() explicitly before navigating, so this effect
// is a no-op there (guarded inside auth.loadCurrentUser).
$effect(() => {
  if (auth.isAuthenticated && auth.orgs === null) {
    void auth.loadCurrentUser();
  }
});
```

**Implementation Notes**:
- The two existing chrome consumers (`SessionList.svelte`, `SessionViewShell.svelte`,
  `InviteAccept.svelte`) read `auth.currentUser` and gate via `{#if auth.currentUser}`.
  They render `null` today because nothing calls `loadCurrentUser()`. After this
  story they render properly post-bootstrap. No code changes needed in those screens.
- `_orgs` MUST be assigned a new array reference in `addOrg`, never mutated in
  place — Svelte 5's `$state` proxies handle pushes correctly for arrays in
  most cases, but explicit reassignment is unambiguous and matches the project's
  existing patterns (`Login.svelte` similarly reassigns `mode = 'magic-link-sent'`).
- `_loadingMe` is cleared in `signOut()` so a sign-out + sign-in in the same
  tab does not get stuck on a stale promise.
- The `MeOrgMembership` import comes from `components['schemas']['MeOrgMembership']`
  in the generated `types.gen.ts` — verify that path against the current
  `openapi-typescript` output before locking import syntax.

**Acceptance Criteria**:
- [ ] `auth.orgs` returns `null` before `loadCurrentUser()` resolves and
      an `MeOrgMembership[]` after.
- [ ] `auth.loadCurrentUser()` is idempotent: two concurrent calls fire
      one fetch; a call after resolution is a no-op.
- [ ] `auth.signOut()` clears `_orgs` and `_loadingMe`.
- [ ] `auth.addOrg(org)` appends to `_orgs`, creating the array if `null`.
- [ ] App.svelte's new effect calls `loadCurrentUser()` exactly once on
      cold-load with persisted tokens (verify via fetch-mock call count).
- [ ] Existing `auth.test.ts` cases continue to pass after the shape change.

---

### Unit 2: Home screen + router wiring

**Story**: `spa-logged-in-landing-home-screen`
**Depends on**: `spa-logged-in-landing-auth-store-orgs-cache`
**Files**:
- `frontend/src/lib/screens/Home.svelte` (new)
- `frontend/src/lib/screens/Home.test.ts` (new)
- `frontend/src/lib/router.svelte.ts` (edit — add `/` route)
- `frontend/src/App.svelte` (edit — render Home on `current.name === 'home'`)

**`router.svelte.ts` delta** — add as the FIRST route entry so first-match
semantics catch `/` before any other pattern:

```typescript
const routes: Route[] = [
  { pattern: /^\/$/,                                                   name: 'home',         params: [] },
  { pattern: /^\/login$/,                                              name: 'login',        params: [] },
  // ... existing entries unchanged
];
```

**`App.svelte` rendering delta** — extend the `{#if}` chain:

```svelte
{#if current.name === 'login'}
  <Login />
{:else if current.name === 'home'}
  <Home />
{:else if current.name === 'magic-link'}
  <!-- ... rest unchanged -->
```

**`Home.svelte` shape**:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { navigate } from '$lib/router.svelte';
  import { auth } from '$lib/auth.svelte';
  import { client } from '$lib/api/client';
  import Button from '$lib/components/Button.svelte';
  import Input from '$lib/components/Input.svelte';
  import Card from '$lib/components/Card.svelte';

  // 'creating' is the in-flight POST /api/orgs state; 'create-error' surfaces
  // a server failure. The list-load state machine is driven by auth.orgs:
  //   null      -> loading
  //   []        -> empty
  //   [single]  -> auto-route effect fires (no render)
  //   [a, b...] -> picker
  type CreateState = 'idle' | 'creating' | 'create-error';

  let newOrgName = $state('');
  let createState = $state<CreateState>('idle');
  let createError = $state<string | null>(null);

  // Single-org auto-route. Fires once when auth.orgs resolves to exactly one
  // entry. The _autoRouted latch prevents re-firing if the user navigates
  // back to / after creating an org (in which case orgs will already have
  // grown beyond 1, but the latch keeps things safe).
  let _autoRouted = false;
  $effect(() => {
    if (!_autoRouted && auth.orgs !== null && auth.orgs.length === 1) {
      _autoRouted = true;
      navigate(`/orgs/${auth.orgs[0].id}/sessions`);
    }
  });

  async function createOrg(e: Event) {
    e.preventDefault();
    const name = newOrgName.trim();
    if (name.length === 0 || createState === 'creating') return;
    createState = 'creating';
    createError = null;

    try {
      const { data, error } = await client.POST('/api/orgs', { body: { name } });
      if (data) {
        auth.addOrg({ id: data.id, name: data.name, slug: data.slug, role: 'creator' });
        navigate(`/orgs/${data.id}/sessions`);
        return;
      }
      createError = (error as { message?: string } | undefined)?.message ?? 'Could not create org';
      createState = 'create-error';
    } catch {
      createError = 'Could not reach the server. Try again.';
      createState = 'create-error';
    }
  }

  function roleLabel(role: string): string {
    return role.charAt(0).toUpperCase() + role.slice(1);
  }
</script>

<!-- topbar matches the mock; Login/OAuthCallback use a similar custom shape -->
<header class="topbar">
  <div class="wordmark">jam<span class="dot">·</span>sesh</div>
  <div class="user-strip">
    {#if auth.currentUser}<span class="email">{auth.currentUser.email}</span>{/if}
    <button class="signout-btn" type="button" onclick={() => auth.signOut()}>Sign out</button>
  </div>
</header>

<main class="page">
  <Card padding="lg">
    {#if auth.orgs === null}
      <p class="loading" aria-busy="true">Loading your workspaces...</p>

    {:else if auth.orgs.length === 0}
      <h1>Welcome to jamsesh{auth.currentUser ? `, ${auth.currentUser.displayName}` : ''}</h1>
      <p class="lead">
        You're signed in but not in any orgs yet. Spin up your first org —
        you'll be its creator, and you can invite teammates from there.
      </p>
      <!-- create-form (shared with picker state) -->
      {@render createForm()}

    {:else}
      <!-- auth.orgs.length >= 2 (the length === 1 case auto-routes above) -->
      <h1>Pick a workspace</h1>
      <p class="lead">You're in {auth.orgs.length} orgs. Click one to drop in, or create another below.</p>

      <ul class="org-list">
        {#each auth.orgs as org (org.id)}
          <li>
            <a class="org-row" href="/orgs/{org.id}/sessions"
               onclick={(e) => { e.preventDefault(); navigate(`/orgs/${org.id}/sessions`); }}>
              <span class="org-avatar">{org.name.charAt(0).toUpperCase()}</span>
              <span class="org-meta">
                <span class="org-name">{org.name}</span>
                <span class="org-slug">/orgs/{org.slug}/sessions</span>
              </span>
              <span class="role-badge role-{org.role}">{roleLabel(org.role)}</span>
            </a>
          </li>
        {/each}
      </ul>

      <div class="divider" aria-hidden="true">or</div>
      {@render createForm()}
    {/if}
  </Card>
</main>

{#snippet createForm()}
  <div class="create-block">
    <label class="label" for="new-org-name">
      {auth.orgs && auth.orgs.length === 0 ? 'Name your org' : 'Create another org'}
    </label>
    <form class="create-form" onsubmit={createOrg}>
      <Input bind:value={newOrgName} placeholder="e.g. acme" id="new-org-name" />
      <Button variant="primary" type="submit" size="md" disabled={createState === 'creating'}>
        {#snippet children()}{createState === 'creating' ? 'Creating...' : 'Create org'}{/snippet}
      </Button>
    </form>
    <p class="form-hint">The slug is derived from the name. You become its creator.</p>
    {#if createState === 'create-error' && createError}
      <p class="form-error" role="alert">{createError}</p>
    {/if}
  </div>
{/snippet}
```

**Implementation Notes**:
- The auto-route `$effect` uses `_autoRouted` as a plain `let` (NOT `$state`) —
  it's a one-shot latch, not reactive state. Marking it `$state` would cause
  the effect to re-run when it flips, no-op'ing harmlessly but adding noise.
- The `_autoRouted` latch is per-component-instance. If the user navigates
  away and comes back (Home unmounts and remounts), the latch resets — which
  is the right behavior: a single-org user coming back from elsewhere should
  auto-route again.
- The `<a href>` on org rows still calls `navigate()` via `onclick` with
  `preventDefault()`. The `href` is present for middle-click / open-in-new-tab
  affordance and accessibility (real anchors are navigable by keyboard); the
  `onclick` keeps the SPA's history-API behavior for normal clicks.
- The `{#snippet createForm()}` block is rendered in both empty and picker
  states with `{@render createForm()}`. Same form, same handler.
- `role` from the schema is a `string`, not an enum, so we don't switch on it
  exhaustively. Title-cased label works for any value the server returns.
  CSS class `role-{org.role}` lets us style `role-creator` distinctly via
  the accent-muted token; other roles fall through to the neutral pill style.
- Styling targets the locked tokens from `.mockups/design-system/tokens.css`
  (matches mock option-1 verbatim — see `.mockups/screens/spa-logged-in-landing-and-org-bootstrap/option-1.html`).

**Acceptance Criteria**:
- [ ] Navigating to `/` renders `Home.svelte` (router maps `/` -> `home`).
- [ ] When `auth.orgs` is `null`, the screen renders a loading state with
      `aria-busy="true"`.
- [ ] When `auth.orgs.length === 0`, the screen renders the empty state
      with the inline create-form (welcome heading + form, no list).
- [ ] When `auth.orgs.length === 1`, the screen navigates to
      `/orgs/{onlyId}/sessions` without rendering the picker.
- [ ] When `auth.orgs.length >= 2`, the screen renders the picker with one
      row per org, each row navigable.
- [ ] Submitting a non-empty org name calls `POST /api/orgs` and on `201`:
      (a) `auth.addOrg` is invoked with the response, (b) navigation goes
      to `/orgs/{newId}/sessions`.
- [ ] Empty / whitespace-only names are rejected client-side (no network call).
- [ ] Network or non-2xx responses surface `createError` to the user
      without leaving the create button stuck in 'Creating...'.
- [ ] Role badges render with `Creator` / `Member` (title-cased) and the
      creator pill uses the accent-muted color.

---

### Unit 3: Authed-redirect fixes

**Story**: `spa-logged-in-landing-authed-redirect-fixes`
**Depends on**: `spa-logged-in-landing-home-screen`
**Files**:
- `frontend/src/lib/screens/OAuthCallback.svelte` (edit lines 46-54)
- `frontend/src/lib/screens/OAuthCallback.test.ts` (edit — update navigate assertions)
- `frontend/src/lib/screens/Login.svelte` (edit lines 46-50)
- `frontend/src/lib/screens/Login.test.ts` (edit — new authed-redirect tests)
- `frontend/src/App.svelte` (edit existing auth-gate `$effect`)

**`OAuthCallback.svelte` delta** — change the fallback target and await
`loadCurrentUser` before navigating so the "Signing you in..." view
encompasses the `/api/me` round-trip:

```svelte
async function exchange(provider: string, code: string, state: string, returnTo: string | null) {
  try {
    const { data, error } = await client.POST('/api/auth/oauth/callback', {
      body: { provider, code, state },
    });

    if (data) {
      auth.setTokens(data.access_token, data.refresh_token);
      await auth.loadCurrentUser();   // NEW — populates auth.orgs before Home renders
      navigate(returnTo ?? '/');       // CHANGED — was '/login'
      return;
    }
    // ... error branch unchanged
  } catch {
    // unchanged
  }
}
```

**`Login.svelte` delta** — redirect authed users unconditionally:

```svelte
$effect(() => {
  if (auth.isAuthenticated) {
    navigate(returnTo ?? '/');
  }
});
```

(The previous body `if (auth.isAuthenticated && returnTo) navigate(returnTo)`
left authed users stuck on the form when no `returnTo` was set. Now they
always land somewhere — `returnTo` if present, else `/`.)

**`App.svelte` delta** — extend the existing auth-gate `$effect` to also
catch authed users hitting `/login`. The existing branch handles
unauthed-on-protected; the new branch handles authed-on-login:

```svelte
$effect(() => {
  // Authed user landed on /login somehow (direct visit, back button, etc.).
  // Bounce them to the home route. Skip oauth-callback (it has its own
  // post-exchange navigation logic) and magic-link (it MAY be hit while
  // still unauthed to complete the exchange — handled there).
  if (auth.isAuthenticated && current.name === 'login') {
    navigate('/');
    return;
  }

  // Existing: unauthed user on a protected route -> /login.
  if (current.name !== 'login' && current.name !== 'magic-link'
      && current.name !== 'oauth-callback' && !auth.isAuthenticated) {
    if (current.name === 'invite-accept') {
      const returnTo = window.location.pathname + window.location.search;
      navigate('/login?return_to=' + encodeURIComponent(returnTo));
    } else {
      navigate('/login');
    }
  }
});
```

There's an intentional redundancy here: both `Login.svelte`'s own `$effect`
and `App.svelte`'s gate catch the authed-on-login case. The App.svelte
gate fires whenever the route changes; the Login.svelte effect fires
whenever auth flips while Login is mounted. Both converge on
`navigate('/')`. Redundancy is cheap (idempotent `navigate`) and the
defense-in-depth means neither file alone breaks the behavior if touched
in isolation.

**Implementation Notes**:
- The OAuthCallback `await auth.loadCurrentUser()` adds one round-trip
  to the post-OAuth flow. It happens during the "Signing you in..."
  view, so the user sees no additional spinner — the existing UI just
  takes a moment longer to leave.
- If `loadCurrentUser` fails (network blip), `auth.orgs` stays `null`,
  the user lands on Home, and the App.svelte cold-load effect retries.
  Worst case: the user sees Home's loading spinner for a few seconds
  before the retry resolves.
- The `return_to` preservation logic in OAuthCallback for invite-accept
  is unchanged. Only the `??` fallback changes.

**Acceptance Criteria**:
- [ ] OAuthCallback on success navigates to `returnTo ?? '/'` (was `'/login'`).
- [ ] OAuthCallback awaits `auth.loadCurrentUser()` before navigating.
- [ ] Login.svelte redirects authed users to `'/'` when `returnTo` is null.
- [ ] Login.svelte redirects authed users to `returnTo` when set (existing).
- [ ] App.svelte redirects authed users hitting `/login` to `/`.
- [ ] App.svelte still redirects unauthed users on protected routes to `/login`
      (existing behavior preserved).
- [ ] OAuthCallback.test.ts and Login.test.ts cover the new paths.

---

## Implementation Order

1. **spa-logged-in-landing-auth-store-orgs-cache** — foundation; everything
   else reads from `auth.orgs`.
2. **spa-logged-in-landing-home-screen** — depends on Unit 1; introduces
   the `/` route and the Home component.
3. **spa-logged-in-landing-authed-redirect-fixes** — depends on Unit 2;
   redirects only work once `/` is a valid route.

Linear chain — `/agile-workflow:implement-orchestrator` will fan these
out sequentially, one Sonnet agent per wave.

## Testing

Each story has its own test file mirroring the existing project pattern:
- `auth.test.ts` — vitest with fetch-mocking; assertions on store state
  after `loadCurrentUser` resolves.
- `Home.test.ts` — `@testing-library/svelte` `render` + `screen` queries;
  fetch-mock for `POST /api/orgs`; navigate-mock for verifying redirects.
- `OAuthCallback.test.ts` / `Login.test.ts` — extend existing files; do
  NOT delete existing assertions (regression coverage of unchanged paths).

Integration coverage of the full flow (sign in -> Home -> create org ->
session list) is out of scope for v1 unit tests. The e2e program lives
in `tests/e2e/` and an end-to-end-style assertion against this happy path
can be a follow-up backlog item if the existing playwright/cypress
infrastructure (if any) is in place.

## Risks

- **Reactivity edge case in `auth.addOrg`**. Svelte 5 `$state` proxies
  arrays, but `_orgs.push(...)` from outside the module would not trigger
  consumer effects. Mitigation: `addOrg` reassigns (`_orgs = [..._orgs, org]`)
  and is the only path that mutates orgs. Tested explicitly.
- **`loadCurrentUser` idempotency under fast OAuthCallback -> App.svelte
  effect race**. After `auth.setTokens`, the App.svelte effect schedules
  a `loadCurrentUser` call, and OAuthCallback awaits its own
  `loadCurrentUser` call. Both hit the `_loadingMe` guard; the second
  call awaits the first's in-flight promise. Verified by the
  idempotency acceptance criterion.
- **Single-org user signs out, signs in as different user with 1 org in
  another tab**. Cross-tab state isn't synchronized in this design;
  outside scope. Documented behavior: each tab has its own `auth` state;
  localStorage tokens sync but `_orgs` does not. Acceptable v1.
- **Auto-route firing twice during transient orgs.length === 1 window**.
  If a user has 1 org, sees auto-route, hits back, lands on Home again,
  the latch `_autoRouted` resets on remount so auto-route fires again —
  intentional. There's no path where orgs grows from 0 to 1 while Home
  is mounted (creating an org navigates away first).
- **The `MeOrgMembership.role` field is untyped string**. The schema
  documents examples (`'creator'`, `'member'`) but isn't an enum. Future
  role values render in the picker with title-casing. Visual style for
  unknown roles falls through to the neutral pill (acceptable).
