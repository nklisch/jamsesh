---
id: epic-portal-foundation-auth-flows-oauth-provider-github
kind: story
stage: done
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

## Implementation notes

### Files created / modified

- `internal/portal/oauth/provider.go` — `Provider` interface, `Identity`
  struct, `ErrExchange` error type.
- `internal/portal/oauth/github.go` — `GitHub` struct implementing
  `Provider`. Takes `GitHubOptions` with injectable `HTTPClient` and
  `BaseURL` for test isolation via `httptest.Server`. Three GitHub API
  calls: token exchange, `/user`, `/user/emails`. Primary+verified email
  selection with fallback.
- `internal/portal/oauth/state.go` — `GenerateNonce`, `StoreState`,
  `ConsumeState` helpers over `store.OAuthStateStore`. 5-minute TTL.
- `db/schema/sqlite.sql` + `db/schema/postgres.sql` — `oauth_state` table
  appended.
- `internal/db/migrations/sqlite/00003_oauth_state.sql` +
  `internal/db/migrations/postgres/00003_oauth_state.sql` — Goose
  Up/Down migrations.
- `db/queries/sqlite/oauth_state.sql` +
  `db/queries/postgres/oauth_state.sql` — `InsertOAuthState :exec`,
  `ConsumeOAuthState :one` (DELETE…RETURNING — atomic consume), 
  `CleanupExpiredOAuthState :exec`.
- `internal/db/sqlitestore/` + `internal/db/pgstore/` — regenerated via
  `sqlc generate`.
- `internal/db/store/store.go` — `OAuthState` domain type,
  `InsertOAuthStateParams`, `OAuthStateStore` sub-interface added. `Store`
  embeds `OAuthStateStore`.
- `internal/db/store/sqlite_adapter.go` +
  `internal/db/store/postgres_adapter.go` — `OAuthStateStore` implemented.
- `internal/portal/auth/oauth.go` — `OAuthHandler` with `StartOAuth` and
  `OauthCallback` methods satisfying `openapi.StrictServerInterface`.
  Start: generates nonce → stores state → returns `authorize_url`. 
  Callback: consumes nonce atomically → checks expiry + provider match →
  calls `provider.Exchange` → `FindOrProvision` → `tokens.Service.Issue`
  → TokenPair.
- `docs/openapi.yaml` — `OAuthStartBody`, `OAuthStartResponse`,
  `OAuthCallbackBody` schemas; `/api/auth/oauth/start` (POST, public) and
  `/api/auth/oauth/callback` (POST, public) paths.
- `internal/api/openapi/server.gen.go` — regenerated via `make
  generate-api-go`. New `StartOAuth` and `OauthCallback` strict interface
  methods.
- `frontend/src/lib/api/types.gen.ts` — regenerated via `make
  generate-api-ts`.
- `internal/portal/config/config.go` — `OAuthConfig`, `GitHubOAuthConfig`
  structs added to `Config`. Env vars `JAMSESH_OAUTH_GITHUB_CLIENT_ID`
  and `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET`.
- `cmd/portal/main.go` — `OAuthHandler` added to `combinedHandler`;
  GitHub provider constructed from config (nil when unconfigured); routes
  `/auth/oauth/start` and `/auth/oauth/callback` registered in public group.

### Design choices

- **Provider seam**: adding Google/OIDC is a new file implementing
  `Provider` + a config key + one line in the providers map.
- **Nil provider entry**: the map always has the key `"github"` but the
  value is `nil` when `ClientID`/`ClientSecret` are empty; the start
  handler returns 503 rather than 404 — clearer intent for operators who
  haven't configured credentials yet.
- **Atomic nonce consume**: `DELETE … RETURNING` is used for
  `ConsumeOAuthState` — both SQLite and Postgres support this. A second
  callback with the same nonce gets `ErrNotFound` immediately, with no
  race window.
- **Expiry check after consume**: the nonce is deleted before the expiry
  is checked so an expired nonce cannot be replayed even if two requests
  arrive simultaneously.
- **Test isolation**: `GitHubOptions.BaseURL` and `GitHubOptions.HTTPClient`
  are the only hooks needed; no interface indirection on the HTTP layer.

### Test coverage

- `internal/portal/oauth/github_test.go`: Name, AuthorizeURL (with and
  without BaseURL), Exchange success (display name fallback, primary email
  selection), Exchange error paths (token error, /user error, /user/emails
  error).
- `internal/portal/auth/oauth_test.go`: start returns authorize_url, start
  400 for unknown provider, start 503 for unconfigured provider, callback
  full happy path, nonce consumed on first use (second → 400), invalid
  state → 400, provider mismatch → 400, exchange error → 500, state store
  round-trip, ConsumeState on nonexistent → error.

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Provider seam clean. Atomic DELETE…RETURNING for nonce consume avoids replay windows. Nil-provider-entry + 503 is the right defensive default for unconfigured deployments. Test injection via HTTPClient + BaseURL is minimal and effective.
