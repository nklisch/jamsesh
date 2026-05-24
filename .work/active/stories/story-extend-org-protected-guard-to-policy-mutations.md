---
id: story-extend-org-protected-guard-to-policy-mutations
kind: story
stage: implementing
tags: [portal, playground, defense-in-depth]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Extend org_protected guard to session_invite_policy mutations

## Context

Surfaced by review of `feature-epic-ephemeral-playground-reserved-org`.
The locked design decision says `org_protected` blocks delete + rename
only. Risk #4 of that feature's design called out the inconsistency:
the playground org is provisioned with `session_invite_policy='open'`,
which is load-bearing for anonymous joins. An operator with creator
role on the playground org could PATCH the policy back to
`members_only` via the existing `PatchOrg` handler, silently breaking
playground without tripping the protection flag.

Today the foot-gun is unreachable in practice because:

- The playground org has no creator account (provisioned by startup
  hook, not by a user), so the `member.Role != "creator"` check in
  `internal/portal/accounts/orgs.go` PatchOrg will reject any caller.
- No `DELETE /api/orgs/{id}` or rename surface exists yet, so the
  primary guards are also pure defense-in-depth.

The risk re-emerges the moment either of these changes:

- Adding an admin / observability member with creator role on the
  playground org (mentioned as a future use case in the design).
- Adding a privileged ops endpoint that PATCHes orgs bypassing the
  member-role check.

## What to do

Add an `OrgProtected` check to `PatchOrg` (and any future `DeleteOrg`
/ rename handler) at `internal/portal/accounts/orgs.go`. Reject with
`409 org.protected` when the target is protected. The check is one
extra `GetOrgByID` round-trip plus a guard clause — cheap and
durable.

Update the design decision in the parent feature (or supersede it in
a follow-up note) to record that the guard's scope was widened from
"delete + rename" to "delete + rename + policy mutations on the
session_invite_policy field".

## Acceptance

- [ ] `PatchOrg` returns `409 org.protected` when called against any
      org with `OrgProtected=true`, regardless of which fields the
      caller is trying to change.
- [ ] Regression test in `internal/portal/accounts/orgs_test.go`
      exercises the rejection path against the playground org.
- [ ] Comment in `store.go`'s `OrgProtected` doc-string is widened to
      include "policy mutations".
