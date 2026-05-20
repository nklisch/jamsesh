---
id: bug-docs-oauth-callback-url-and-flow-prose-mismatch
kind: story
stage: done
tags: [bug, documentation, auth, oauth, self-host]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-19
updated: 2026-05-19
---

# Docs tell self-hosters to register the wrong GitHub OAuth callback URL

## Brief

A self-hoster trying to enable GitHub OAuth followed the instructions in
`deploy/compose/.env.example` and `docs/SELF_HOST.md` to register the
GitHub OAuth app Authorization callback URL as
`https://<host>/api/auth/oauth/callback`, then hit `redirect_uri_mismatch`
from GitHub on the first sign-in attempt. The Go code sends a *different*
`redirect_uri` value than the docs say to register — so GitHub rejects
the exchange.

The bug has two parts:

1. **Wrong URL** in `deploy/compose/.env.example:16` and
   `docs/SELF_HOST.md:294`. Both say to register `/api/auth/oauth/callback`;
   the actual value the portal sends is `/auth/oauth/callback` (no `/api/`),
   per `internal/portal/auth/oauth.go:74`.

2. **Wrong flow architecture prose** in `docs/SELF_HOST.md:296-297`. The
   docs claim "GitHub redirects the user's browser directly to this portal
   endpoint (server-side exchange — there is no SPA-side redirect hop)."
   That's the opposite of what the code does. The `redirect_uri` is a
   frontend SPA route; the SPA reads code+state from the query string and
   POSTs them to the backend at `/api/auth/oauth/callback`. There IS a
   SPA-side hop.

The code is the source of truth (standard SPA + server-side-exchange
pattern). Only the docs need to catch up.

## Strategic decisions

Resolved at scope.

- **Code is authoritative; docs catch up.** Flipping the code to use
  `/api/auth/oauth/callback` as the OAuth `redirect_uri` directly would
  require the backend to handle a GET with query params (currently
  POST-only), would break the established SPA-hop flow, and would
  invalidate every deploy that's already worked around this by
  registering `/auth/oauth/callback`. The current architecture is the
  standard pattern; only the docs need to align.
- **Separate the two URLs clearly in `SELF_HOST.md` §4.** The bug is
  partly a documentation conflation: the section presents both
  `/auth/oauth/callback` (OAuth app registration value, frontend route)
  and `/api/auth/oauth/callback` (backend POST endpoint) as if they were
  one URL. The rewrite must call out the two distinct URLs and the
  SPA-hop flow that connects them.
- **No code change.** All edits are docs-only.

## Acceptance criteria

- [ ] `deploy/compose/.env.example:14-16` updated to instruct registering
      `https://<JAMSESH_DOMAIN>/auth/oauth/callback` (no `/api/`). Comment
      wording preserved otherwise; only the URL changes.
- [ ] `docs/SELF_HOST.md` §4 ("OAuth callback URLs") rewritten to:
  - Separate the two URLs cleanly: the **OAuth app registration URL**
    (`/auth/oauth/callback`, frontend route) vs the **backend code-exchange
    endpoint** (`POST /api/auth/oauth/callback`, informational only — not
    user-configurable).
  - Replace the incorrect "no SPA-side redirect hop" sentence with a brief,
    accurate description of the SPA-hop flow:
    > GitHub redirects the user's browser to the SPA route
    > `/auth/oauth/callback?code=...&state=...`. The SPA reads the query
    > params and POSTs them to the backend at `/api/auth/oauth/callback`,
    > which validates the state nonce, exchanges the code with GitHub,
    > and returns a session token.
  - The "Registering the GitHub OAuth app" subsection updated to register
    `https://<your-portal-host>/auth/oauth/callback`.
  - Preserve the client-id / client-secret env-var table and the
    reverse-proxy guidance unchanged.
- [ ] `README.md`: audit for any reference to `/api/auth/oauth/callback` as
      a registration URL. If the quickstart mentions the OAuth callback,
      fix to `/auth/oauth/callback`. If it doesn't reference it (likely),
      log "no README drift" in implementation notes and move on.
- [ ] Cross-check no other docs source got missed:
      `grep -rn "api/auth/oauth/callback" docs/ deploy/ README.md` —
      every remaining match should be in the context of the backend POST
      endpoint (correct usage), not the OAuth registration URL (wrong
      usage). Surface any unexpected hits in implementation notes.
- [ ] No code change. Verify `internal/portal/auth/oauth.go` and
      `internal/portal/auth/oauth_test.go` are unchanged before commit.

## Reproducer

Followed by the reporter:
1. Register a GitHub OAuth app with callback URL
   `https://jamsesh.dev/api/auth/oauth/callback` (per current docs).
2. `docker compose up -d`, set `JAMSESH_OAUTH_GITHUB_CLIENT_*`.
3. Click "Sign in with GitHub" on the portal home page.
4. GitHub returns: `redirect_uri_mismatch`.

Workaround the reporter used (also the production-correct config):
re-register the OAuth app with `/auth/oauth/callback` (no `/api/`).
Client ID and secret unchanged.

## References

