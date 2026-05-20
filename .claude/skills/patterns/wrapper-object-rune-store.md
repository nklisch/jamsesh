# Wrapper-Object Rune Store

Rune stores in `frontend/src/lib/*.svelte.ts` keep private module-level
`$state` / `$derived` variables prefixed with `_` and expose them through a
single `export const` plain object that uses getter syntax
(`get foo() { return _foo; }`) plus regular methods for mutation. Direct
export of `$state` / `$derived` is prohibited at module scope in Svelte 5,
and the wrapper preserves reactivity while giving consumers a stable,
dot-callable API.

## Rationale

Svelte 5 disallows exporting a raw `$state` or `$derived` value from a
`.svelte.ts` module — the rune's reactive proxy unwraps to a plain value at
the export boundary, losing reactivity. The wrapper object preserves
reactivity because each getter is re-invoked on every reactive read inside a
`$effect` / `$derived`. It also gives consumers a uniform shape:
`auth.token`, `current.name`, `wsStatus.for(id)` all read the same way,
regardless of whether the underlying source is `$state`, `$derived`, or
computed from a private map.

## Examples

### Example 1: `auth` store — private `$state` + getter facade + mutation methods
**File**: `frontend/src/lib/auth.svelte.ts:16`

```ts
let _token = $state<string | null>(
  typeof localStorage !== 'undefined' ? localStorage.getItem(TOKEN_KEY) : null,
);
let _refresh = $state<string | null>(...);
let _currentUser = $state<{...} | null>(null);
let _orgs = $state<MeOrgMembership[] | null>(null);

export const auth = {
  get token(): string | null { return _token; },
  get refresh(): string | null { return _refresh; },
  get currentUser() { return _currentUser; },
  get orgs() { return _orgs; },
  get isAuthenticated(): boolean { return _token !== null; },
  setTokens(access, refreshTok) { _token = access; ... },
  signOut() { _token = null; ... },
  async loadCurrentUser() { ... },
  addOrg(org) { _orgs = _orgs === null ? [org] : [..._orgs, org]; },
};
```

### Example 2: `current` route store — private `$derived` exposed through getter wrapper
**File**: `frontend/src/lib/router.svelte.ts:32`

```ts
let path = $state(typeof window !== 'undefined' ? window.location.pathname : '/');
let _current = $derived(match(path));

// `current` is exposed as an object with a $derived getter so consumers can
// read `current.name` and `current.params` reactively. Exporting a plain
// `$derived` value is not permitted in Svelte 5 module context.
export const current = {
  get name() { return _current.name; },
  get params() { return _current.params; },
};

export function navigate(to: string): void { ... }
```

### Example 3: `wsStatus` store — private record `$state` exposed via method-style getter
**File**: `frontend/src/lib/ws.svelte.ts:81`

```ts
// Reactive per-session status. Wrapper-object pattern (mirrors
// auth.svelte.ts) so consumers get reactivity without us exporting the
// raw $state expression.
const _statuses = $state<Record<string, WsStatus | null>>({});

export const wsStatus = {
  for(sessionId: string): WsStatus | null {
    return _statuses[sessionId] ?? null;
  },
};
```

## When to Use

- Any new module-level rune store (`*.svelte.ts`) that needs to expose
  reactive state to consumers.
- Whenever a derived value is computed from private state and consumers
  should observe it reactively.
- When a domain has both reactive state AND mutation methods that belong
  together (single export, single import line at call sites).

## When NOT to Use

- Inside a Svelte component — components can declare `$state` and `$derived`
  locally; no wrapper needed there.
- For pure read-only constants — `export const FOO = '...'` is fine when
  there is no rune behind it.
- For per-instance stores that need to be constructed multiple times — the
  pattern assumes module-singleton state.

## Common Violations

- `export const token = $state(...)` at module scope — Svelte will warn, and
  consumers will see a non-reactive snapshot.
- `export const auth = { token: _token, ... }` (no getter) — captures the
  value at export time; never updates.
- Exporting `_orgs` directly and a separate `addOrg()` function — splits the
  API, makes mocking awkward, and tempts callers to mutate the array in
  place (which breaks reactivity unless reassigned — see the `addOrg`
  implementation in `auth.svelte.ts:99` that explicitly reassigns to fire
  reactivity).
