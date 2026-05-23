---
id: feature-epic-ephemeral-playground-anon-bearer
kind: feature
stage: drafting
tags: [portal, security]
parent: epic-ephemeral-playground
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Anonymous session-scoped bearer tokens

## Brief

Extends the existing `oauth_tokens` table to carry anonymous
session-scoped bearers — the auth substrate that makes ephemeral
playground identities work without touching the OAuth flow. Adds an
`anonymous_session_bearer` value to the existing `kind` column and a
nullable `session_id` foreign key. Anonymous identities also get an
`accounts` row marked `is_anonymous: true` so the existing
`session_members.account_id` FK and `RequireSessionMember` middleware
work unchanged. The bearer plugs into the bearer middleware, MCP's
`verifyToken`, and the git Basic-auth resolver without per-call-site
branching — handlers don't differentiate identity kind, only membership.

The token service grows one new method:
`IssueAnonymousSessionBearer(ctx, sessionID, nickname) (string, error)`.
Validation reuses `tokens.Validate` unchanged. Revocation happens
implicitly during session destruction (the destruction sweep sets
`oauth_tokens.revoked_at` for every bearer with the session_id before
deleting the session row).

This feature is auth-substrate only — it does NOT create playground
sessions or issue bearers in response to API calls. That's
`session-lifecycle`. This feature ships the primitive; the lifecycle
feature wires it up.

## Epic context
- Parent epic: `epic-ephemeral-playground`
- Position in epic: **wave 1 foundation** — no dependencies; required by
  `session-lifecycle` (wave 2) for the bearer-issuance step in
  playground session creation and joiner accept.

## Foundation references
- `docs/SPEC.md` § Auth model — the anonymous-bearer bullet added at
  scope time describes the contract this feature implements
- `docs/ARCHITECTURE.md` § Data layer — `oauth_tokens` and `accounts`
  table shapes; this feature is the first migration touching them since
  the epic's scope work
- `docs/SECURITY.md` — anon-bearer threat model addendum (token leak
  scope is session-bounded, no cross-session blast radius) is owned by
  this feature's design pass

## Mockups
No UI surface — substrate-only feature. The parent epic's flow mocks
cover everything user-visible.
