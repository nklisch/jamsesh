---
id: epic-cli-browser-session-resume-portal-contract-token-store
kind: story
stage: implementing
tags: [portal, security]
parent: epic-cli-browser-session-resume-portal-contract
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# resume_tokens dual-dialect store

Implements **Unit 1** of `epic-cli-browser-session-resume-portal-contract`. See
the feature body for the schema + query shapes.

## Scope

- `db/schema/{sqlite,postgres}.sql`: add the `resume_tokens` table
  (`id, token_hash UNIQUE, session_id, org_id, account_id, issued_at,
  expires_at, used_at`) — mirror `magic_link_tokens`.
- `db/queries/{sqlite,postgres}/resume_tokens.sql`: `CreateResumeToken :one`,
  `GetResumeTokenByHash :one`, and a **winner-returning** `ConsumeResumeToken
  :one` — `UPDATE resume_tokens SET used_at=? WHERE token_hash=? AND used_at IS
  NULL AND expires_at > ? RETURNING *`. Dual-dialect parity per
  `dual-dialect-mirror-queries`. ⚠ Do NOT use `:exec` (the generated `:exec`
  discards rows-affected → concurrent exchanges could double-issue); the row
  returned by `RETURNING` is the single-use winner signal the exchange handler
  relies on.
- Regenerate sqlc; add the store adapter methods in `internal/db/store/`
  (sqlite + postgres adapters, `wrap1`/`wrapList` per `adapter-wrap-helpers`).

## Acceptance criteria

- [ ] Dual-dialect parity: identical query names/columns across sqlite + postgres.
- [ ] `sqlc generate` clean; `go build ./...` clean.
- [ ] `ConsumeResumeToken` is a winner-returning `:one` (RETURNING a row only on
      the first valid consume); a second consume returns no row (single-use), and
      an expired token returns no row. Test concurrent consume → exactly one row.
- [ ] Store stores only the token **hash**, never the raw token.
- [ ] Adapter methods covered by tests (both dialects where the suite runs them).
