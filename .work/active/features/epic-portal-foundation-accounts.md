---
id: epic-portal-foundation-accounts
kind: feature
stage: done
tags: [portal, security]
parent: epic-portal-foundation
depends_on: [epic-portal-foundation-auth-flows]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Foundation — Accounts & Orgs

## Brief

The account and org management surface that lives in the foundation epic
(the user-facing endpoints that have nothing to do with sessions, comments,
or git — those belong to `epic-portal-api`). Covers:

- `GET /api/me` — current account info: account id, display name, email,
  org memberships with roles. Used by the portal UI on every page load
  for the avatar + org switcher.
- `POST /api/orgs` — manual creation of an additional org. The
  authenticated account becomes the `creator` of the new org.
- `GET /api/orgs/<org_id>/members` — list members of an org (admin /
  creator role required).
- `POST /api/orgs/<org_id>/invites` — invite a member by email (admin /
  creator role required). Sends an org-invite email via the auth-flows
  `Sender` interface; the recipient signs in (auth-flows flow) and is
  then bound to the org by accepting the invite.
- `POST /api/orgs/<org_id>/invites/<invite_id>/accept` — accept a
  pending org invite (authenticated, recipient-matching).

**Role model**: two roles per org (`creator`, `member`). Creators can
invite, remove, and edit org metadata. Members can read.

Does NOT cover session lifecycle endpoints, session invites (`epic-portal-api`
owns `POST /api/sessions/<id>/invites`), or any per-session membership
work. Does NOT cover personal account deletion or email change — deferred
until the post-v1 settings surface.

## Epic context

- Parent epic: `epic-portal-foundation`
- Position in epic: the last feature in the foundation epic's chain;
  consumes auth-flows for token issuance / member binding and the email
  `Sender` interface for org invites. After this lands, the foundation
  epic is complete and `epic-portal-api` can begin.

## Foundation references

- `docs/PROTOCOL.md` — REST API > Orgs & accounts section, HTTP error
  contract
- `docs/SECURITY.md` — Authorization (role checks)
- `docs/SPEC.md` — Multi-tenant by design (every route is org-scoped)
- `docs/ARCHITECTURE.md` — Data layer (multi-tenancy)

## Inherited epic design decisions

- **Multi-org per user**: account ↔ org is many-to-many via `members`;
  no "current org" stored server-side — org id is in the URL path.
- **Org auto-provisioning** already happened in auth-flows; this feature
  adds the manual-org-creation path and the org-invite admin surface.
- **Role model**: minimal two-role set (`creator`, `member`). Richer
  permissions are deferred.

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Role enforcement**: a small middleware `RequireOrgRole(roles ...)`
  reading the org_id from the URL path and the account from the
  BearerMiddleware context, looking up the org_member row. Returns
  `auth.insufficient_permission` if the account has no membership
  or the role isn't in the allowed set.
- **`/api/me` shape**: returns `{id, email, display_name, orgs: [{id, name, slug, role}]}`. The `orgs` array is built via `ListOrgsForAccount` (already exists in Store).
- **Org slug generation on `POST /api/orgs`**: same algorithm as
  the auto-provisioning helper from `auth-flows` (email-prefix +
  random-suffix on collision). Reuse the existing helper if
  possible (it lives at `internal/portal/auth/provision.go` —
  refactor to expose `GenerateOrgSlug(name, store) string` if
  needed).
- **Org invite shape**: a single-use token, 7-day TTL, scoped to a
  specific email recipient. Schema addition: `org_invites` table
  via 00005 migration. Invite acceptance binds the account to the
  org with `role: member`.
- **Invite email**: sent via the configured Sender. Subject/body
  references the inviter's display name and org name. The link
  points at the SPA route `/orgs/<org_id>/invites/<token>/accept`
  which posts to the accept endpoint.
- **Story decomposition**: 2 stories.
  1. `me-and-org-create` — RequireOrgRole middleware, `/api/me`,
     `POST /api/orgs`, openapi schemas. depends_on: []
  2. `org-members-and-invites` — `org_invites` table + 00005
     migration, list members, invite, accept. depends_on:
     [me-and-org-create]

## Architectural choice

Handlers in `internal/portal/accounts/` and `internal/portal/orgs/`
(or combined into one `accounts` package — single domain area).
Role middleware in `internal/portal/auth/middleware.go` (extends
the existing `internal/portal/auth/` package).

## Implementation Units

### Unit 1: Role middleware

**File**: `internal/portal/auth/middleware.go`
**Story**: `epic-portal-foundation-accounts-me-and-org-create`

