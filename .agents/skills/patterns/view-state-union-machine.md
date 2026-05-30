# String-Literal-Union View-State Machine

Screens and screen-shaped components declare a local string-literal-union
`type ViewState = 'a' | 'b' | 'c'` (named variously `ViewState`, `Mode`,
`CreateState`) and a single `let viewState = $state<ViewState>('initial')`
rune that drives template branching with
`{#if viewState === 'a'} ... {:else if viewState === 'b'} ...`. Transitions
are written as direct assignments inside async handlers
(`viewState = 'error'`) — no separate "transition" function.

## Rationale

Most screens have 3-5 visually distinct UI states (loading → success /
error / submitting / done) that share state (form fields, error codes,
server data) and need a single source of truth for which UI block renders.

A string-literal-union union plus a single rune is the minimal Svelte 5
shape: TypeScript exhaustiveness-checks the union at every assignment site,
the rune fires reactivity on every transition, and the template can
pattern-match each variant with an `{#if}` chain.

A boolean-soup alternative (`isLoading`, `hasError`, `isSubmitting`) makes
invalid combinations representable (`isLoading && hasError` — what does it
mean?) and forces every transition to update multiple booleans atomically.

## Examples

### Example 1: OAuthCallback — exchanging / error
**File**: `frontend/src/lib/screens/OAuthCallback.svelte:12`

```ts
// ── State machine ─────────────────────────────────────────────────────────
//
//  exchanging → done (POST 200: store tokens, redirect)
//  exchanging → error (POST non-200, missing params, or network failure)
type ViewState = 'exchanging' | 'error';

let viewState = $state<ViewState>('exchanging');
let errorCode = $state<string | null>(null);
```

Template:

```svelte
{#if viewState === 'exchanging'}
  <p class="status" aria-busy="true">Signing you in…</p>
{:else}
  <h1>This sign-in link is no longer valid</h1>
{/if}
```

### Example 2: InviteAccept — five-state union
**File**: `frontend/src/lib/screens/InviteAccept.svelte:25`

```ts
//  loading  → ready (GET 200)
//  loading  → error (GET non-200, missing token)
//  ready    → accepting (POST in flight)
//  accepting → navigate away (POST 200)
//  accepting → rejection (POST 403 auth.org_membership_required)
//  accepting → error (POST other failure)
type ViewState = 'loading' | 'ready' | 'accepting' | 'rejection' | 'error';
let viewState = $state<ViewState>('loading');
```

### Example 3: Login — Mode-named variant
**File**: `frontend/src/lib/screens/Login.svelte:12`

```ts
type Mode = 'choose' | 'magic-link-sent' | 'magic-link-error' | 'oauth-error';

let mode = $state<Mode>('choose');
let errorMsg = $state<string | null>(null);
```

### Example 4: Home — CreateState-named variant for sub-flow
**File**: `frontend/src/lib/screens/Home.svelte:15`

```ts
// 'creating' is the in-flight POST /api/orgs state; 'create-error' surfaces
// a server failure. The list-load state machine is driven by auth.orgs:
//   null      -> loading
//   []        -> empty
//   [single]  -> auto-route effect fires (no render)
//   [a, b...] -> picker
type CreateState = 'idle' | 'creating' | 'create-error';

let createState = $state<CreateState>('idle');
```

### Example 5: MagicLinkExchange — same shape as OAuthCallback
**File**: `frontend/src/lib/screens/MagicLinkExchange.svelte:12`

```ts
type ViewState = 'exchanging' | 'error';
let viewState = $state<ViewState>('exchanging');
```

### Example 6: CommentsTab / SessionList — LoadState-named variant
**Files**: `frontend/src/lib/components/CommentsTab.svelte:26`,
`frontend/src/lib/screens/SessionList.svelte:24`

```ts
type LoadState = 'loading' | 'ready' | 'error';
let loadState = $state<LoadState>('loading');
```

Two distinct surfaces independently arrived at the same shape: a remote-fetch
load lifecycle with the three terminal states `loading → ready` (data
present) or `loading → error` (fetch failed). The `LoadState` name signals
the intent — "what's the state of the load" — rather than overloading
`ViewState` for a sub-flow.

## When to Use

- Any screen that has more than two visually distinct rendering modes driven
  by async outcomes.
- When the next state depends on the previous (forming a directed graph) —
  the comment block at the top of the union typedef typically draws the
  graph.
- When TypeScript exhaustiveness checking on `switch (viewState)` would
  catch bugs (the template `{#if}` chain is the visual analog).

## When NOT to Use

- Two-state flips that are naturally boolean (`isOpen`, `isDirty`) —
  `let isOpen = $state(false)` is clearer than
  `type DialogState = 'open' | 'closed'`.
- When the source of truth is a remote store value (e.g. Home.svelte derives
  loading/empty/picker from `auth.orgs === null` / `[]` / `[...]` rather
  than a local union, because the source of truth is the store).
- When the states aren't mutually exclusive — use independent booleans /
  sub-objects.

## Common Violations

- Three booleans (`isLoading`, `hasError`, `isDone`) where exactly one is
  supposed to be `true` at a time — introduces invalid states and forces
  every transition to update all three.
- `let state = $state<string>('loading')` (no union type) — loses TypeScript
  exhaustiveness, allows typos like `state = 'loadingg'`.
- Mixing the union with overlapping booleans
  (`if (viewState === 'loading' || isLoading)`) — pick one.
