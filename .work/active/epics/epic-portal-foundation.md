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

## Design decisions

- **HTTP routing framework**: `chi` — jamsesh's HTTP surface has multiple distinct
  auth mechanisms per route group (`/api/*` Bearer, `/git/*` HTTP Basic,
  `/mcp/*` Bearer with headersHelper, `/ws` upgrade). Chi's per-subroute middleware
  stacks make this clean; stdlib middleware composition gets verbose for the multi-auth
  shape. Compatible with `http.Handler` so we can drop into stdlib anywhere.
- **OAuth identity model**: both magic-link direct + delegated OAuth (GitHub/Google).
  Magic-link is always available (no password concept ever). Delegated OAuth is offered
  alongside as a convenience. Aligns with the epic-portal-ui auth-UX lock (both equally
  prominent on the sign-in card).
- **Org provisioning**: self-serve at signup. First sign-in creates a personal org by
  default; users can also create additional orgs from the portal UI. SaaS-friendly;
  self-host operators get the same flow (the "first user" is the operator).
- **Magic-link email delivery**: pluggable provider abstraction. Interface over
  delivery (Send method takes recipient + magic-link URL); concrete implementations
  for SMTP (self-host default), SendGrid, Postmark, Resend. Selected by config. More
  code up front, no rewrites later when hosted deployment needs a transactional
  provider.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->


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
