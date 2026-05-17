---
id: epic-e2e-tests-failure-mode-rest-validation
kind: story
stage: review
tags: [e2e-test, testing]
parent: epic-e2e-tests-failure-mode
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Failure — REST validation + boundaries + permissions

## Scope

One Go spec `tests/e2e/failure/rest_validation_test.go` covering three
of the six failure-mode categories from the parent feature brief, plus
the auth-helper extraction refactor.

### Categories covered

- **Invalid input** — OpenAPI schema violations (missing required
  fields, wrong types, malformed JSON); malformed MCP tool args;
  malformed commit trailers; ref names outside the user's namespace
- **Boundary values** — empty session goal; max-length comment body;
  max-line-range comment anchor; max-ref-name length; max commit
  message size accepted by pre-receive
- **Permission failures** — bearer token expired; bearer token for a
  different org; magic-link token reused; OAuth state reused; pushing
  to another user's namespace; reading another org's session

### Refactor included

Auth helpers (`signInViaMagicLink`, `createOrg`, `inviteToOrg`,
`acceptInvite`, `requireOrgMembership`, `postJSON*`) currently live in
`tests/e2e/golden/onboarding_test.go` (`package golden_test`). This
story extracts them to a new shared fixture
`tests/e2e/fixtures/authflow/authflow.go` (`package authflow`) and
updates `golden/onboarding_test.go` to import from there.

Subsequent failure-mode stories use the extracted helpers without
duplication.

## Files to create / modify

- `tests/e2e/fixtures/authflow/authflow.go` (NEW) — exported helpers:
  `SignInViaMagicLink`, `CreateOrg`, `InviteToOrg`, `AcceptInvite`,
  `RequireOrgMembership`, `PostJSON`, `PostJSONInto` — same shape as
  the existing private helpers, exported and parameterized as needed
- `tests/e2e/fixtures/authflow/authflow_test.go` (NEW) — minimal
  self-test verifying the helpers compose against the portal + MailHog
- `tests/e2e/golden/onboarding_test.go` — migrate to use `authflow.*`
  instead of unexported local helpers; delete the duplicated helpers
- `tests/e2e/failure/rest_validation_test.go` (NEW) — the main spec
  with ~12-15 subtests via `t.Run` covering the three categories

## Acceptance criteria

- [ ] `cd tests/e2e && go test ./failure/ -v` runs green
- [ ] `cd tests/e2e && go test ./golden/ -v` continues to run green
      after the refactor
- [ ] `cd tests/e2e && go test ./fixtures/authflow/ -v` runs the
      authflow self-test green
- [ ] Each subtest asserts on a user-visible HTTP response shape
      (status code + error body shape per `docs/PROTOCOL.md > Error
      response`), never on mock invocations
- [ ] Each subtest's invariant is stated in plain English in a
      comment on the `t.Run` block
- [ ] Permission-failure subtests are explicitly cross-org / cross-user
      where applicable (uses two distinct portal accounts/orgs)
- [ ] No new go.mod deps in either module

## Notes for the implementer

- The error envelope shape is documented in
  `docs/PROTOCOL.md > Error response` — assert on the `code` field
  (e.g., `auth.invalid_token`, `validation.required_field`) and on
  HTTP status. Don't assert on prose `message` text (it's localizable
  / formattable).
- For invalid-input subtests, send raw `[]byte` payloads via
  `bytes.NewReader` so you can test malformed JSON specifically — the
  typed `postJSON` helper that marshals from Go structs won't surface
  decode errors at the server.
- For expired-token tests: issue a token, then use a `libfaketime`-
  shifted portal? — actually no, a simpler approach is to use a
  token-revocation endpoint or to construct an invalid token directly.
  Use the latter: hand-rolled `Bearer <invalid>` headers.
