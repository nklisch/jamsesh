---
id: story-epic-ephemeral-playground-cli-first-creation-invite
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

# `jamsesh invite` subcommand (CLI side)

## Scope

Implements the standalone `jamsesh invite <session-id> <emails>` Go
subcommand (Unit 9 from the parent feature's design body). Both the
inline `--invite` flag on `jamsesh new` AND the standalone subcommand
ultimately call the same underlying helper (`sendInvitesIfRequested` +
`parseInviteEmails`) — this story owns the helper implementation; the
sibling `-new` story calls into it.

Does NOT include: the `jamsesh new` subcommand (Story `-new` owns it),
the post-receive `base_sha` stamping fix (Story `-base-sha` owns it).

## First action: verify the invite endpoint shape

The parent feature's design body assumed
`POST /api/sessions/{id}/invites` but did NOT confirm during the
Explore phase. Before writing CLI code, read `docs/openapi.yaml` and
find the actual invite endpoint:

- If `POST /api/sessions/{id}/invites` exists: proceed as designed
- If the path is `POST /api/orgs/{org}/sessions/{id}/invites`:
  adjust `sendInvitesIfRequested` to fetch the org from session state
  (read `${CLAUDE_PLUGIN_DATA}/sessions/<id>/org_id`)
- If no invite endpoint exists: this is an unexpected scope expansion —
  STOP and append a `## Blocker` section to this story body; autopilot
  will skip and surface. The portal-side invite endpoint creation is a
  much larger ask (handler + storage + email-sender wiring) and shouldn't
  be silently absorbed into this story.

## Units delivered

1. `cmd/jamsesh/sessioncmd/invite.go` — `InviteCommand()`,
   `inviteAction()`, plus the shared helpers `parseInviteEmails()` and
   `sendInvitesIfRequested()` (the latter called by Story `-new`'s
   `newAction` for the `--invite` flag pathway)
2. `cmd/jamsesh/main.go` — register `sessioncmd.InviteCommand()` in the
   top-level command list (next to `NewCommand()`)
3. `cmd/jamsesh/sessioncmd/invite_test.go` — test suite per the parent
   feature's Testing section (4 test functions enumerated for Story B)

## Acceptance criteria

- [ ] `jamsesh invite sess123 a@x.com,b@y.com` sends two invites, prints
      per-email status, exit 0
- [ ] `jamsesh invite sess123 a@x.com b@y.com c@z.com` (space-separated
      positional args) ALSO works (parsing joins them)
- [ ] Missing session ID or emails → usage error, exit 1
- [ ] Partial failure (e.g., one email returns 400 invalid-email) →
      stderr lists per-email outcomes, exit 1
- [ ] `parseInviteEmails` handles commas, spaces, mixed whitespace,
      empty entries (trimmed) — covered by table-driven test
- [ ] Tests run in under 2s; no real network

## Notes for the implementing agent

- The agent-primary mental model from the parent feature applies here
  too: humans typically have their CC agent invoke this via a future
  `/jamsesh:invite` skill (owned by plugin-skills feature). Keep the
  output parseable enough that an agent reading it can summarize the
  outcome to the human ("invited alice and bob; carol's email bounced").
- Per the locked feature decision, invites are non-atomic — partial
  success is reported but doesn't fail the whole batch.
- This story's helpers (`parseInviteEmails`, `sendInvitesIfRequested`)
  are called by Story `-new`. They must land in this story's file
  (`invite.go`); Story `-new`'s file imports from the same package.
  Implementation note: Story `-new` and Story `-invite` are
  parallel-implemented; if Story `-new`'s author lands first, they may
  inline the helpers temporarily — Story `-invite`'s author then moves
  them to `invite.go` and updates the call site. Either order works.

## Implementation notes

The wave-1 `cli-first-creation-new` commit had already stubbed
`parseInviteEmails` and `sendInvitesIfRequested` inline in `new.go` with the
correct org-scoped endpoint (`POST /api/orgs/{orgID}/sessions/{sessionID}/invites`).
This story moved both helpers to `invite.go` (same `sessioncmd` package) and
deleted them from `new.go`; the callsite in `newAction` required no change
beyond the file relocation.

**Endpoint confirmed**: `POST /api/orgs/{orgID}/sessions/{sessionID}/invites`.
OrgID resolution: `--org` flag overrides; falls back to
`${CLAUDE_PLUGIN_DATA}/sessions/<id>/org_id` state file written by `jamsesh new`.

**Files delivered**:
- `cmd/jamsesh/sessioncmd/invite.go` — new; `InviteCommand()`, `inviteAction()`,
  `parseInviteEmails()`, `sendInvitesIfRequested()`
- `cmd/jamsesh/sessioncmd/invite_test.go` — new; 7 test functions covering
  happy path, space/comma args, partial failure, usage errors, state-file
  org resolution, missing-org error
- `cmd/jamsesh/sessioncmd/new.go` — helpers removed (moved to invite.go)
- `cmd/jamsesh/sessioncmd/testhelpers_test.go` — `InviteCommand()` added to
  `buildCLIApp()`
- `cmd/jamsesh/main.go` — `sessioncmd.InviteCommand()` registered next to
  `NewCommand()`

**Verification**: `go build ./cmd/jamsesh/...` clean; all 48 tests in
`sessioncmd` pass; `go vet ./...` clean.

## Review (2026-05-23)

**Verdict**: Approve with comments

**Blockers**: none
**Important**:
- `cli-invite-dedupe-parseinviteemails-test` — `TestParseInviteEmails` is
  now defined twice (`new_test.go:849` and `invite_test.go:235` as
  `_inviteFile`). The implementer flagged this in an inline comment as
  deferred cleanup. Filed as a backlog item to drop the `new_test.go`
  copy and rename the `invite_test.go` copy back to the canonical name.

**Nits**:
- Acceptance criterion wording says "exit 1" for partial failure — the
  implementation returns a non-nil error from `Action`, and `main.go`
  prints it and exits 1. Behaviorally correct; no change needed.
- `url.PathEscape(orgID)` and `url.PathEscape(sessionID)` are used when
  constructing the invite path — good defensive hygiene against
  shell-injected or test-mangled IDs.

**Notes**: Lenses applied — Correctness, Tests, Design alignment,
Security (lightweight), Foundation-doc alignment, Naming. Endpoint shape
(`POST /api/orgs/{orgID}/sessions/{sessionID}/invites`) confirmed against
`docs/openapi.yaml:2367`. Request body `{"email": ...}` matches
`InviteRequest` schema. `state.Read` properly wraps `fs.ErrNotExist` so
the `errors.Is` check in `inviteAction` works as intended. The helper
move from `new.go` → `invite.go` is package-local, so the sibling Story
`-new`'s callsite in `newAction` continues to resolve via package scope
(verified by clean build). 7 invite tests pass.
