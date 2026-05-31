# Fragment Secret Handoff with Immediate Scrub

Browser-delivered one-time secrets are placed in URL fragments, read
client-side, then immediately removed from browser-visible URL state before
exchange.

## Rationale

Fragments are not sent in HTTP requests, so secret tokens avoid server access
logs and proxy logs. The SPA then strips the fragment quickly to reduce exposure
in browser history, screenshots, and DevTools.

## Examples

### Resume URL composes `#rt=<token>`

**File**: `internal/portal/sessionresume/mint.go:162`

```go
u.Fragment = "rt=" + rawToken
return u.String(), nil
```

### Resume screen reads and scrubs fragment

**File**: `frontend/src/lib/screens/ResumeExchange.svelte:47`

```ts
const hash = window.location.hash.slice(1);
const params = new URLSearchParams(hash);
const resumeToken = params.get('rt');
history.replaceState(null, '', window.location.pathname + window.location.search);
```

### Magic-link screen uses the same token fragment shape

**File**: `frontend/src/lib/screens/MagicLinkExchange.svelte:19`

```ts
const hash = window.location.hash.slice(1);
const params = new URLSearchParams(hash);
const token = params.get('token');
history.replaceState(null, '', window.location.pathname);
```

## When to Use

- Links carrying one-time bearer material to an SPA.
- CLI-opened browser handoffs where the URL includes a secret fragment.

## When NOT to Use

- Normal navigation URLs with no secret.
- Server-handled routes that must read the token from the HTTP request.

## Common Violations

- Putting bearer material in query params or path segments.
- Forgetting `history.replaceState` after reading the fragment.
- Rendering the raw token or including it in user-facing errors.

