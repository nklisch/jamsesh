---
id: epic-portal-foundation-auth-flows-oauth-provider-github
kind: story
stage: implementing
tags: [portal, security]
parent: epic-portal-foundation-auth-flows
depends_on: [epic-portal-foundation-auth-flows-sender-and-magic-link]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Auth Flows — OAuth Provider + GitHub

## Scope

Build the OAuth `Provider` interface, the GitHub implementation,
the state-nonce storage (with new `oauth_state` table), and the
`/api/auth/oauth/start` + `/api/auth/oauth/callback` REST
endpoints.

## Units delivered

- `internal/portal/oauth/provider.go` — Provider interface +
  Identity struct
- `internal/portal/oauth/github.go` — GitHub implementation
- `internal/portal/oauth/state.go` — state nonce helper using
  the new oauth_state table
- `internal/db/migrations/sqlite/00003_oauth_state.sql` +
  postgres variant — schema addition
- `db/schema/{sqlite,postgres}.sql` (edit) — append `oauth_state`
- `db/queries/{sqlite,postgres}/oauth_state.sql` — Insert,
  Consume (delete-where-matches), Cleanup
- Regenerate sqlitestore + pgstore
- `internal/db/store/store.go` (edit) — add `OAuthStateStore`
  sub-interface
- Both adapters updated
- `internal/portal/auth/oauth.go` — handler logic
- `docs/openapi.yaml` (edit) — `/api/auth/oauth/start` and
  `/api/auth/oauth/callback`
- Regenerate server.gen.go + types.gen.ts
- `cmd/portal/main.go` (edit) — construct Provider, wire into
  router
- Tests

## Acceptance Criteria

- [ ] `POST /api/auth/oauth/start` with `{provider:"github"}`
      returns `{authorize_url}` with state nonce embedded
- [ ] State nonce stored in `oauth_state` table with 5-minute TTL
- [ ] `POST /api/auth/oauth/callback` with valid state + code
      exchanges via Provider.Exchange, calls FindOrProvision
      (shared helper from previous story), issues TokenPair
- [ ] State nonce is consumed (deleted) on first use — second
      callback with same state returns 400
- [ ] Expired state nonces (>5min) return 400
- [ ] Provider.Exchange invokes GitHub API correctly (test against
      `httptest.NewServer` mocking `/login/oauth/access_token` and
      `/user`, `/user/emails`)
- [ ] FindOrProvision is reused (not re-implemented) — verified
      by walking imports
- [ ] make generate clean afterwards

## Notes

- Reuses the `FindOrProvision` helper from the
  `sender-and-magic-link` story — that's why this story depends
  on it.
- GitHub OAuth requires a registered application (client_id +
  client_secret) — config is per-deployment, documented in
  `docs/SELF_HOST.md` already.
- For `Identity.ProviderID` from GitHub: use the numeric `id`
  field from `/user` (stable across name changes) as a string.
- Primary email selection: GET `/user/emails` returns array; pick
  the `primary: true` entry.
