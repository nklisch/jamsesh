---
id: epic-portal-foundation-tokens
kind: feature
stage: drafting
tags: [portal, security]
parent: epic-portal-foundation
depends_on: [epic-portal-foundation-data-layer, epic-portal-foundation-http-skeleton]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Foundation — Tokens

## Brief

The token subsystem the entire system authenticates against. Implements
issuance, validation, sliding-window refresh, and revocation for the user
OAuth tokens that serve as Bearer auth (REST + MCP), HTTP Basic password
(git smart-HTTP), and any future client surface. One token model, one
codepath, multiple transports consume it.

**Token shape**: opaque random tokens (32 bytes, hex-encoded), stored
hashed at rest in the `oauth_tokens` table (epic-design decision — no JWTs).
Validation is a hashed-lookup against the row; revocation is row deletion
or a `revoked_at` timestamp set; expiry is `expires_at` enforced at
validation time. The `oauth_tokens` row also carries `account_id` and
metadata (issued-by flow, last-used timestamp).

**Lifetimes (from SECURITY.md):**
- Access tokens: 1 hour TTL
- Refresh tokens: 30 days TTL, renewed on each refresh (sliding window)
- Revocation propagates within 1 minute (every protected request validates
  against the DB; cache, if introduced, has ≤ 60s TTL)

**Surface delivered:**
- Bearer-auth middleware for `/api/*` that resolves `Authorization:
  Bearer <token>` to an `account_id` (and rejects with the standard error
  contract on failure)
- A token-as-Basic-Auth-password helper that `epic-portal-git`'s
  smart-HTTP handler uses to validate `git push` credentials
- Issuance helper called by auth-flows after successful OAuth or
  magic-link exchange
- Refresh endpoint (`POST /api/auth/refresh`) and revoke endpoint
  (`POST /api/auth/revoke`) per `docs/PROTOCOL.md > REST API > Auth`
- Background sweep that prunes expired tokens (optional but documented;
  the design pass decides whether to land it here or defer)

Does NOT cover OAuth client flow or magic-link delivery (auth-flows owns
those). Does NOT cover account/org provisioning — that happens in
auth-flows on first sign-in.

## Epic context

- Parent epic: `epic-portal-foundation`
- Position in epic: depends on both data-layer (oauth_tokens table) and
  http-skeleton (middleware mount points); auth-flows depends on this for
  issuance.

## Foundation references

- `docs/SECURITY.md` — Authentication > Token lifetime and renewal,
  Authorization > MCP and REST API authorization, What a single-user-token
  compromise exposes
- `docs/PROTOCOL.md` — REST API > Auth section, HTTP error contract
- `docs/SPEC.md` — Auth model (one token per user, scope of uses)

## Inherited epic design decisions

- **Token format**: opaque random bytes, hex-encoded, hashed at rest.
- **Revocation mechanism**: row state on `oauth_tokens`; every protected
  request validates against the DB (or a ≤60s TTL cache). No JWT
  blocklist machinery.
- **Token reuse across transports**: same token serves Bearer (REST/MCP)
  and HTTP Basic password (git push). One token, three transports.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
