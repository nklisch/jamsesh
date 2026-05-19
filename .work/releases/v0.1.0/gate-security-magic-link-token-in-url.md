---
id: gate-security-magic-link-token-in-url
kind: story
stage: done
tags: [security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# Magic-link / invite-accept token in URL query string leaks via Referer / history / proxy logs

## Severity
Low

## Domain
Data Protection

## Location
`internal/portal/auth/magic_link.go:107`,
`internal/portal/accounts/orgs.go:191`,
`internal/portal/sessions/invites.go:115`

## Evidence
```go
magicURL := h.portalURL + "/auth/magic-link?token=" + raw
```

Same pattern in `accounts/orgs.go:191`
(`/orgs/.../invites/.../accept?token=...`) and `sessions/invites.go:115`.
URLs with tokens persist in browser history, get forwarded as `Referer:`
when the magic-link landing page links anywhere offsite, and appear in
upstream access logs. Tokens are single-use and short-lived, so impact
is bounded.

## Remediation direction
Land the user on a token-less landing page and POST the token from JS to
the exchange endpoint instead of putting it in the query, or accept the
token in a hash fragment (`#token=...`) which is not sent to the server
or logged.

## Implementation notes

### Approach chosen
URL fragment (`#token=...`) on all three email-composed URLs. Fragments:
- Are not sent to the server (never in access logs, never in `Referer:`)
- Are stripped by modern browsers from `Referer:` header
- Do not appear in proxy access logs

After reading, the SPA clears the hash via `history.replaceState` so
the token does not persist in browser history or developer tools.

### JS-only decision
The fragment approach assumes JavaScript. No no-JS magic-link flow exists
or is specified in `docs/SPEC.md` or `docs/UX.md`. Documented here as
the accepted constraint.

### Server-side changes

Three files, each switching `?token=` → `#token=` in the URL composition:

- `internal/portal/auth/magic_link.go` line 107: magic-link email URL
- `internal/portal/accounts/orgs.go` line 191: org invite accept URL
- `internal/portal/sessions/invites.go` line 115: session invite accept URL

### Go test updates

`internal/portal/auth/magic_link_test.go`:
- `requestAndExtractToken`: prefix changed from `"?token="` → `"#token="`
- `extractTokenFromBody`: same change
- `TestRequestMagicLink_SentBodyContainsURL`: assertion changed to expect `#token=`

The `orgs_test.go` and `invites_test.go` files do not assert the email
URL shape (they test the accept endpoint, not the composed URL), so no
changes needed there.

### Frontend changes

**`InviteAccept.svelte`** (session invite accept landing):
- `onMount` now reads `window.location.hash.slice(1)` instead of
  `window.location.search`, parsed via `URLSearchParams`
- Calls `history.replaceState(null, '', pathname + search)` immediately
  after reading the token to clear the hash

**New: `MagicLinkExchange.svelte`**
A magic-link exchange landing screen that previously did not exist in the
SPA (there was no route for `/auth/magic-link`). The screen:
- Reads token from `window.location.hash`
- Clears hash via `history.replaceState`
- POSTs to `POST /api/auth/magic-link/exchange` via the typed client
- On success: calls `auth.setTokens` and navigates to `?return_to` (if
  present) or `/login` as fallback
- On error: shows the error code with a "Back to sign in" affordance

**`router.svelte.ts`**: added `{ pattern: /^\/auth\/magic-link$/, name: 'magic-link', params: [] }`

**`App.svelte`**:
- Imports and renders `MagicLinkExchange` for `current.name === 'magic-link'`
- Excludes `'magic-link'` from the auth gate (it must be reachable
  unauthenticated — it IS the auth flow)

### Frontend test updates

`InviteAccept.test.ts`:
- `setSearch` helper renamed to `setHash`, now sets `window.location.hash`
  instead of `window.location.search`
- `beforeEach` sets `#token=tok-abc` instead of `?token=tok-abc`
- "token missing from query string" test description updated to "from hash"

New `MagicLinkExchange.test.ts` (9 tests):
- Token read from hash
- Hash cleared via `history.replaceState`
- POST called with extracted token
- `auth.setTokens` called on success, then navigate
- Error states for missing token, invalid token, expired token, no error code

### Test coverage summary
- Go: all three changed packages pass (`auth`, `accounts`, `sessions`)
- Frontend: 387 tests / 38 files — all green
- No e2e / playwright magic-link fixture tests found in codebase

### Note on `GetSessionInvite` API query param
The `GET /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}` endpoint
takes `?token=` as a required query param (OpenAPI spec, in: query). This is
an API-level parameter in the HTTP request from SPA to backend — NOT in the
user-facing browser URL bar. The SPA reads the token from the hash, then
sends it as a query param in the API call. No change needed to the OpenAPI
spec or the backend `GetSessionInvite` handler.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The `GET /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}?token=...`
  API call still carries the token in a query string. The portal's own access
  log is now redacted (parallel story `gate-security-debug-log-redact-tokens`),
  so this is mostly covered, but a CDN/load-balancer in front of the portal
  would still see the unredacted query. If/when CDN proxying becomes a
  deployment concern, switching this API to take the token in a header or
  request body would close that surface.
- Open-redirect guard on `return_to` (`startsWith('/') && !startsWith('//')`)
  is correctly implemented; worth pulling out to a shared helper if the
  pattern appears in more screens later.

**Notes**: The `MagicLinkExchange` component fills a real gap — the route
didn't exist before, only the email composition did. Component handles every
state machine path (missing token, exchange failure, success, return_to
redirect). Hash-clear via `history.replaceState` lands immediately after
read, before any await, so the token can't escape via a re-render. All three
callsites updated consistently. 387 frontend tests + all relevant Go tests
pass.
