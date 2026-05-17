---
id: epic-portal-foundation-auth-flows
kind: feature
stage: drafting
tags: [portal, security]
parent: epic-portal-foundation
depends_on: [epic-portal-foundation-tokens]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Foundation — Auth Flows

## Brief

The two user-facing authentication flows that produce a session token at
the end: **delegated OAuth** (GitHub for v1, with a provider interface so
Google/OIDC can land later without rework) and **magic-link** (always
available, no password concept ever — the canonical headless self-host
flow and the standard alternative on the locked sign-in card).

Both flows terminate in a call to the tokens feature's issuance helper.
Both flows also trigger **org auto-provisioning on first sign-in**: a new
account triggers creation of a personal org and a `members` row binding
account-to-org with role `creator` (the operator semantics on a fresh
self-host).

**Endpoints delivered** (per `docs/PROTOCOL.md > REST API > Auth`):

- `POST /api/auth/oauth/start` — returns the provider authorization URL
  (with state nonce stored server-side)
- `POST /api/auth/oauth/callback` — exchanges authorization code for
  provider access, resolves provider identity → portal account (creating
  if first sign-in), issues portal token
- `POST /api/auth/magic-link/request` — accepts email, generates a
  `magic_link_tokens` row (single-use, 15-minute TTL), sends the link via
  the configured email provider
- `POST /api/auth/magic-link/exchange` — accepts the magic-link token,
  marks it `used_at`, resolves/creates account, issues portal token

**Email provider abstraction**: a `Sender` interface with `Send(ctx,
recipient, subject, body) error`, with concrete implementations for SMTP
(self-host default), SendGrid, Postmark, and Resend. Selection via config
(`email.provider: smtp|sendgrid|postmark|resend` + per-provider settings).
Adding a provider is a new file implementing `Sender` and a config switch
entry — no other code touched.

**OAuth provider abstraction**: a `Provider` interface that exposes
authorization URL builder, code-to-identity exchange, and an identity
struct (provider id + email + display name). GitHub implementation
ships. Adding a provider (Google, generic OIDC) is a new file
implementing `Provider` + a config switch.

Does NOT cover the visual sign-in surface (`.mockups/flows/onboarding/02-sign-in.html`
is the locked target; epic-portal-ui-foundation implements the
client-side UI that posts to these endpoints). Does NOT cover account
management endpoints (those belong to the accounts feature).

## Epic context

- Parent epic: `epic-portal-foundation`
- Position in epic: consumes the tokens feature for issuance; consumed
  by the accounts feature (logged-in user needed for management surface).

## Foundation references

- `docs/PROTOCOL.md` — REST API > Auth section
- `docs/SECURITY.md` — Authentication > User authentication
  (OAuth + magic-link flow descriptions), Token lifetime and renewal
- `docs/UX.md` — Flow: joining a session (where these flows fire)
- `.mockups/flows/onboarding/02-sign-in.html` — locked sign-in card
  (OAuth + magic-link equally prominent)

## Inherited epic design decisions

- **OAuth providers v1**: GitHub only, behind a `Provider` interface.
  Google/OIDC add cleanly later.
- **Magic-link anti-replay**: single-use, 15-minute TTL, stored in
  `magic_link_tokens` with `used_at` timestamp. Second click returns
  "this link was already used" via the standard error contract.
- **Org auto-provisioning**: first sign-in via either flow creates a
  personal org + `members` row with role `creator`. No special
  bootstrap dance for self-host operators.
- **Email provider abstraction**: pluggable `Sender` interface; SMTP
  default for self-host, SendGrid/Postmark/Resend available via config.

## Decomposition risks

- The email provider abstraction is the most novel piece. If SMTP and
  the 3 hosted providers' error / retry semantics diverge significantly,
  the design pass may need to spike a 4-provider table of edge cases
  before locking the interface.
- Magic-link delivery latency depends on the email provider's queueing
  behavior. The 15-minute TTL is generous but design-time we should
  document how long-tail delivery interacts with the TTL.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
