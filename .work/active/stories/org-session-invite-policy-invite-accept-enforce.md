---
id: org-session-invite-policy-invite-accept-enforce
kind: story
stage: implementing
tags: [portal, security]
parent: org-session-invite-policy
depends_on: [org-session-invite-policy-schema]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Invite-accept — enforce per-org policy in `AcceptSessionInvite`

Add the policy gate to `internal/portal/sessions/invites.go:AcceptSessionInvite`.
Under `members_only`, reject non-org-members with 403
`auth.org_membership_required`. Under `open`, preserve current behavior.

## Files

- Modify: `internal/portal/sessions/invites.go` — `AcceptSessionInvite`
  (~lines 135-240)
- Modify: `docs/openapi.yaml` — declare the new 403 error code on the
  `AcceptSessionInvite` operation (no schema change, just a documented
  code under the existing 403 response).
- Regenerate: `internal/api/openapi/server.gen.go` if the OpenAPI change
  surfaces a generated constant.
- Modify or create: `internal/portal/sessions/invites_test.go` covering
  both policy paths × both account states.

## Target shape

After all existing invite-validity checks (token, expiry, accepted-flag,
email-match) and BEFORE the `WithTx` block that inserts `session_members`:

```go
// Per-org policy: members_only requires the account to be an existing org_member.
org, err := h.store.GetOrg(ctx, orgID)
if err != nil {
    return nil, fmt.Errorf("sessions: accept invite: get org: %w", err)
}

if org.SessionInvitePolicy == "members_only" {
    _, memErr := h.store.GetOrgMember(ctx, store.GetOrgMemberParams{
        OrgID:     orgID,
        AccountID: acc.ID,
    })
    if memErr != nil {
        if errors.Is(memErr, store.ErrNotFound) {
            return openapi.AcceptSessionInvite403JSONResponse{
                ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
                    Error:   "auth.org_membership_required",
                    Message: "this org requires you to be a member before joining sessions; ask an org admin to add you first",
                },
            }, nil
        }
        return nil, fmt.Errorf("sessions: accept invite: get org member: %w", memErr)
    }
}

// Existing WithTx that adds session_members continues unchanged.
```

The check goes AFTER email-match verification (line ~193 of the current
code) — that way the precondition messaging stays user-friendly (don't tell
a wrong-email request about org policy until we've verified the actor at
least owns the invite).

## OpenAPI

Add `auth.org_membership_required` to the documented error codes for the
`AcceptSessionInvite` operation in `docs/openapi.yaml`. The 403 response
shape itself stays `ForbiddenJSONResponse`; no new schema needed.

## Acceptance criteria

- [ ] `AcceptSessionInvite` rejects with 403 + `auth.org_membership_required`
      when org policy is `members_only` AND accepting account is not in
      `org_members`
- [ ] `AcceptSessionInvite` succeeds when org policy is `members_only` AND
      accepting account IS in `org_members` (existing happy path)
- [ ] `AcceptSessionInvite` succeeds when org policy is `open` regardless
      of org membership (preserves the refactor's prior behavior)
- [ ] Test fixture sets org policy directly via SQL or via
      `Store.UpdateOrgSessionInvitePolicy` to exercise both paths in one
      test run
- [ ] `go build ./...` clean
- [ ] `go test ./internal/portal/sessions/...` passes

## Risk

MEDIUM. Security-sensitive: a wrong check inversion would let non-members
in when the org policy forbids it. Mitigations:
- The pattern mirrors `handlerauth.RequireOrgMember` (already reviewed)
- Test fixtures pin both policy paths
- The check is purely additive — it doesn't change the open-policy path

## Rollback

`git revert` the commit. Behavior reverts to "session-membership only
required" (the post-refactor pre-feature behavior).
