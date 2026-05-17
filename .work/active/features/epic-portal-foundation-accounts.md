---
id: epic-portal-foundation-accounts
kind: feature
stage: drafting
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

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
