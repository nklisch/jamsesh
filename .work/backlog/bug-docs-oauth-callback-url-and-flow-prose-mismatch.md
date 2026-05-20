---
id: bug-docs-oauth-callback-url-and-flow-prose-mismatch
created: 2026-05-19
tags: [bug, documentation, auth, oauth, self-host]
---

# Docs tell self-hosters to register the wrong GitHub OAuth callback URL

A self-hoster trying to enable GitHub OAuth followed the instructions in
`deploy/compose/.env.example` and `docs/SELF_HOST.md` to register the
Authorization callback URL as `https://<host>/api/auth/oauth/callback`,
then hit `redirect_uri_mismatch` from GitHub on the first sign-in
attempt. The Go code sends a different `redirect_uri` than the docs
say to register, so GitHub rejects the exchange.

## Code (authoritative) vs docs

**The portal sends this `redirect_uri` to GitHub** (`internal/portal/auth/oauth.go:74`):

```go
redirectURI := h.portalURL + "/auth/oauth/callback"   // no /api/
```

That value is stored in the OAuth state row and passed to
`provider.AuthorizeURL(nonce, redirectURI)`. GitHub redirects the
user-agent there with `?code=...&state=...`, the SPA route picks up
the params and POSTs them to `/api/auth/oauth/callback` — the backend
endpoint where the actual code-for-token exchange happens.

That endpoint URL **does** exist (`oauth.go:86`,
`internal/portal/auth/oauth_test.go:199`, `docs/openapi.yaml:1565`,
`docs/PROTOCOL.md:93`) — but it's the backend POST endpoint, NOT what
should be registered on GitHub's OAuth app.

**The docs say register the wrong URL:**

- `deploy/compose/.env.example:16`:
  > `#   https://<JAMSESH_DOMAIN>/api/auth/oauth/callback`
- `docs/SELF_HOST.md:294` (under "Registering the GitHub OAuth app"):
  > `https://<your-portal-host>/api/auth/oauth/callback`

## Bonus bug: SELF_HOST.md §4 gets the flow architecture wrong

`docs/SELF_HOST.md:296-297` claims:

> "GitHub redirects the user's browser directly to this portal endpoint
> (server-side exchange — there is no SPA-side redirect hop)."

That's the opposite of what the code does. The `redirect_uri` is a
frontend SPA route; the SPA reads code+state from the query string and
POSTs them to the backend at `/api/auth/oauth/callback`. There IS a
SPA-side hop. The prose was likely written assuming a pure server-side
flow that was never built (or was refactored out).

`docs/SELF_HOST.md:286` separately says:

> `POST https://<your-portal-host>/api/auth/oauth/callback`

That line is correct in isolation (it's the backend endpoint), but the
surrounding prose conflates it with the OAuth-app registration URL, so
the whole section reads as if those two were the same URL.

## Fix scope

Docs-only — the code is the source of truth. Three coordinated edits:

1. **`deploy/compose/.env.example:14-16`**: change the comment to register
   `https://<JAMSESH_DOMAIN>/auth/oauth/callback` (no `/api/`).

2. **`docs/SELF_HOST.md` §4**: rewrite to separate two distinct URLs and
   describe the SPA-hop flow accurately:
   - **Register on the OAuth app:** `https://<portal>/auth/oauth/callback`
     (frontend route; GitHub redirects the user-agent here).
   - **Backend endpoint** (informational, not user-configurable):
     `POST https://<portal>/api/auth/oauth/callback` — the SPA POSTs
     code+state here to exchange for a session.
   - Remove the "no SPA-side redirect hop" sentence; replace with a one-
     paragraph explanation of the SPA-hop flow.

3. **`README.md` quickstart** (if it mentions the OAuth callback): audit
   for the same drift. Spot-check before this story closes.

## Reproducer

Followed by the reporter:
1. Register a GitHub OAuth app with callback URL
   `https://jamsesh.dev/api/auth/oauth/callback` (per current docs).
2. `docker compose up -d`, set `JAMSESH_OAUTH_GITHUB_CLIENT_*`.
3. Click "Sign in with GitHub" on the portal home page.
4. GitHub returns: `redirect_uri_mismatch`.

Workaround the reporter used: re-register the OAuth app with
`/auth/oauth/callback` (no `/api/`). Client ID and secret unchanged.

## Out of scope (for this story)

- Changing the code to use `/api/auth/oauth/callback` as the OAuth
  `redirect_uri` directly. Would require the backend to handle a GET
  with query params (currently POST-only), would break the established
  SPA-hop flow, and would invalidate every deploy that's already
  workarounded by registering `/auth/oauth/callback`. The code is the
  right pattern; only the docs need to catch up.

- Adding a startup-time self-check that logs the registered OAuth
  callback URL the portal will send, so self-hosters can spot mismatches
  before clicking "Sign in". Nice-to-have, separate work.

## References

- Code site: `internal/portal/auth/oauth.go:74`
- Test confirming the redirect_uri shape:
  `internal/portal/auth/oauth_test.go:492` and `:585`
  (`"https://portal.example.com/auth/oauth/callback"`)
- Backend endpoint (correct location in docs):
  `internal/portal/auth/oauth.go:86`, `docs/openapi.yaml:1565`,
  `docs/PROTOCOL.md:93`
- Wrong URL in docs: `deploy/compose/.env.example:16`,
  `docs/SELF_HOST.md:294`
- Wrong flow prose: `docs/SELF_HOST.md:296-297`