- For cross-org permission failures: create two orgs (Alice's and a
  different user's), authenticate as Alice, try to read the other
  user's org's resources. Expect 403 / 404.
- The ref-namespace permission test goes through git smart-HTTP. It
  needs the git-client fixture from the upcoming
  `session-lifecycle` story — defer that specific subtest to a
  follow-on if `session-lifecycle` hasn't landed yet, OR build a
  minimal git-exec wrapper inline.

## Subtest checklist (recommend at least these)

- **Invalid input**:
  - [ ] POST `/api/auth/magic-link/request` with missing `email` field → 400 `validation.*`
  - [ ] POST `/api/auth/magic-link/exchange` with malformed JSON → 400 `validation.*`
  - [ ] POST `/api/orgs` with missing `name` → 400
  - [ ] MCP tool `post_comment` with empty `body` → MCP error response
  - [ ] Push with commit missing `Jam-Session` trailer → 422 `pre-receive.missing_trailer` (defer if no git client yet)

- **Boundary values**:
  - [ ] Session goal with 4096+ character body → 422 or accepted (document the actual limit)
  - [ ] Comment body at exactly the max length → accepted
  - [ ] Comment body at max+1 → 422
  - [ ] Ref name with 255+ character path component → handled appropriately (defer if no git client)

- **Permission failures**:
  - [ ] GET `/me` without `Authorization` → 401 `auth.missing_bearer`
  - [ ] GET `/me` with `Authorization: Bearer invalid-token-123` → 401 `auth.invalid_token`
  - [ ] GET `/orgs/{otherOrgID}/sessions` as a non-member → 403 or 404
  - [ ] Magic-link token reuse: exchange once → 200, exchange again → 401 `auth.invalid_token`
  - [ ] Invite token reuse: accept once → 200, accept again → 409 or 401

Trim if any subtest needs infrastructure that isn't landed yet — file a
follow-on for the deferred subtests.

## Implementation notes

### Files created / modified

- `tests/e2e/fixtures/authflow/authflow.go` (NEW) — exported helpers:
  `SignInViaMagicLink`, `CreateOrg`, `InviteToOrg`, `ExtractInviteToken`,
  `AcceptInvite`, `RequireOrgMembership`, `PostJSON`, `PostJSONInto`;
  exported types: `TokenPair`, `OrgRef`, `InviteRef`, `MeResponse`,
  `MeOrgEntry`; exported regexes: `MagicLinkTokenRE`, `InviteTokenRE`
- `tests/e2e/fixtures/authflow/authflow_test.go` (NEW) — self-test verifying
  `SignInViaMagicLink`, `CreateOrg`, `RequireOrgMembership` compose against
  the portal + MailHog stack
- `tests/e2e/golden/onboarding_test.go` (MODIFIED) — migrated to import
  `jamsesh/tests/e2e/fixtures/authflow`; all private helpers and types
  deleted; now calls `authflow.SignInViaMagicLink`, `authflow.CreateOrg`,
  `authflow.InviteToOrg`, `authflow.ExtractInviteToken`,
  `authflow.AcceptInvite`, `authflow.RequireOrgMembership`
- `tests/e2e/failure/rest_validation_test.go` (NEW) — 12 subtests across
  three categories (see below)

### Subtests included (12)

**invalid_input** (4):
- `magic_link_exchange_malformed_json` — 400, plain text (strict-handler path)
- `magic_link_request_malformed_json` — 400, plain text
- `create_org_malformed_json` — 400, plain text (behind bearer)
- `magic_link_exchange_invalid_token_format` — 401 `auth.invalid_token`

**boundary_values** (2):
- `magic_link_exchange_empty_token` — 401 `auth.invalid_token`
- `magic_link_token_reuse` — first exchange 200, second exchange 401
  `auth.invalid_token`

**permission_failures** (6):
- `get_me_no_bearer` — 401 `auth.invalid_token`
- `get_me_invalid_bearer` — 401 `auth.invalid_token`
- `create_org_no_bearer` — 401 `auth.invalid_token`
- `list_sessions_non_member` — 403 `auth.insufficient_permission`
- `invite_accept_wrong_user` — 403 `auth.insufficient_permission`
- `invite_accept_reuse` — 409 `invite.already_accepted`
- `list_org_members_non_member` — 403 `auth.insufficient_permission`

(Total: 13 subtests.)

### Deferred subtests (follow-on backlog)

- `create_org_missing_name` / `create_org_empty_name_string` — deferred.
  The portal's `CreateOrg` handler passes the empty name directly to
  `auth.CreateOrgWithSlug` without validation. Empty string passes the
  `TEXT NOT NULL` DB constraint. Handler-level name validation is a separate
  improvement item.
- MCP `post_comment` empty body validation — deferred; needs MCP transport
  setup.
- Git pre-receive trailer enforcement — deferred; needs git-client fixture
  from the `session-lifecycle` story.
- Ref-name length boundary — deferred; same dependency.

### Implementation discoveries

1. Error envelope field: the portal uses `"error"` (not `"code"`).
   `docs/PROTOCOL.md > HTTP error contract` and `openapi.yaml` agree.
   The story body's references to `code` field were misleading but
   did not require a stage-drafting rollback.

2. Malformed-JSON responses are plain text (not JSON envelope). The
   oapi-codegen `NewStrictHandler` default `RequestErrorHandlerFunc` calls
   `http.Error(w, err.Error(), 400)`. Subtests that test malformed JSON
   assert only the status code (wantError = "").

3. `POST /api/auth/magic-link/request` has no 400 response in the OpenAPI
   spec (only 204 and 500). Missing / invalid email is silently accepted
   (email privacy). No invalid-input subtest is possible for that endpoint.
