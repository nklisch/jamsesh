# SPA Test Module-Mock Barrel

Component and screen test files (`*.test.ts`) declare a fixed set of
`vi.mock(...)` blocks at the top, one per `$lib/` runtime singleton they
touch — `auth.svelte`, `router.svelte`, `api/client`, and `ws.svelte`. Each
mock pairs a module-scoped `vi.fn()` (or a mutable `let` variable) with a
function-form factory that delegates to it (`vi.mock('$lib/api/client',
() => ({ client: { GET: (...args) => mockGET(...args) } }))`), so per-test
reassignment doesn't get clobbered by the mock factory hoisting.

## Rationale

`vi.mock` is hoisted to the top of the file, before any `import` or `const`
declarations execute. A naive
`vi.mock('$lib/api/client', () => ({ client: { GET: vi.fn() } }))` creates a
fresh `vi.fn()` that tests can't reach because it's not bound to an outer
name. Declaring `const mockGET = vi.fn()` (which also gets hoisted but as a
runtime constant) and routing the mock through `(...args) => mockGET(...args)`
gives the test code a handle it can stub via `mockGET.mockResolvedValue(...)`
per test.

Stateful mocks for `$lib/auth.svelte` use a mutable `let mockOrgs` plus
getter accessors inside the mock factory, so tests can re-point the
underlying value without rebuilding the factory.

## Examples

### Example 1: client mock with delegate-to-fn
**File**: `frontend/src/lib/screens/Home.test.ts:11`

```ts
const mockPOST = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    POST: (...args: unknown[]) => mockPOST(...args),
  },
}));

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  current: { name: 'home', params: {} },
  navigate: (...args: unknown[]) => mockNavigate(...args),
}));

// Mutable auth mock — tests override `orgs` / `currentUser` per scenario.
let mockOrgs: { id: string; name: string; slug: string; role: string }[] | null = null;
let mockCurrentUser: { id: string; email: string; displayName: string } | null = null;
const mockSignOut = vi.fn();
vi.mock('$lib/auth.svelte', () => ({
  auth: {
    get orgs() { return mockOrgs; },
    get currentUser() { return mockCurrentUser; },
    get isAuthenticated() { return true; },
    signOut: (...args: unknown[]) => mockSignOut(...args),
    addOrg: (...args: unknown[]) => mockAddOrg(...args),
  },
}));
```

### Example 2: same shape, multi-verb client
**File**: `frontend/src/lib/screens/OrgSettings.test.ts`

```ts
const mockGET = vi.fn();
const mockPATCH = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: {
    GET: (...args: unknown[]) => mockGET(...args),
    PATCH: (...args: unknown[]) => mockPATCH(...args),
  },
}));

vi.mock('$lib/auth.svelte', () => ({
  auth: {
    currentUser: { id: 'user-1', email: 'creator@example.com', displayName: 'Creator' },
    isAuthenticated: true,
    signOut: vi.fn(),
  },
}));

vi.mock('$lib/router.svelte', () => ({
  current: { name: 'org-settings', params: { orgId: 'org-1' } },
  navigate: vi.fn(),
}));
```

### Example 3: same shape, with ws mock + per-event handler registry
**File**: `frontend/src/lib/screens/SessionList.test.ts`

```ts
const mockSubscribe = vi.fn().mockReturnValue(() => {});
vi.mock('$lib/ws.svelte', () => ({
  subscribe: (...args: unknown[]) => mockSubscribe(...args),
}));

const mockGET = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: { GET: (...args: unknown[]) => mockGET(...args), POST: vi.fn() },
}));

vi.mock('$lib/router.svelte', () => ({
  current: { name: 'sessions', params: { orgId: 'org-1' } },
  navigate: vi.fn(),
}));

vi.mock('$lib/auth.svelte', () => ({
  auth: {
    currentUser: { id: 'user-1', email: 'test@example.com', displayName: 'Test User' },
    isAuthenticated: true,
    signOut: vi.fn(),
  },
}));
```

## When to Use

- Any `*.test.ts` for a screen or component that imports a `$lib/` runtime
  singleton (`auth`, `current`/`navigate`, `client`, `subscribe`/`wsStatus`).
- When a test needs to assert which arguments a singleton method was called
  with — the indirect `(...args) => mockX(...args)` gives a stable spy
  handle.
- When tests need to vary the singleton's state per test case — use
  `let mockOrgs` + getter inside the mock factory.

## When NOT to Use

- Unit tests for pure functions in the same module — those don't need any
  mock.
- Tests of `auth.svelte.ts` itself or `router.svelte.ts` itself — they
  `await import('$lib/auth.svelte')` after `vi.resetModules()` and use
  `vi.doMock` for transitive deps (`auth.test.ts:52`).

## Common Violations

- `vi.mock('$lib/api/client', () => ({ client: { GET: vi.fn() } }))` with no
  outer `mockGET` — the test cannot get a handle to the spy, so it can't
  stub responses or assert calls.
- Mocking `auth` with literal property values (`auth: { orgs: [...] }`) when
  tests want to vary `orgs` — once the factory returns, the property is
  fixed; rerunning the factory across tests doesn't happen.
- Forgetting to mock `$lib/router.svelte` when the component uses `current`
  reactively — the component crashes because `current` is undefined at
  import time.
