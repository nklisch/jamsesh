---
id: story-epic-ephemeral-playground-cli-first-creation-new
kind: story
stage: done
tags: [plugin]
parent: feature-epic-ephemeral-playground-cli-first-creation
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# `jamsesh new` subcommand (CLI side)

## Scope

Implements the `jamsesh new` Go subcommand end-to-end on the CLI side.
Covers Units 1-7 from the parent feature's design body (registration,
orchestrator, param resolution, multi-org picker, create-session API call,
push-base-ref, state-file writing) plus the calls into the invite helpers
owned by the sibling Story B (`-invite`).

Also lands the `docs/UX.md` roll-forward describing the CLI-first
creation flow — the parent feature's `## Risks` section calls this out
as the natural home for the doc update (this is the main user-facing
surface).

Does NOT include: the standalone `jamsesh invite` subcommand
(Story B's responsibility), the portal post-receive `base_sha` stamping
fix (Story C's responsibility), or the `/jamsesh:new` SKILL.md body
(plugin-skills feature in wave 3).

## Units delivered

1. `cmd/jamsesh/sessioncmd/new.go` — entire file (NewCommand, newAction,
   resolveCreateParams, pickOrgInteractive, createSessionAPI,
   pushBaseRef, writeSessionState, buildPortalClient,
   parseInviteEmails, sendInvitesIfRequested wiring)
2. `cmd/jamsesh/sessioncmd/git.go` extension if needed — `runGitWithEnv`
   helper added if not already present; existing `runGit`/`runGitOutput`
   pattern preserved
3. `cmd/jamsesh/main.go` — register `sessioncmd.NewCommand()` in the
   top-level command list
4. `cmd/jamsesh/sessioncmd/new_test.go` — full test suite per the parent
   feature's Testing section (9 test functions enumerated)
5. `cmd/jamsesh/sessioncmd/testhelpers_test.go` — extend with
   `setupNewEnv(t, srvURL) testEnv` (paralleling existing `setupJoinEnv`)
6. `docs/UX.md` — `Flow: creating a session` section reworked to describe
   the unified `jamsesh new` flow, with a forward reference to
   `jamsesh new --playground` (shipped later in this epic by
   `session-lifecycle` feature)

## Acceptance criteria

(Aggregated from the parent feature's Units 1-7 acceptance criteria; see
the feature body for the per-unit breakdown.)

- [ ] `jamsesh new --help` shows the documented flags and usage
- [ ] In a clean repo with at least one commit, `jamsesh new --org <id>`
      creates a session, pushes HEAD as base, writes per-session state
      files, prints a success summary, exit 0
- [ ] Multi-org interactive picker pre-selects the most-recently-used org
      (best-effort via `last_org_id` state file)
- [ ] `--non-interactive` (or detected non-TTY) without `--org` fails
      with an explicit "pass --org" error message
- [ ] On push failure, the session row stays live with `base_sha: NULL`;
      the CLI prints the explicit retry command
      `git push <session-remote> HEAD:refs/heads/jam/<sessionID>/base`;
      exit 1
- [ ] `--invite a@x,b@y` triggers the invite calls; partial failures are
      warnings, not errors
- [ ] No new entries appear in `.git/config` after a successful run
      (token-via-extraHeader approach, not `git remote add`)
- [ ] All listed `new_test.go` tests pass; suite runs in under 5s
      (no real network, no real git fetch)
- [ ] `docs/UX.md` updated; new flow description reads cleanly alongside
      the existing portal-UI flow (or replaces it cleanly per the
      CLI-first unification)

## Implementation notes

### Units completed

All 6 units (1-7 from the parent feature, plus Unit 8 stubs) delivered:

1. **`cmd/jamsesh/sessioncmd/new.go`** — `NewCommand()`, `newAction`,
   `resolveCreateParams`, `pickOrgInteractive`, `createSessionAPI`,
   `pushBaseRef`, `writeNewSessionState`, `buildPortalClient`,
   `parseInviteEmails`, `sendInvitesIfRequested`, `normalizeScope`,
   `printSuccessSummary`, `wrapPushError`.
2. **`cmd/jamsesh/sessioncmd/git.go` extension** — `runGitWithEnv` added
   as a package-level var using the same function-pointer pattern as
   `runGit`/`runGitOutput`.
3. **`cmd/jamsesh/main.go`** — `sessioncmd.NewCommand()` registered.
4. **`cmd/jamsesh/sessioncmd/new_test.go`** — 11 test functions (all 9
   from the feature spec plus `TestNormalizeScope` and `TestParseInviteEmails`
   as unit helpers).
5. **`cmd/jamsesh/sessioncmd/testhelpers_test.go`** — `buildCLIApp()` now
   includes `NewCommand()`.
6. **`docs/UX.md`** — "Flow: creating a session" reworked to describe
   the CLI-first `jamsesh new` flow with agent-primary path, interactive
   path, and forward reference to `--playground`.

### Deviations from the design skeleton

- **`writeNewSessionState`** named differently from the design's
  `writeSessionState` to avoid shadowing the existing `writeSessionState`
  in `join.go` (same package). The existing join helper writes
  `instance_id`; the new one intentionally does not.
- **`runGitWithEnv`** added to `new.go` (not `git.go`) since it's only
  needed there. Same package-level var pattern as `runGit`/`runGitOutput`.
- **`sendInvitesIfRequested`** uses `map[string]string{"email": email}`
  instead of `openapi.InviteRequest{Email: email}` because
  `openapi_types.Email` is a distinct type that requires a cast and the
  plain map serializes identically. Avoids importing `openapi_types`.
- **Invite endpoint confirmed**: `POST /api/orgs/{orgID}/sessions/{sessionID}/invites`
  (not the flat `/api/sessions/{id}/invites` form from the design sketch).
  The orgID is available on `session.OrgId` returned from create.
- **`golang.org/x/term` not needed** — `mattn/go-isatty` (already an
  indirect dep) provides `IsTerminal(fd uintptr)` with the same semantics.
  No `go get` required.
- **`isTTY` package-level var** overrideable in tests; `readStdinLine`
  also extracted as a package-level var so multi-org picker stdin can be
  stubbed without a real pipe.

### Portal handler gap check (goal empty string)

The portal's `CreateSession` handler in
`internal/portal/sessions/handler.go` was inspected. It does NOT reject
empty `goal` strings — the validation only enforces max 4096 chars.
No handler change needed.

### Verification status

- `go build ./cmd/jamsesh/...` — PASS
- `go test ./cmd/jamsesh/... ./internal/portal/sessions/...` — PASS (all)
- `go vet ./cmd/jamsesh/...` — PASS
- Pre-existing vet failures in `internal/portal/handlerauth_test` and
  `internal/portal/playground` are not related to this story.

## Notes for the implementing agent

- The parent feature's design body has the full code skeletons for each
  unit — they're starting points, not final code. Adjust to match the
  actual `portalclient` type signatures, the `state` package's helpers,
  and the OpenAPI-generated structs as you discover them.
- The most likely surprise: the OpenAPI spec marks `goal` as required.
  The locked design decision says optional. Resolution per the feature
  body: CLI always sends a value (empty string if user didn't provide).
  If the portal handler rejects empty strings, lift that rejection in
  the same PR (small change in `internal/portal/sessions/handler.go`).
- Token-via-extraHeader is the right call; don't switch to URL-embedded
  credentials even if a git push error nudges you that way. See Unit 6
  notes in the feature body for why.
- The `instance_id` state file is intentionally NOT written by `new` —
  binding happens at first attach. Document this in the unit if anything
  about the pattern surprises you.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `pushBaseRef` line 274 validates `headSHA` then discards with
  `_ = headSHA`. The actual validation is the rev-parse success check
  (line 269-272), so the discard is harmless but the comment could be
  tighter ("validation only; refspec uses HEAD" would be clearer).
- Story-body acceptance-criteria checkboxes left as `[ ]` rather than
  flipped to `[x]` after completion — cosmetic.

**Notes**: All 9 enumerated acceptance criteria are covered by 11 test
functions in `new_test.go` (the 9 named in the spec plus
`TestNormalizeScope` and `TestParseInviteEmails` as helper unit tests).
`go build ./cmd/jamsesh/...`, `go vet ./cmd/jamsesh/...`, and
`go test ./cmd/jamsesh/sessioncmd/...` all pass.

Design deviations documented in the implementation notes are sensible
and called out clearly:
- `writeNewSessionState` rename avoids shadowing `join.go`'s helper.
- `mattn/go-isatty` reused instead of adding `golang.org/x/term`.
- Invite endpoint is org-scoped (`/api/orgs/{org}/sessions/{id}/invites`)
  per actual OpenAPI surface, not the flat path from the design sketch.
- `map[string]string{"email": ...}` body for invites avoids the
  `openapi_types.Email` cast — serializes identically.

Security checks: HTTP Basic via `-c http.extraHeader` (process-local) is
the correct approach; no URL-embedded credentials, no `git remote add`
side-effect on user's `.git/config`. Comments in `pushBaseRef` document
why future refactors should not switch.

Foundation-doc roll-forward (docs/UX.md "Flow: creating a session"
rewrite to CLI-first agent-primary + interactive paths) lands cleanly
and is still intact after subsequent gate-docs commits.

Sibling commits to `new.go` (the `--playground` flag from
`session-lifecycle-cli-playground-flag`) build on top without
regressing this story's surface — `parseInviteEmails`/`sendInvitesIfRequested`
were moved to `invite.go` by the invite-sibling story; the helpers
still work correctly from this story's callsite via the same package.
