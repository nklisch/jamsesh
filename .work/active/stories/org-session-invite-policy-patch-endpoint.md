---
id: org-session-invite-policy-patch-endpoint
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

# `PATCH /api/orgs/{orgID}` — flip org session-invite policy

New admin-gated endpoint that updates `orgs.session_invite_policy`. Consumed
by the OrgSettings.svelte frontend story. Admin role required (the existing
`creator` role on `org_members`).

## Files

- Modify: `docs/openapi.yaml` — add `PATCH /api/orgs/{orgID}` operation
- Regenerate: `internal/api/openapi/server.gen.go`, `frontend/src/lib/api/types.gen.ts`
- Modify or create: `PatchOrg` handler in `internal/portal/accounts/orgs.go`
  (or `accounts/handlers.go` — check where org endpoints live)
- Modify: `internal/portal/accounts/orgs_test.go` (or create)

## OpenAPI

```yaml
/api/orgs/{orgID}:
  patch:
    operationId: PatchOrg
    summary: Update org-level settings. Admin role (creator) required.
    parameters:
      - { name: orgID, in: path, required: true, schema: { type: string } }
    requestBody:
      required: true
      content:
        application/json:
          schema:
            type: object
            additionalProperties: false
            properties:
              session_invite_policy:
                type: string
                enum: [members_only, open]
                description: |
                  Who may accept session invites in this org.
                  `members_only` (default) requires the invitee to already
                  be an `org_member`. `open` allows email-invitees to join
                  as session-scoped guests without org membership.
    responses:
      '200':
        description: Org updated
        content:
          application/json:
            schema: { $ref: '#/components/schemas/Org' }
      '400': { $ref: '#/components/responses/BadRequest' }
      '401': { $ref: '#/components/responses/Unauthorized' }
      '403': { $ref: '#/components/responses/Forbidden' }
      '404': { $ref: '#/components/responses/NotFound' }
```

Surface `session_invite_policy` on the `Org` response schema if it isn't
already (the schema regenerates anyway).

## Handler

```go
// internal/portal/accounts/orgs.go (or wherever PatchOrg lands)
func (h *Handler) PatchOrg(ctx context.Context, req openapi.PatchOrgRequestObject) (openapi.PatchOrgResponseObject, error) {
    _, member, fail, ok := handlerauth.RequireOrgMember(ctx, h.store, req.OrgID)
    if !ok {
        if fail.Err != nil { return nil, fmt.Errorf("orgs: patch: %w", fail.Err) }
        return patchOrgFail(fail), nil
    }
    if member.Role != "creator" {
        return openapi.PatchOrg403JSONResponse{
            ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
                Error:   "auth.insufficient_permission",
                Message: "only the org creator can modify org settings",
            },
        }, nil
    }

    if req.Body.SessionInvitePolicy != nil {
        val := string(*req.Body.SessionInvitePolicy)
        if val != "members_only" && val != "open" {
            // Defensive: OpenAPI enum should catch this, but belt + suspenders.
            return openapi.PatchOrg400JSONResponse{...}, nil
        }
        if err := h.store.UpdateOrgSessionInvitePolicy(ctx, store.UpdateOrgSessionInvitePolicyParams{
            ID:                  req.OrgID,
            SessionInvitePolicy: val,
        }); err != nil {
            return nil, fmt.Errorf("orgs: update policy: %w", err)
        }
    }

    org, err := h.store.GetOrg(ctx, req.OrgID)
    if err != nil { return nil, fmt.Errorf("orgs: get: %w", err) }
    return openapi.PatchOrg200JSONResponse(orgToOpenAPI(org)), nil
}

func patchOrgFail(f handlerauth.AuthFail) openapi.PatchOrgResponseObject {
    if f.Status == 401 {
        return openapi.PatchOrg401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
    }
    return openapi.PatchOrg403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}
```

The `orgToOpenAPI` helper may not exist yet — if not, write one alongside
the handler, mapping `store.Org` → `openapi.Org`. Include
`SessionInvitePolicy` in the mapping.

## Grandfather note

Per the feature's strategic decisions, flipping policy from `open` to
`members_only` does NOT eject existing session-only members. The schema
update is single-row; no cascade. This is intentional behavior, not an
oversight — capture in the handler-doc comment.

## Acceptance criteria

- [ ] OpenAPI spec validates and regenerates Go + TS types
- [ ] `PatchOrg` requires bearer auth (401 if missing)
- [ ] `PatchOrg` requires org membership (403 if not a member)
- [ ] `PatchOrg` requires `creator` role (403 if regular member)
- [ ] Setting `session_invite_policy` to `members_only` or `open` persists
- [ ] Setting `session_invite_policy` to an unknown value returns 400
- [ ] No retroactive cascade: existing `session_members` rows unchanged
      after a policy flip
- [ ] `go build ./...` clean
- [ ] `go test ./internal/portal/accounts/...` passes

## Risk

LOW. Standard CRUD endpoint following the established handlerauth pattern.

## Rollback

`git revert` the commit. The endpoint disappears; the schema column stays
(idempotent — orgs default to `members_only` via the migration).
