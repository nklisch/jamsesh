# window.location Override via Object.defineProperty

Tests that exercise components reading `window.location.search`,
`window.location.hash`, `window.location.pathname`, or invoking
`window.location.assign(...)` override the property via
`Object.defineProperty(window, 'location', { value: { ...window.location, ... },
writable: true, configurable: true })`. The `setSearch` / `setHash` helper
variant wraps the call in a single-line function per test file.

## Rationale

jsdom 21+ makes `window.location` read-only — direct assignment
(`window.location = {...}`) throws
`TypeError: Cannot assign to read only property`. `Object.defineProperty`
with `writable: true, configurable: true` bypasses the readonly descriptor
and replaces the property with a plain object so that any code reading
`window.location.search` (etc.) gets the stubbed value.

Spreading the existing `window.location` preserves the unmodified fields
(origin, host, etc.) so unrelated reads still work. `configurable: true` is
critical — without it, the next test's `Object.defineProperty` would throw.

## Examples

### Example 1: helper that stubs `search`
**File**: `frontend/src/lib/screens/OAuthCallback.test.ts:37`

```ts
function setSearch(search: string) {
  Object.defineProperty(window, 'location', {
    value: { ...window.location, pathname: '/auth/oauth/callback', search, hash: '' },
    writable: true,
    configurable: true,
  });
}
```

### Example 2: helper that stubs `hash`
**File**: `frontend/src/lib/screens/MagicLinkExchange.test.ts`

```ts
function setHash(hash: string) {
  Object.defineProperty(window, 'location', {
    value: { ...window.location, hash, pathname: '/auth/magic-link', search: '' },
    writable: true,
    configurable: true,
  });
}
```

### Example 3: inline override of `assign` for OAuth redirect
**File**: `frontend/src/lib/screens/Login.test.ts:52`

```ts
const assignSpy = vi.fn();
Object.defineProperty(window, 'location', {
  value: { ...window.location, assign: assignSpy },
  writable: true,
  configurable: true,
});
```

### Example 4: inline override of `pathname` for router popstate test
**File**: `frontend/src/lib/router.test.ts`

```ts
Object.defineProperty(window, 'location', {
  value: { ...window.location, pathname: '/login' },
  writable: true,
  configurable: true,
});
window.dispatchEvent(new PopStateEvent('popstate', {}));
```

## When to Use

- Any test that exercises code reading `window.location.search`, `.hash`,
  `.pathname`, or invoking `.assign(...)`.
- Prefer the helper-function form (`setSearch`, `setHash`) when more than
  two tests in the same file vary the same field.

## When NOT to Use

- Tests that don't touch `window.location` — don't mutate global state
  unnecessarily.
- Tests for code that uses `URL` / `URLSearchParams` directly with a known
  string — pass the string in rather than going through `window.location`.

## Common Violations

- `window.location = { ... }` — throws in jsdom; the test fails with an
  opaque "Cannot assign" message.
- `Object.defineProperty(window, 'location', { value: ... })` without
  `configurable: true` — the next test's override throws "Cannot redefine
  property: location".
- Forgetting `...window.location` and only providing the one stubbed field —
  `window.location.origin` / `.host` / `.protocol` reads in unrelated code
  paths return `undefined`.
