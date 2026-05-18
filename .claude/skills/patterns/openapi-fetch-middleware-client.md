# Pattern: openapi-fetch Client with Global Middleware

The frontend has a single shared
`client = createClient<paths>({ baseUrl, fetch: lateFetch })` exported
from `frontend/src/lib/api/client.ts`. Cross-cutting concerns
(bearer-token injection, 401 → sign-out) are attached via
`client.use(middleware)`; all callers (`Login`, `FinalizeView`,
`CommentsTab`, `TreeDag`, etc.) import the same `client` and call
`client.GET('/api/...', { params: { path: {...} } })` with typed
responses. The `fetch` parameter is a `lateFetch` indirection over
`globalThis.fetch` so vitest can stub fetch per-test without
re-creating the client.

## Rationale

A single client instance with attached middleware is the only way to
apply auth uniformly across every typed call without each caller
hand-rolling header injection or 401 handling. The `lateFetch` shim is
forced by openapi-fetch capturing `globalThis.fetch` at `createClient`
time — without the wrapper, `vi.stubGlobal('fetch', ...)` after module
load would have no effect. The two middlewares (`bearerMiddleware`,
`unauthorizedMiddleware`) are the only generic interceptors;
component-specific logic uses the per-call `{ error }` destructure.

## Examples

### Example 1: client definition

**File**: `frontend/src/lib/api/client.ts:39`

```ts
const lateFetch: typeof fetch = (...args) => globalThis.fetch(...args);
export const client = createClient<paths>({ baseUrl, fetch: lateFetch });
client.use(bearerMiddleware);
client.use(unauthorizedMiddleware);
```

### Example 2: typed call site

**File**: `frontend/src/lib/components/TreeDag.svelte:61`

```ts
const { data } = await client.GET('/api/orgs/{orgID}/sessions/{sessionID}/refs', {
  params: { path: { orgID, sessionID } }
});
```

### Example 3: response destructure pattern is uniform

**Files**: `frontend/src/lib/screens/FinalizeView.svelte:107`,
`frontend/src/lib/components/CommentsTab.svelte:25`,
`frontend/src/lib/components/NewSessionDrawer.svelte:41`,
`frontend/src/lib/auth.svelte.ts:53` — every caller destructures
`{ data, error }` from the awaited `client.{GET,POST,PATCH}(...)`.

13+ call sites across screens and components.

## When to Use

- Any new typed API call from the SPA — import `{ client }` from
  `$lib/api/client` and call `client.METHOD(path, options)`.
- New cross-cutting concerns (retry, request-id, observability) — add
  a `Middleware` and attach with `client.use(...)`.

## When NOT to Use

- Endpoints not yet in `docs/openapi.yaml` — promote to a typed call
  once the endpoint lands in the spec.
- Server-side or build-time code where there's no auth or fetch
  context.

## Common Violations

- Creating a second `createClient` somewhere — duplicates the
  bearer-injection logic and gets stale if `auth.token` is read from
  the wrong scope.
- Reading `globalThis.fetch` once at module load instead of through
  `lateFetch` — breaks vitest stubGlobal.
- Hand-rolling `if (response.status === 401)` in callers — drifts from
  the central `auth.signOut()` policy.
