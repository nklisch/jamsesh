# openapi-fetch Destructure-and-Branch Result

Every typed API call in screens and components destructures `{ data, error }`
(sometimes `response`) from `await client.GET/POST/PATCH(...)`, then branches
on `if (data)` for the happy path and on `if (error)` (often with a
typed-property cast like `(error as { error?: string } | undefined)?.error`)
for the failure path. The catch block (when present) is reserved for thrown
network failures, not for HTTP error responses.

## Rationale

`openapi-fetch` never throws on a 4xx/5xx — it returns
`{ data: undefined, error: <typed envelope> }`. Calls that mix `try`/`catch`
with the destructure either swallow network errors silently or double-handle
the same condition. The destructure-and-branch shape forces the author to
think about three distinct outcomes: typed success (`data`), typed server
failure (`error`), and thrown transport failure (catch).

The error envelope itself is not strongly typed in `types.gen.ts` for many
endpoints (until those endpoints declare their error schema), so a narrow
inline cast `(error as { error?: string; message?: string })` makes the
discriminating field readable without a generic helper.

## Examples

### Example 1: POST with success branch + cast error envelope + network catch
**File**: `frontend/src/lib/screens/Home.svelte:40`

```ts
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
```

### Example 2: POST with `response` for status-code discrimination
**File**: `frontend/src/lib/screens/InviteAccept.svelte:82`

```ts
const { data, error, response } = await client.POST(
  '/api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept',
  { params: { path: {...} }, body: { token } },
);

if (data) {
  navigate(`/orgs/${orgId}/sessions/${sessionId}`);
  return;
}

const errCode = (error as { error?: string } | undefined)?.error;
if (response?.status === 403 && errCode === 'auth.org_membership_required') {
  viewState = 'rejection';
  return;
}
```

### Example 3: GET with two-call Promise.all and per-call error narrow
**File**: `frontend/src/lib/screens/OrgSettings.svelte:22`

```ts
const [orgResult, membersResult] = await Promise.all([
  client.GET('/api/orgs/{orgID}', { params: { path: { orgID: orgId } } }),
  client.GET('/api/orgs/{orgID}/members', { params: { path: { orgID: orgId } } }),
]);

if (orgResult.error) {
  loadError = 'Failed to load org settings.';
  return;
}
if (membersResult.error) {
  loadError = 'Failed to load org members.';
  return;
}

org = orgResult.data;
```

### Example 4: POST with typed error narrow on field
**File**: `frontend/src/lib/screens/OAuthCallback.svelte:48`

```ts
const { data, error } = await client.POST('/api/auth/oauth/callback', {
  body: { provider, code, state },
});

if (data) {
  auth.setTokens(data.access_token, data.refresh_token);
  await auth.loadCurrentUser();
  navigate(returnTo ?? '/');
  return;
}

errorCode = (error as { error?: string } | undefined)?.error ?? 'exchange_failed';
viewState = 'error';
```

## When to Use

- Every `client.GET/POST/PATCH/DELETE/PUT` call site that needs to handle
  failure.
- When you need to discriminate on HTTP status (use the `response`
  destructure variant).
- When the error envelope's field shape is loosely typed and you only need
  one or two fields.

## When NOT to Use

- Pure fire-and-forget calls where neither `data` nor `error` is read —
  `await client.POST(...)` alone is fine (e.g. `auth.svelte.ts:148` for
  `/api/auth/ws-ticket` only destructures `data`).
- Inside the `client.ts` middleware itself — middleware sees
  `Request`/`Response`, not the destructured shape.

## Common Violations

- Wrapping the whole call in `try { ... } catch { setError(err.message) }`
  and never checking `error` — silently treats every 4xx/5xx as success
  because openapi-fetch never throws on them.
- Using `if (!error)` as the happy-path branch when `data` is also nullable
  — produces a TS narrowing where `data` is still `T | undefined` inside
  the block.
- Treating `error` as `Error` and reading `.message` directly — `error` is
  the typed envelope from the OpenAPI spec, not a JS `Error` object, so
  `.message` is whatever the server's `ErrorEnvelope` declared.

## See Also

- `openapi-fetch-middleware-client` — the upstream pattern that provides the
  `client` instance these calls destructure from.
