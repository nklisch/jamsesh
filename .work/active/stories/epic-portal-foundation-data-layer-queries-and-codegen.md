---
id: epic-portal-foundation-data-layer-queries-and-codegen
kind: story
stage: implementing
tags: [portal]
parent: epic-portal-foundation-data-layer
depends_on: [epic-portal-foundation-data-layer-schema-and-migrations]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Data Layer — Queries and Codegen

## Scope

Author the initial sqlc query files (per-dialect) for every table created
by the schema-and-migrations story, run `sqlc generate`, and commit the
generated Go packages.

After this story, `internal/db/sqlitestore` and `internal/db/pgstore`
both exist with structurally-identical `Querier` interfaces.

## Units delivered

- **Unit 5**: query files in `db/queries/sqlite/` and `db/queries/postgres/`
- **Unit 11**: `Makefile` target `generate` running `sqlc generate`;
  committed generated packages under `internal/db/sqlitestore/` and
  `internal/db/pgstore/`

## Query surface (per dialect, same names)

- `orgs.sql`: CreateOrg, GetOrgByID, GetOrgBySlug
- `accounts.sql`: CreateAccount, GetAccountByID, GetAccountByEmail,
  GetAccountByGitHubUserID, UpdateAccountDisplayName
- `org_members.sql`: AddOrgMember, GetOrgMember, ListOrgsForAccount,
  ListOrgMembers, RemoveOrgMember
- `sessions.sql`: CreateSession, GetSession, ListSessionsForOrg,
  UpdateSessionStatus, SetSessionBaseSHA
- `session_members.sql`: AddSessionMember, GetSessionMember,
  ListSessionMembers, RemoveSessionMember,
  ListSessionMembershipsForAccount
- `oauth_tokens.sql`: CreateOAuthToken, GetOAuthTokenByHash,
  TouchOAuthTokenLastUsed, RevokeOAuthToken,
  RevokeAllOAuthTokensForAccount, ListOAuthTokensForAccount
- `magic_link_tokens.sql`: CreateMagicLinkToken,
  GetMagicLinkTokenByHash, ConsumeMagicLinkToken

## Acceptance Criteria

- [ ] All query files compile cleanly under `sqlc generate`
- [ ] Generated `Querier` interface in `sqlitestore` and `pgstore` has
      identical method signatures (verify by writing a no-op file that
      declares `var _ sqlitestore.Querier = (pgstore.Querier)(nil)` or
      equivalent type-equality assertion)
- [ ] Every query against `sessions` or `session_members` carries
      `org_id` in WHERE, except the documented exception
      `ListSessionMembershipsForAccount`
- [ ] `make generate && git diff --exit-code` is green (generated code
      committed)
- [ ] Generated packages export `Queries` and a `New(DBTX) *Queries`
      constructor

## Notes

- sqlc per-engine type overrides (defined in parent feature's Unit 1)
  pin every timestamp column to `time.Time`. If the generated signatures
  diverge anyway, fix the overrides — do NOT add a translation layer
  inside the adapters as a workaround.
- The query files are the canonical home for the org_id-in-WHERE
  discipline. A reviewer can scan one directory and verify the rule.