- Code site (authoritative): `internal/portal/auth/oauth.go:74` —
  `redirectURI := h.portalURL + "/auth/oauth/callback"`
- Tests confirming the redirect_uri shape:
  - `internal/portal/auth/oauth_test.go:492` and `:585` —
    `"https://portal.example.com/auth/oauth/callback"`
  - `internal/portal/oauth/github_test.go:115` —
    `g.AuthorizeURL("mynonce", "https://portal.example.com/auth/oauth/callback")`
- Backend endpoint (correctly documented elsewhere):
  - `internal/portal/auth/oauth.go:86` — `// OauthCallback implements POST /api/auth/oauth/callback.`
  - `docs/openapi.yaml:1565` — defines the `/api/auth/oauth/callback` path
  - `docs/PROTOCOL.md:93` — documents it as the POST endpoint
- Wrong URL in docs:
  - `deploy/compose/.env.example:16`
  - `docs/SELF_HOST.md:294`
- Wrong flow prose: `docs/SELF_HOST.md:296-297`

## Notes

- The `docs/openapi.yaml` and `docs/PROTOCOL.md` entries for
  `/api/auth/oauth/callback` are correct as-is — they document the
  backend POST endpoint, which is real. Don't change them.
- The reporter has already worked around this on their server by
  re-registering the OAuth app with the correct URL. Their personal
  `/home/nathan/CLAUDE.md` notes the workaround for future operators
  hitting the same wall before this fix lands.

## Implementation notes

- **`.env.example` edit**: line 16 — changed
  `https://<JAMSESH_DOMAIN>/api/auth/oauth/callback` to
  `https://<JAMSESH_DOMAIN>/auth/oauth/callback` (removed `/api/`).
  Surrounding lines unchanged.

- **`docs/SELF_HOST.md` §4 edit**: lines 280-325 (section was ~280-313;
  the rewrite expanded it slightly). Replaced the entire section body with:
  - An opening paragraph naming the two-stage SPA-hop flow and explicitly
    distinguishing the two URLs before any step-by-step instructions.
  - A bullet list separating the OAuth app registration URL
    (`/auth/oauth/callback`) from the backend code-exchange endpoint
    (`POST /api/auth/oauth/callback`, informational only).
  - Updated step 2 of "Registering the GitHub OAuth app" to use
    `/auth/oauth/callback` (no `/api/`), plus the accurate SPA-hop flow
    description replacing the false "no SPA-side redirect hop" sentence.
  - Preserved the env-var table and reverse-proxy guidance unchanged.

- **README audit**: `grep -in "oauth\|callback" README.md` returned two
  hits — line 55 (`OAuth or email creds` in a comment) and line 62 (prose
  pointing to SELF_HOST.md). Neither references the OAuth callback
  registration URL. No README drift — no changes needed.

- **Cross-check grep** (`grep -rn "api/auth/oauth/callback" docs/ deploy/ README.md`):
  Four remaining hits, all correct-usage contexts:
  - `docs/PROTOCOL.md:93` — documents the POST endpoint
  - `docs/openapi.yaml:1565` — OpenAPI path definition
  - `docs/SELF_HOST.md:295` — informational "not user-configurable" note
  - `docs/SELF_HOST.md:309` — SPA-hop flow description
  No registration-URL misuses remain.

- **No code changes.** `git status` shows only `deploy/compose/.env.example`
  and `docs/SELF_HOST.md` modified (plus pre-existing unrelated
  `scheduled_tasks.lock` and untracked `.antigravitycli/`).

## Out of scope

- **Changing the code to use `/api/auth/oauth/callback` as the OAuth
  `redirect_uri` directly.** See Strategic decisions — would require
  the backend to handle a GET with query params, break the SPA-hop
  flow, and invalidate existing workarounded deploys.
- **Adding a startup-time self-check** that logs the registered OAuth
  callback URL the portal will send, so self-hosters can spot
  mismatches before clicking "Sign in". Nice-to-have, separate work.
- **A regression test** that asserts the `redirect_uri` constant
  matches the documented value. Useful but belongs to a broader docs/
  ↔ code drift-detection story.

## Review (2026-05-19)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Clean three-change diff: one-line URL fix in
`deploy/compose/.env.example`, surgical §4 rewrite of
`docs/SELF_HOST.md` that separates the OAuth-app registration URL
(frontend SPA route) from the backend POST endpoint and replaces the
inaccurate "no SPA-side redirect hop" sentence with an accurate
SPA-hop flow description. The opening prose is actually stronger than
the acceptance criteria asked for — it leads with "these are two
distinct URLs — confusing them causes GitHub to reject sign-in with
`redirect_uri_mismatch`", which preempts the exact mistake the
reporter (and every future operator) would have made. Cross-check
grep confirmed every remaining `/api/auth/oauth/callback` mention is
backend-endpoint usage, not registration-URL drift. No code touched
(`internal/portal/auth/oauth.go` and tests verified unchanged). What's
now possible: future self-hosters following the official docs register
the correct OAuth callback URL on the first attempt; the
`redirect_uri_mismatch` failure mode the reporter hit no longer
ships.
