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

## Design decisions

Locked at `--only-questions` time. Feature-design Phase 5 inherits these
as fixed input.

- **Bearer expiry mechanism**: belt-and-suspenders. Bearer carries
  `expires_at` synced to the session's hard-cap deadline (24h from
  creation by default; in lockstep with the session's
  `JAMSESH_PLAYGROUND_HARD_CAP` setting). `tokens.Validate` rejects
  expired bearers automatically via existing logic. The destruction
  sweep ALSO sets `revoked_at` on every bearer for the destroyed
  session as a second layer. If the sweep is delayed, TTL still kicks
  in; if TTL math drifts, sweep cleans up. Either failure mode is
  benign — both have to fail simultaneously for bearers to outlive
  their session.

- **Anonymous `accounts` row shape**: one row per anonymous session
  participant. Schema: `accounts.id` = `anon_<random>`, `is_anonymous:
  true` (new boolean column), `email: NULL`, `display_name` =
  server-minted nickname. 1:1 with `session_members.account_id` —
  every account-joined query, commit-attribution path, and addressed-
  comment lookup works unchanged. Storage cost is bounded by max
  concurrent participants × active session count (negligible).

- **Nickname storage**: `accounts.display_name`. Single source of
  truth; reuses the existing column populated for durable accounts.
  Joiner-rename at the nickname-picker surface is one UPDATE. Both
  the presence panel and `@<nickname>` addressing lookups read from
  the same column for both durable and anonymous identities — no
  identity-kind branching needed.

- **Anonymous account cleanup on session destruction**: cascade-delete
  with the session. The destruction routine sequence becomes:
  1. Revoke all bearers for the session (set `oauth_tokens.revoked_at`)
  2. Delete `comments` and `conflict_events` for the session
  3. Delete the `sessions` row (FK CASCADE handles `session_members`,
     `events`, `presence`)
  4. Delete the anonymous `accounts` rows that joined this session
     (identified by `is_anonymous: true` and a JOIN against the
     just-deleted `session_members` rows captured pre-delete)
  5. Delete the bare repo on disk under `<storage>/orgs/playground/sessions/<id>.git`
  
  Step 4's identification depends on capturing the to-delete account
  IDs before step 3 (since the FK cascade removes the
  `session_members` rows by then). Implementation note: the
  destruction routine collects `account_id` list at the top of the
  transaction, applies cascades, then deletes the captured account
  IDs. Anonymous accounts never participate in another session
  (per-session-row decision above), so the cleanup is safe and
  consistent with the strict-ephemeral commitment.
