---
id: epic-portal-foundation
kind: epic
stage: done
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

Locked at epicize time:

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

Locked at epic-design time (this pass):

- **Token storage format**: opaque random tokens (32 bytes, hex-encoded), stored
  hashed at rest in the `oauth_tokens` table. No JWTs. Validation is a hashed
  lookup; revocation is row deletion or `revoked_at`. Rationale: the portal is a
  single process — stateless JWT verification has no benefit here. Opaque tokens
  meet the 1-minute revocation propagation SLO from SECURITY.md trivially and
  avoid JWT/blocklist machinery.
- **OAuth providers in v1**: GitHub only, behind a `Provider` interface that
  exposes auth-URL builder, code-to-identity exchange, and an identity struct.
  Google/OIDC add as new files implementing the interface + a config switch.
  Rationale: matches the locked sign-in mock (only "Continue with GitHub"),
  avoids overbuilding, preserves pluggability.
- **Multi-org "current org" resolution**: derived from URL path
  (`/api/orgs/<org_id>/...`). No server-side "current org" state. Routes that
  aren't org-scoped (e.g., `/api/me`) carry no org context. Rationale:
  SPEC.md mandates every API route is org-scoped; URL-encoded org id is the
  source of truth, no drift.
- **Magic-link anti-replay**: separate `magic_link_tokens` table, single-use
  via `used_at` timestamp, 15-minute TTL. Second click returns the standard
  "already used" error. Rationale: matches the opaque-token discipline of
  the rest of auth; 15 min balances email-delivery latency vs. compromise
  window.
- **Self-host first-user bootstrap**: no special state. First sign-in via any
  flow creates an org + `members` row with role `creator`. Rationale: matches
  the self-serve-at-signup decision and the "first user is the operator"
  framing from the brief; one codepath for SaaS and self-host.
- **TLS termination**: support both native HTTPS (cert path config) AND
  HTTP-behind-trusted-proxy mode (config-selected). Rationale: SPEC.md and
  SECURITY.md leave it open; operators have varied infrastructure; cost is
  one config flag.

## Decomposition

Five child features. The split goes by capability arc, not by layer:
**data-layer** and **http-skeleton** are independent infrastructure pieces
that parallelize from day one. **tokens** is the auth substrate; both
infrastructure pieces feed into it. **auth-flows** consumes tokens to produce
issued sessions via OAuth + magic-link. **accounts** sits on top with the
management surface (`/api/me`, manual org creation, admin endpoints).

Critical path is 4 deep:
`{data-layer || http-skeleton} → tokens → auth-flows → accounts`. The
opening pair runs in parallel; the rest is sequential because each step
materially consumes the previous one's contract.

### Child features

- `epic-portal-foundation-data-layer` — sqlc setup, dual-dialect SQLite +
  Postgres, initial schema for orgs/accounts/sessions/members/oauth_tokens,
  org_id WHERE-clause discipline — depends on: `[]`
- `epic-portal-foundation-http-skeleton` — chi router, per-subroute
  middleware shape, structured logging, JSON error contract, config loading
  (env + YAML), native HTTPS + behind-proxy TLS modes — depends on: `[]`
- `epic-portal-foundation-tokens` — opaque token issuance, sliding-window
  refresh, revocation, Bearer middleware, token-as-Basic-Auth-password
  helper for git push — depends on:
  `[epic-portal-foundation-data-layer, epic-portal-foundation-http-skeleton]`
- `epic-portal-foundation-auth-flows` — GitHub OAuth (start + callback)
  behind a `Provider` interface, magic-link (request + exchange) with a
  pluggable `Sender` interface (SMTP/SendGrid/Postmark/Resend), org
  auto-provisioning on first sign-in — depends on:
  `[epic-portal-foundation-tokens]`
- `epic-portal-foundation-accounts` — `/api/me`, manual org creation,
  org-member admin endpoints, org-invite flow — depends on:
  `[epic-portal-foundation-auth-flows]`

### Decomposition risks

- **Data layer is the linchpin.** The sqlc dual-dialect pattern (per-dialect
  query packages, dialect selection at build or runtime) and the
  org_id-in-WHERE convention must be locked at data-layer's design pass —
  every other feature in this epic and every sibling persistence feature
  pays if the pattern is wrong.
- **Token revocation propagation budget.** SECURITY.md says revocation
  propagates within 1 minute. Opaque tokens validated against the DB on
  every protected request meet this trivially, but if the design pass
  introduces a cache, its TTL must be ≤ 60s.
- **Email provider abstraction surface.** auth-flows owns four concrete
  email implementations (SMTP + 3 hosted). Their error / retry semantics
  may diverge enough that the `Sender` interface needs more than a single
  `Send(ctx, recipient, subject, body) error` method (e.g., async vs sync
  send, idempotency keys). Design pass spikes the four-provider error
  table before locking the interface.
- **OAuth provider interface forward-compat.** v1 ships GitHub only, but
  Google/OIDC are explicit forward paths. The `Provider` interface must
  accommodate OIDC's discovery-document model without GitHub baking in
  assumptions.

## Final review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none

**Notes**: All 5 child features at done: data-layer (sqlc dual-dialect + org_id discipline), http-skeleton (chi + httperr + openapi pipeline + config + TLS), tokens (Service + middleware + refresh/revoke endpoints), auth-flows (OAuth + magic-link + Sender abstraction + auto-provisioning), accounts (/api/me + org create + invites). The portal foundation is complete. The portal-api epic and the cc-plugin/finalize-flow chains can now build on this substrate end-to-end.
