# Per-Instance Factory Rune Store

Rune stores in `frontend/src/lib/**/*.svelte.ts` exported as an
`export function create<Name>(...)` factory whose body declares
`$state`/`$derived` runes in a closure and returns the plain-object facade —
so each component instance gets its own isolated state, rather than sharing a
module-level singleton.

## Rationale

The existing `wrapper-object-rune-store` pattern keeps `$state` at module
scope, which is correct when the state is a true app-wide singleton (auth,
router, ws). But several stores model **per-mount** state — one drawer's
form, one shell's tree-mode, one shell's countdown — and a module-level
singleton would leak across mounts (two `SessionViewShell` in the same SPA
session would clobber each other's `_activeMenuRef`). The factory variant
keeps the same `$state` + private `_`-prefixed + facade-object discipline
(so Svelte 5's "no raw `$state` exports" rule is preserved), but each
`create*` call returns a fresh closure.

## Examples

### Example 1: per-shell composer state (zero-arg factory)

**File**: `frontend/src/lib/session/useCommentComposer.svelte.ts:10`

```ts
export function createCommentComposer() {
  let _open = $state(false);
  let _range = $state<{ start: number; end: number } | null>(null);

  return {
    get open(): boolean { return _open; },
    get range(): { start: number; end: number } | null { return _range; },
    handleRangeSelect(range: { start: number; end: number } | null) {
      _range = range;
      if (range) _open = true;
    },
    close() { _open = false; },
  };
}
```

### Example 2: per-session tree-mode factory with localStorage + $effect

**File**: `frontend/src/lib/session/useTreeState.svelte.ts:24`

```ts
export function createTreeState(sessionId: string) {
  let _state = $state<TreeState>(loadFromLS(sessionId));

  $effect(() => {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(treeStateKey(sessionId), _state);
    }
  });

  return {
    get value(): TreeState { return _state; },
    cycle() {
      const idx = TREE_STATES.indexOf(_state);
      _state = TREE_STATES[(idx + 1) % TREE_STATES.length];
    },
  };
}
```

### Example 3: per-drawer form facade with declared interface type

**File**: `frontend/src/lib/components/useNewSessionForm.svelte.ts:89`

```ts
export function createNewSessionForm(): NewSessionFormFacade {
  let _goal = $state('');
  let _scopeRaw = $state('');
  let _defaultMode = $state<'sync' | 'isolated'>('sync');
  // ...
  return {
    get goal(): string { return _goal; },
    setGoal(v: string): void { _goal = v; },
    submit(orgId: string): boolean { /* ... */ },
    reset(): void { /* ... */ },
  };
}
```

Replicated in `createRefActions` (`session/useRefActions.svelte.ts:11`) and
`createPlaygroundCountdown` (`session/usePlaygroundCountdown.svelte.ts:20`) —
5 factories consumed by
`frontend/src/lib/screens/SessionViewShell.svelte:54-57` and
`frontend/src/lib/components/NewSessionDrawer.svelte:21`.

Also used by the four finalize stores (`createFinalizeLock`,
`createFinalizePlan`, `createFinalizeCuration`, `createFinalizeExecution`) —
instantiated once per `FinalizeView` mount so that overlapping A→B mount/unmount
under SPA routing never shares lock, plan, or curation state across instances.

## When to Use

- State is owned by one component mount and there can be more than one mount
  alive at once (drawer, shell, modal, per-row controller).
- The factory needs a per-instance argument (session id, key prefix) that
  should be captured in the closure, not passed on every method call.
- The store uses `$effect` for side effects that must run within a
  component's reactive root.

## When NOT to Use

- State is genuinely app-singleton (auth identity, router, websocket
  connection). Use the existing module-level `wrapper-object-rune-store`
  instead — `auth.svelte.ts`, `router.svelte.ts`, `ws.svelte.ts` are the
  right shape there.

## Common Violations

- Module-level `let _x = $state(...)` for state that needs per-mount
  isolation — silently fuses two component instances' state.
- Returning the facade from a closure but reading `$state` at module scope —
  defeats the closure isolation.
- Forgetting to capture the factory arg in the closure (e.g. using
  `sessionId` directly inside `$derived` without an arrow-function
  indirection — see the explicit `getSessionId = () => sessionId` shim in
  `usePlaygroundCountdown.svelte.ts:24`).