```go
package auth

// RequireOrgRole returns a chi middleware that verifies the
// authenticated account is a member of the org named in the URL path
// (via chi.URLParam(r, "orgID")) AND that their role is in the allowed
// set. Use AFTER tokens.BearerMiddleware so the account is in context.
func RequireOrgRole(s store.Store, roles ...string) func(http.Handler) http.Handler
```

### Unit 2: `/api/me` + `POST /api/orgs`

**Files**:
- `internal/portal/accounts/handlers.go` — `MeHandler`, `CreateOrgHandler`
- `docs/openapi.yaml` (edit) — `GET /api/me` + `POST /api/orgs` paths; `MeResponse`, `OrgRef`, `CreateOrgBody` schemas

`GET /api/me`: read account from Bearer ctx, look up org memberships, return `MeResponse`.
`POST /api/orgs`: accept `{name}`, generate slug via shared helper, create org + org_member with creator role, return the new `Org`.

### Unit 3: org_invites schema

**File**: `internal/db/migrations/{sqlite,postgres}/00005_org_invites.sql`
**Story**: `epic-portal-foundation-accounts-org-members-and-invites`

```sql
CREATE TABLE org_invites (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    inviter_account_id TEXT NOT NULL REFERENCES accounts(id),
    recipient_email TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    accepted_at DATETIME,
    accepted_by_account_id TEXT REFERENCES accounts(id)
);
CREATE INDEX org_invites_org_idx ON org_invites(org_id);
CREATE INDEX org_invites_email_idx ON org_invites(recipient_email);
```

Plus query files + Store extension.

### Unit 4: Members + invites handlers

**Files**:
- `internal/portal/accounts/orgs.go` — `ListMembersHandler`, `CreateInviteHandler`, `AcceptInviteHandler`
- `docs/openapi.yaml` (edit) — corresponding paths + schemas (`InviteBody`, `InviteRef`, `MemberRef`)

Endpoints (per parent feature body Brief):
- `GET /api/orgs/<org_id>/members` — RequireOrgRole(creator, member); returns array
- `POST /api/orgs/<org_id>/invites` — RequireOrgRole(creator); accepts `{email}`; generates token (32 bytes hex, SHA256 hashed), 7-day TTL; sends email via Sender; returns `InviteRef`
- `POST /api/orgs/<org_id>/invites/<invite_id>/accept` — Bearer auth; verifies invite by token (URL param contains token), email matches auth account's email, not expired/already-accepted; binds account as org member with role `member`; marks accepted

Wait — accept endpoint uses invite_id in URL but the token comes from the email link. Let me reconsider. The accept can take `{token}` in the body OR `<invite_id>` in URL + verification by recipient email match. The spec lists `<invite_id>` so use that.

Actually the brief says `POST /api/orgs/<org_id>/invites/<invite_id>/accept` (authenticated, recipient-matching). So `invite_id` in URL, recipient-email-match check vs the auth user's email. The email link's token role is to confirm the recipient has email access — that role can be implicit if the email link includes a token verified by the SPA before invoking accept. For simplicity: `invite_id` in URL, recipient-email match server-side, no separate token needed.

Hmm but that's slightly insecure — anyone who knows an invite ID and has the matching email could accept. The token from the email is the proof. OK use a separate token:

`POST /api/orgs/<org_id>/invites/<invite_id>/accept` with body `{token}`. Server verifies (`invite_id, token_hash`) match + recipient_email matches authed user.

## Implementation Order

1. `me-and-org-create` — role middleware + GET /api/me + POST /api/orgs
2. `org-members-and-invites` — org_invites schema + list members + create invite + accept invite

## go.mod additions

None new. Uses existing senders, openapi, store, tokens.

## Testing

- Role middleware: not-a-member → 403; wrong-role → 403; right-role → next
- /api/me happy path: returns account + orgs array
- POST /api/orgs: creates org + member; slug collision suffix appended
- Invite create: generates token, calls Sender; row inserted
- Invite accept: valid token → binds member; expired token → 401; wrong account email → 403; already-accepted → 409
- list members: only authorized members can call

## Risks

- **Slug collisions at high concurrency**: same email-prefix from two simultaneous POST /api/orgs calls. Mitigation: retry on unique-violation with suffix re-roll.
- **Invite email phishing**: standard for email-based invites. The link MUST be HTTPS in production; SELF_HOST.md already documents this.

## Implementation summary

Both child stories done. Five endpoints + RequireOrgRole middleware + org_invites schema landed.

### Verification
- `go build ./...` clean
- `go test ./...` green
- `make generate && git diff --exit-code` green

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none

**Notes**: Capability complete. The foundation epic's 5th and final feature lands here. epic-portal-foundation can now advance to done — all 5 children (data-layer, http-skeleton, tokens, auth-flows, accounts) are at done.
