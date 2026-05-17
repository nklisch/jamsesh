---
id: epic-portal-foundation-data-layer
kind: feature
stage: drafting
tags: [portal]
parent: epic-portal-foundation
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Foundation — Data Layer

## Brief

The portal's persistence substrate. Establishes sqlc as the type system over
raw SQL, the dual-dialect pattern (SQLite default + Postgres via driver swap)
with per-dialect query packages selected at build or runtime, the initial
schema for the core auth entities (`orgs`, `accounts`, `sessions`, `members`,
`oauth_tokens`), and the org_id-in-WHERE convention that structurally
prevents cross-tenant leakage in every query.

This feature also delivers the migration tool (or convention) that brings a
fresh SQLite or Postgres database to the current schema, plus the connection
pool / driver setup helpers the HTTP skeleton's middleware will consume.

It does NOT cover schemas owned by other epics (`comments`,
`conflict_events`, `events`, `presence`, `invites` belong to
`epic-portal-api`; per-session repo storage on disk belongs to
`epic-portal-git`). It does NOT cover the HTTP-side middleware that uses
this data layer — that's the http-skeleton feature.

## Epic context

- Parent epic: `epic-portal-foundation`
- Position in epic: linchpin feature — every other feature in this epic and
  every sibling epic that touches persistence depends on the sqlc patterns
  and the org_id discipline locked here.

## Foundation references

- `docs/SPEC.md` — Stack > Backend (sqlc, dual-dialect), Hard constraints
  (multi-tenant by design)
- `docs/ARCHITECTURE.md` — Data layer (multi-tenancy) section
- `docs/SECURITY.md` — Authorization > MCP and REST API authorization

## Inherited epic design decisions

The data layer inherits these decisions from epic-portal-foundation:

- **Token storage**: opaque random tokens, hashed at rest in `oauth_tokens`.
  No JWTs. Refresh tokens stored similarly; revocation is row deletion.
- **Multi-org per user**: the schema supports a many-to-many between
  accounts and orgs via the `members` table; "current org" is never
  stored — it's always taken from the URL path.
- **First-user bootstrap**: no special bootstrap state in the schema;
  first sign-in creates an org row and a member row like any other signup.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
