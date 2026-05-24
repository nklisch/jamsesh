---
id: story-epic-ephemeral-playground-session-lifecycle-cli-playground-flag
kind: story
stage: implementing
tags: [plugin, playground]
parent: feature-epic-ephemeral-playground-session-lifecycle
depends_on: [story-epic-ephemeral-playground-session-lifecycle-rest-endpoints]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# `jamsesh new --playground` CLI extension

## Scope

Story 4 of the parent feature. Extends the `jamsesh new` subcommand
(owned by wave-1 `cli-first-creation` feature) with a `--playground`
flag. When set, the subcommand:
1. Skips auth (no OAuth bearer required)
2. Skips org picker (creator joins the reserved `playground` org)
3. Calls `POST /api/playground/sessions` instead of `POST /api/orgs/{id}/sessions`
4. Writes the bearer received in the response to per-session token storage
5. Pushes local HEAD as base ref using the just-received bearer
6. Prints a playground-specific success summary (share URL + nickname +
   ends-in counter)

Full design in the parent feature body's "Story 4" section.

## Files delivered

- `cmd/jamsesh/sessioncmd/new.go` (modify, originally created by
  wave-1 `cli-first-creation`):
  - Add `&cli.BoolFlag{Name: "playground", ...}` to NewCommand flags
  - Branch early in `newAction`: if `--playground` set, dispatch to
    `newPlaygroundAction`
  - Add `newPlaygroundAction(ctx, cmd) error`
  - Add `pushBaseRefWithBearer(ctx, pc, sessionID, bearer)` — variant
    of wave-1's `pushBaseRef` that uses an explicit bearer rather than
    reading from `state.ReadToken()`
  - Add mutual-exclusion guard: `--playground` + `--org` returns error
- `cmd/jamsesh/sessioncmd/new_test.go` (extend) — playground-path tests
- `cmd/jamsesh/state/state.go` (modify if not already done by wave-3
  `plugin-skills`) — `WriteSessionToken(sessionID, bearer)` helper that
  writes to `${CLAUDE_PLUGIN_DATA}/sessions/<id>/token`

## Acceptance criteria

See the parent feature body's "Story 4 acceptance criteria" section.
Summary: `jamsesh new --playground` creates via the playground endpoint
(no auth), pushes HEAD with the just-received bearer, writes per-session
state, prints share URL + nickname + ends_at; mutually exclusive with
`--org`.

## Notes for the implementing agent

- This story extends a file owned by wave-1 cli-first-creation
  (`cmd/jamsesh/sessioncmd/new.go`). When wave-1 implements first
  (which it does — it's wave 1, this is wave 2), the file exists with
  the durable-path code. This story ADDS to it; doesn't rewrite.
- The `WriteSessionToken` helper is also referenced by wave-3
  `plugin-skills` feature (which owns the unified per-session bearer
  storage migration). If `plugin-skills` lands first (wave 3 vs wave 2
  — won't happen in practice), reuse its helper; otherwise this story
  introduces it and `plugin-skills` extends.
- `pushBaseRefWithBearer` is a thin variant of the wave-1
  `pushBaseRef`: same `-c http.extraHeader=...` token-injection
  approach (don't switch to URL-embedded credentials), but takes the
  bearer as a parameter rather than reading from
  `state.ReadToken()`. After Story 4 lands, the wave-1 `pushBaseRef`
  could refactor to call `pushBaseRefWithBearer(ctx, pc, sessionID,
  state.ReadToken())` — but that refactor is out of scope here.
- The success summary should match the visual feel of the parent epic's
  mockup step `02-create-cli-output.html`: share URL, "You are:
  <nickname>", "Ends: in <hard-cap-remaining> (or after <idle-remaining>
  idle)", "Open in browser? [Y/n]" prompt. The browser-open piece can
  reuse the existing `cmd/jamsesh/auth/` browser-open helper.

## Cross-story notes

- Depends on Story 1 (REST endpoints) because this story calls the
  endpoints Story 1 creates.
- Depends conceptually on wave-1 cli-first-creation having landed
  (the `new.go` file must exist). Cross-feature deps are inherited
  via the feature-level depends_on declared in the parent feature's
  frontmatter — no need to re-declare here at the story level.
