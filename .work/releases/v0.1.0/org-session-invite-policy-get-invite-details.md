---
id: org-session-invite-policy-get-invite-details
kind: story
stage: done
tags: [portal]
parent: org-session-invite-policy
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# GET invite details — new endpoint for the InviteAccept screen

The InviteAccept onboarding-hero mockup renders inviter name, org name,
session name, and session goal before the user clicks Accept. The existing
`POST /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept`
performs the accept; it doesn't return the details required to render the
pre-accept state. Add a sibling GET endpoint that returns those fields.

## Files

- Modify: `docs/openapi.yaml` — add the new operation
- Regenerate: `internal/api/openapi/server.gen.go` and
  `frontend/src/lib/api/types.gen.ts`
- Modify or create: handler implementation in
  `internal/portal/sessions/invites.go` (`GetSessionInvite` method)
- Modify or create: `internal/portal/sessions/invites_test.go`

## OpenAPI

```yaml
/api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}:
  get:
    operationId: GetSessionInvite
    summary: Fetch invite details for rendering the InviteAccept screen.
    parameters:
      - { name: orgID,     in: path, required: true, schema: { type: string } }
      - { name: sessionID, in: path, required: true, schema: { type: string } }
      - { name: inviteID,  in: path, required: true, schema: { type: string } }
      - name: token
        in: query
        required: true
        description: The invite token from the email link.
        schema: { type: string }
    responses:
      '200':
        description: Invite details
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/SessionInviteDetails'
      '401': { $ref: '#/components/responses/Unauthorized' }
      '403': { $ref: '#/components/responses/Forbidden' }
      '404': { $ref: '#/components/responses/NotFound' }
      '409': { $ref: '#/components/responses/Conflict' }
```

Plus a new schema:

```yaml
components:
  schemas:
    SessionInviteDetails:
      type: object
      required: [invite_id, org_name, session_id, session_name, invited_by_name, expires_at, your_role_on_accept]
      properties:
        invite_id:           { type: string }
        org_name:            { type: string }
        session_id:          { type: string }
        session_name:        { type: string }
        session_goal:        { type: string, nullable: true }
        invited_by_name:     { type: string, description: "Display name of the inviter (creator/admin who issued the invite)" }
        expires_at:          { type: string, format: date-time }
        your_role_on_accept: { type: string, enum: [member] }
```

Verify field names and casing against `docs/openapi.yaml` conventions
before locking in (the existing spec mixes camelCase and snake_case
deliberately — match the closest pattern).

## Handler

```go
// internal/portal/sessions/invites.go
func (h *Handler) GetSessionInvite(ctx context.Context, req openapi.GetSessionInviteRequestObject) (openapi.GetSessionInviteResponseObject, error) {
    // 1. Bearer auth (RequireAccount only — no session-member gate yet;
    //    the invite token + email match IS the auth here, same shape as
    //    POST accept).
    acc, fail, ok := handlerauth.RequireAccount(ctx)
    if !ok { return openapi.GetSessionInvite401JSONResponse{UnauthorizedJSONResponse: fail.Unauthorized}, nil }

    // 2. Fetch invite by id.
    invite, err := h.store.GetSessionInviteByID(ctx, req.InviteID)
    if err != nil {
        if errors.Is(err, store.ErrNotFound) {
            return openapi.GetSessionInvite401JSONResponse{...}, nil  // 401 to avoid invite-id enumeration
        }
        return nil, fmt.Errorf("sessions: get invite: %w", err)
    }

    // 3. Verify token hash + email match + not-expired + not-accepted, exactly
    //    as AcceptSessionInvite does. Reuse hashSessionInviteToken helper.
    // (See AcceptSessionInvite lines 162-200 for the canonical sequence.)

    // 4. Fetch org name, session details, inviter name.
    org, err := h.store.GetOrg(ctx, req.OrgID)
    if err != nil { return nil, ... }
    session, err := h.store.GetSession(ctx, req.OrgID, req.SessionID)
    if err != nil { return nil, ... }
    inviter, err := h.store.GetAccount(ctx, invite.CreatedByAccountID)
    if err != nil { return nil, ... }

    return openapi.GetSessionInvite200JSONResponse{
        InviteID:         invite.ID,
        OrgName:          org.Name,
        SessionID:        session.ID,
        SessionName:      session.Name,
        SessionGoal:      &session.Goal,
        InvitedByName:    inviter.DisplayName,
        ExpiresAt:        invite.ExpiresAt,
        YourRoleOnAccept: "member",
    }, nil
}
```

