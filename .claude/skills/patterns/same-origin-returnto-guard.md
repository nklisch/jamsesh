# Same-Origin return_to Validation

When a `return_to` value arrives from a URL query string or sessionStorage
and will be passed to `navigate(...)`, all three auth screens validate it
with the same predicate —
`returnTo && returnTo.startsWith('/') && !returnTo.startsWith('//')` — and
fall back to a safe default (`/` or `/login`) when the check fails.

## Rationale

Open-redirect vulnerabilities: a hostile invite link could include
`?return_to=https://evil.com` or `?return_to=//evil.com` (the latter is
protocol-relative — browsers expand `//evil.com` to `https://evil.com`).
Both forms would let an attacker redirect the user off-site after a
successful login.

The two-condition check enforces "same-origin, root-relative" — must start
with `/`, must NOT start with `//`. The `startsWith('/')` alone is
insufficient because `//evil.com` also starts with `/`; the second clause
is the load-bearing one.

## Examples

### Example 1: read from query string at script init
**File**: `frontend/src/lib/screens/Login.svelte:37`

```ts
const _returnTo = _searchParams.get('return_to');
const returnTo: string | null =
  _returnTo && _returnTo.startsWith('/') && !_returnTo.startsWith('//')
    ? _returnTo
    : null;
```

### Example 2: read from sessionStorage on mount
**File**: `frontend/src/lib/screens/OAuthCallback.svelte:34`

```ts
const storedReturnTo = sessionStorage.getItem('oauth.return_to');
const returnTo =
  storedReturnTo && storedReturnTo.startsWith('/') && !storedReturnTo.startsWith('//')
    ? storedReturnTo
    : null;
```

### Example 3: inline check before navigating
**File**: `frontend/src/lib/screens/MagicLinkExchange.svelte:52`

```ts
const returnTo = searchParams.get('return_to');
if (returnTo && returnTo.startsWith('/') && !returnTo.startsWith('//')) {
  navigate(returnTo);
} else {
  navigate('/login');
}
```

## When to Use

- Any code path that accepts an externally-supplied destination URL (query
  string, sessionStorage, localStorage, postMessage) and passes it to
  `navigate(...)` or `window.location.assign(...)`.
- New auth flows or invite flows that want post-login resumption.

## When NOT to Use

- When the destination is constructed entirely from server-validated IDs
  (e.g. `navigate(\`/orgs/${data.id}/sessions\`)` after a typed
  `client.POST('/api/orgs')`) — the destination is server-controlled, not
  user-controlled.
- For absolute URLs that are deliberately external (OAuth `authorize_url`)
  — those go through `window.location.assign(...)`, not `navigate(...)`,
  and are already trusted because the value came from the server.

## Common Violations

- `navigate(returnTo ?? '/')` with no validation — open redirect:
  `?return_to=https://evil.com` works.
- `if (returnTo) navigate(returnTo)` — same bug.
- `if (returnTo.startsWith('/'))` (missing the `!startsWith('//')` clause)
  — protocol-relative `//evil.com` still works.
- Validating with `new URL(returnTo).origin === window.origin` — heavier,
  and constructs a URL out of attacker input (which is risky in some
  shells).
