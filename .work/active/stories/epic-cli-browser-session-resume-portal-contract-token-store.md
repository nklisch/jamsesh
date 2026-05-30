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
  `GetResumeTokenByHash :one`, `ConsumeResumeToken :exec` (atomic single-use:
  `UPDATE … SET used_at=? WHERE id=? AND used_at IS NULL`). Dual-dialect parity
  per the `dual-dialect-mirror-queries` pattern.
- Regenerate sqlc; add the store adapter methods in `internal/db/store/`
  (sqlite + postgres adapters, `wrap1`/`wrapList` per `adapter-wrap-helpers`).

## Acceptance criteria

- [ ] Dual-dialect parity: identical query names/columns across sqlite + postgres.
- [ ] `sqlc generate` clean; `go build ./...` clean.
- [ ] `ConsumeResumeToken` is atomic single-use (second consume affects 0 rows).
- [ ] Store stores only the token **hash**, never the raw token.
- [ ] Adapter methods covered by tests (both dialects where the suite runs them).