The `invite.CreatedByAccountID` field name is a guess — check the actual
`SessionInvite` row shape and use whichever field tracks the inviter.

## Acceptance criteria

- [ ] OpenAPI spec validates and regenerates Go + TS types
- [ ] `GetSessionInvite` returns 200 + details for a valid token-matching
      request from the matching email
- [ ] `GetSessionInvite` returns 401 for invalid token, missing token, or
      account email mismatch (the four "401-emit" paths in AcceptSessionInvite)
- [ ] `GetSessionInvite` returns 409 if the invite has already been accepted
- [ ] No state mutation — re-running the GET returns the same body
- [ ] `go build ./...` clean
- [ ] `go test ./internal/portal/sessions/...` passes

## Risk

LOW-MEDIUM. New endpoint surface; needs careful auth (mirrors the POST-accept
auth exactly, just without the mutating tx). The risk is that we leak invite
metadata to an account that shouldn't see it — the email-match check is the
backstop against that.

## Rollback

`git revert` the commit. The endpoint disappears; the frontend story falls
back to a less rich invite-accept UX (which is fine for v1.x without this
endpoint).

## Implementation notes

- Handler: `internal/portal/sessions/invites.go` — `GetSessionInvite` method added at line ~134.
- Auth shape: mirrors `AcceptSessionInvite` exactly for bearer + token-hash + expiry + already-accepted checks. Email mismatch returns 401 (not 403) on GET to avoid leaking invite existence.
- Store lookups used: `GetSessionInviteByID`, `GetOrgByID`, `GetSession`, `GetAccountByID`.
- Token comes from query param `req.Params.Token` (not request body).
- Route registered in bearer-auth group in `cmd/portal/main.go` and `handler_test.go`.
- `combinedHandler.GetSessionInvite` delegation added to `cmd/portal/main.go`.
- OpenAPI path `/api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}` (GET) added to `docs/openapi.yaml`.
- `SessionInviteDetails` schema added with snake_case fields per project convention.
- Go types regenerated via `make generate-api-go`, TS types via `make generate-api-ts`.
- Build-blocking side-effects from sibling story `lease-fencing-schema-verify-sqlc-regen` were fixed: `postgres_adapter.go` `pgFinalizeLock` and `postgresTxStore` methods updated to use `time.Time` (matching regenerated pgstore), `sqlite_adapter.go` finalize lock adapter similarly updated.
- 8 test cases added covering: HappyPath, InvalidToken, MissingToken (400 from oapi-codegen), UnknownInvite (401 not 404), WrongEmail (401 not 403), Expired, AlreadyAccepted, NoMutation.
- All 8 new tests pass; full sessions suite (previous tests) remains green.
- `go build ./...` clean; `npm run check` only has pre-existing `RefGroupList.test.ts` errors unrelated to this story.

## Note on shared validation

The token/email/expiry validation logic is duplicated between
`AcceptSessionInvite` and this new `GetSessionInvite`. If the implementing
agent finds the duplication painful, extract a small private helper —
`validateSessionInvite(ctx, store, orgID, sessionID, inviteID, token, acc) (SessionInvite, AcceptInviteResponseEnvelope, ok bool)`
— and have both endpoints call it. This is a refactor-during-implementation
opportunity, not a blocker.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- Token/email/expiry validation is duplicated verbatim between
  `AcceptSessionInvite` and `GetSessionInvite`. The story explicitly allowed
  this and called the refactor optional; agent left both inline. Reasonable
  given the security-sensitive nature — extracting later as a separate
  refactor pass is fine.
- Check order is `already_accepted` before `email_match` (mirrors the existing
  POST handler). This means a 409 leaks "invite exists and was accepted"
  even to a non-invitee — but the adversary would also need the valid token
  hash to reach that point, so practically not exploitable.

**Notes**: Auth shape mirrors `AcceptSessionInvite` exactly with the
deliberate 401-instead-of-403/404 substitutions on GET to prevent invite-id
and email enumeration. Eight tests cover happy, invalid-token, missing-token,
unknown-invite, wrong-email, expired, already-accepted, and no-mutation paths.
Doc comments clearly state the "why" for the security choices.
