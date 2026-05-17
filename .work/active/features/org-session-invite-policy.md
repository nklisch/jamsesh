---
id: org-session-invite-policy
kind: feature
stage: drafting
tags: [portal, ui, security]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Feature — Per-org session invite policy

## Brief

Some orgs want strict members-only access; others want open email-invite
collaboration. Encode that as a per-org configuration — `orgs.session_invite_policy` with
two values, `members_only` (default, strictest) and `open` — and enforce it at the
invite-accept perimeter. Under `members_only`, accepting a session invite requires
the invitee to already be an `org_member`. Under `open`, accepting adds the invitee
to `session_members` only — they become a session-scoped guest, not an org member.

After the gate fires at invite-accept time, the rest of the system stays simple:
`handlerauth.RequireSessionMember` is correct for session-scoped operations (membership
already enforced per the org's policy), and `handlerauth.RequireOrgMember` keeps
open-org session guests out of org-scoped operations.

This is the resolution to the audit previously parked as
`audit-session-membership-org-implication.md` (the audit asked which of three
resolution paths to take; the answer is "all of them per-org via a policy column,
not a global choice").

UI scope: a new dedicated org settings screen at `/orgs/:orgID/settings` for admins
to flip the policy, and a new invite-accept screen with happy-path + members-only
rejection states. Mockups are required for both surfaces before frontend code per the
project's mockup-first rule.

## Strategic decisions

These were locked in via interactive Q&A before scoping. Captured here so
`feature-design` and downstream stories inherit them:

- **Default policy for new orgs + migration backfill**: `members_only`. Strictest
  default, preserves pre-refactor strictness for everyone in the system today.
  Org admins can flip to `open` per-org via the settings UI.
- **Open-org guest model**: session-only. Open-org invitees become `session_members`
  only — never auto-promoted to `org_members`. This keeps org member lists limited
  to actual org employees and avoids inventing a new `guest` role on `org_members`.
  `RequireOrgMember` correctly keeps them out of org-scoped operations.
- **Invite-accept UI scope**: build the full accept screen fresh — no
  `InviteAccept.svelte` exists in the SPA today. The screen handles both
  happy-path (show invite details + Accept button → redirect to session) and the
  members-only rejection state ("ask an admin to invite you to the org first").
- **Org settings navigation**: new dedicated route `/orgs/:orgID/settings` with the
  session-invite-policy section as its first (currently only) section. Future
  sections — members management, billing, etc. — extend the same screen.

## UI surface flag

`ux-ui-design` plugin is installed; this feature has net-new UI surface
(both `OrgSettings.svelte` and `InviteAccept.svelte` are greenfield). Tagged
`[ui]` so `feature-design`'s Phase 4.6 picks up the mockup-first
requirement. Mockups for both surfaces go in
`.mockups/screens/org-session-invite-policy/`. The "guest" visual indicator
for session-only members in existing session views is intentionally a polish
nit, not part of this feature — it can ride along when a UI story
naturally touches member-chip rendering.

## Implementation direction (for feature-design)

When `feature-design` picks this up, the natural decomposition is roughly six
child stories along two parallel tracks:

- **Backend**: schema + sqlc regeneration; invite-accept policy enforcement;
  `PATCH /api/orgs/{orgID}` endpoint with admin-role gate
- **UI**: mockups (blocks both UI implementation stories); `OrgSettings.svelte`
  screen; `InviteAccept.svelte` screen with happy + rejection states

The mockup story is the blocker for both frontend stories; backend can stream
in parallel. The feature can land all on one release tag — no inherent
phasing constraint.

## Affected code (for feature-design grounding)

Pre-existing handlers and flows the implementation will touch:

- `internal/portal/sessions/invites.go:AcceptSessionInvite` (~lines 130-230) —
  add policy check before the tx that inserts `session_members`
- `internal/portal/accounts/orgs.go` — add `PatchOrg` handler with `session_invite_policy` field
- `internal/portal/handlerauth/handlerauth.go` — stays as-is; the perimeter gate
  pattern means `RequireSessionMember` doesn't need policy awareness
- Frontend: no existing org-settings or invite-accept screens; both are greenfield
- Schema: `orgs.session_invite_policy TEXT NOT NULL DEFAULT 'members_only'` with
  CHECK constraint on the two enum values; sqlc regeneration needed

## Foundation-doc note

`docs/ARCHITECTURE.md` currently asserts "Every persisted entity carries `org_id`.
Every API route is org-scoped" without specifying the membership model in detail.
This feature should add a short subsection (in ARCHITECTURE.md or a new section in
`docs/SECURITY.md`) describing:

- That orgs carry a `session_invite_policy` (the two values + their effect)
- That `session_members` and `org_members` are independent tables joined at
  invite-accept time by policy
- That open-org session guests have access to the session they were invited to
  but NOT to org-wide resources (i.e. `RequireOrgMember` is the gate that keeps
  them out)

Whether this is medium-scope-with-inline-doc-touch or warrants a Phase 4
foundation roll-forward at scope time is a judgment call; sizing this as medium
here, with the doc touch listed as feature-level acceptance (handled inline
during implementation, not pre-locked at scope time).

## Acceptance criteria

- [ ] Existing orgs migrated to `members_only` policy via the schema default
- [ ] `AcceptSessionInvite` rejects non-org-members when org is `members_only`
- [ ] `AcceptSessionInvite` succeeds for non-org-members when org is `open`
      (preserves the current refactor's behavior)
- [ ] `PATCH /api/orgs/{orgID}` accepts `session_invite_policy`; admin-role gated
- [ ] `OrgSettings.svelte` screen renders current policy + lets admins save
- [ ] `InviteAccept.svelte` renders happy-path and members-only rejection states
- [ ] All existing tests pass; new tests cover both policy paths
- [ ] `docs/ARCHITECTURE.md` (or `docs/SECURITY.md`) updated with a short
      membership-model section
- [ ] Mockups committed under `.mockups/screens/org-session-invite-policy/`

## History

This feature was sourced from `.work/backlog/audit-session-membership-org-implication.md`
(filed as an Important finding during the `refactor-handler-auth-guards-comments`
review). The audit's three resolution paths (restore org check / fix invite flow /
explicit guest model) collapsed once we noticed the right framing is per-org
configuration: every org gets to pick. The audit body has been replaced by this
feature; git history preserves the audit's original analysis.
