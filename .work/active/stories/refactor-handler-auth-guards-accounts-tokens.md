---
id: refactor-handler-auth-guards-accounts-tokens
kind: story
stage: implementing
tags: [refactor, portal]
parent: refactor-handler-auth-guards
depends_on: [refactor-handler-auth-guards-helpers-and-sessions]
release_binding: null
gate_origin: refactor-design
created: 2026-05-17
updated: 2026-05-17
---

# Auth-Guards — Migrate accounts and tokens handlers

Apply the `handlerauth` helpers to the last remaining consumers:
`internal/portal/accounts/handlers.go` (`/api/me`, org membership endpoints)
and `internal/portal/tokens/handlers.go` (refresh / revoke).

## Files

- Modify: `internal/portal/accounts/handlers.go`
- Modify: `internal/portal/tokens/handlers.go`

## Sites to migrate

- `accounts/handlers.go:GetMe` — currently line 35-69 (uses `RequireAccount`
  only — no org-scope check needed for /api/me).
- `accounts/handlers.go:CreateOrg`, `ListOrgMembers`, `AddOrgMember`, etc.
- `tokens/handlers.go:53-62` and any other 401 sites.

## Implementation notes

- `/api/me` is the only endpoint that does *not* need an org check. Use the
  bare `RequireAccount` for it.
- Tokens endpoints (refresh/revoke) may have their own non-standard auth
  flow (the refresh token comes from the request body, not the bearer). If
  so, mark that handler as deliberately non-migrated with a one-line comment
  pointing at this story.

## Acceptance

- [ ] `go build ./...` passes
- [ ] `go test ./internal/portal/accounts/... ./internal/portal/tokens/...` passes
- [ ] No direct `store.GetOrgMember` / `store.GetSessionMember` calls remain
      in either file (except the deliberately-non-migrated cases, if any)
- [ ] After this story merges, the parent feature can be advanced to
      `stage: review`

## Risk

LOW.

## Rollback

`git revert` each file independently.
