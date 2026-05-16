---
id: epic-portal-foundation
kind: epic
stage: drafting
tags: [portal, security]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Foundation

## Brief

The portal's foundation layer. Establishes multi-tenancy at the data layer
(orgs, accounts, sessions, members tables with `org_id` boundaries enforced
through sqlc-generated queries), the HTTP server skeleton (TLS termination,
routing, middleware, structured logging, the standardized JSON error
contract), and user authentication (OAuth flow with browser handoff +
magic-link flow for headless self-host environments).

The user OAuth token issued by this epic becomes the single credential used
across the entire system: Bearer auth for the MCP endpoint, Bearer auth for
the REST API, HTTP Basic auth (token-as-password) for git push. Token
issuance, refresh, and revocation all live here.

This epic does NOT cover the git smart-HTTP server (`epic-portal-git`), the
auto-merger (`epic-auto-merger`), or any session/comment API endpoints
(`epic-portal-api`). It's the substrate everything else stands on.

## Foundation references

- `docs/SPEC.md` — Stack, Auth model, Hard constraints
- `docs/ARCHITECTURE.md` — Portal component (REST API + Data store subcomponents)
- `docs/SECURITY.md` — Authentication, Authorization, Self-host security posture
- `docs/PROTOCOL.md` — REST API > Auth section, HTTP error contract

## Anticipated child features

Provisional — actual decomposition lands when this epic is designed.

- Multi-tenant data model + initial schema (orgs, accounts, sessions,
  members, oauth_tokens tables; sqlc setup with dual-dialect support
  for SQLite + Postgres)
- HTTP server skeleton (TLS, routing, middleware, structured logging,
  error contract enforcement)
- OAuth flow (initiate, callback, dynamic client registration if needed)
- Magic-link flow for headless self-host
- Token issuance, sliding-window refresh, revocation
- Account and org management endpoints (`/api/me`, admin org routes)

<!-- Design pass on each child feature will fill in specifics. -->
