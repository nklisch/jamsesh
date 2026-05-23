---
id: story-epic-ephemeral-playground-cli-first-creation-new
kind: story
stage: implementing
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
