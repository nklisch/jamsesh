---
id: idea-state-readtoken-callers-post-migration-cleanup
kind: idea
stage: backlog
tags: [plugin, cleanup]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Sweep remaining `state.ReadToken()` callers to per-session reads

## Source

Review finding (important) on
`story-epic-ephemeral-playground-plugin-skills-bearer-storage`.

The bearer-storage story migrated `mcp-headers` to per-session reads and
updated the refresh-flow write callsite. The migration replaces the
legacy `${CLAUDE_PLUGIN_DATA}/token` file with a
`MIGRATED_TO_PER_SESSION` stub after the fan-out succeeds. Any remaining
caller of `state.ReadToken()` will then get the literal stub string back
and use it as an `Authorization: Bearer MIGRATED_TO_PER_SESSION` header
— which the portal will reject with 401.

The zero-sessions guard added to `migrate.go` (lines 54-61) narrows the
blast radius significantly: if no session is bound yet, migration is a
no-op and the legacy token stays usable. The bug only manifests for the
intersection of `(legacy token exists) AND (at least one bound session)
AND (these specific subcommands are invoked after migration)`. For the
pre-launch product (no installed-base users) this is effectively a
non-issue at ship time, but it is a real latent bug for the
post-launch period.

## Remaining callers of `state.ReadToken()`

Found via `git grep -n "state.ReadToken" -- 'cmd/'`:

- `cmd/jamsesh/portalclient/client.go:93` — `attachBearer`, called on
  every portal HTTP request. The most important one to fix.
- `cmd/jamsesh/sessioncmd/new.go:282` — read for git Basic-auth push
- `cmd/jamsesh/sessioncmd/new.go:349` — `buildPortalClient` auth check
- `cmd/jamsesh/sessioncmd/fork.go:62` — read for MCP fork call
- `cmd/jamsesh/sessioncmd/join.go:70` — auth check on join

For each, the right shape is the same pattern the bearer-storage story
introduced in `mcp-headers`: try per-session first when a session is
bound, fall back to legacy for unbound invocations (e.g. standalone
auth flows that legitimately run pre-binding).

Auth write paths (`cmd/jamsesh/auth/browser.go:187`,
`cmd/jamsesh/auth/device.go:106`) write to the legacy path — that is
intentional and correct (they run pre-binding). After a write, the next
binary invocation will re-migrate.

## Suggested scope

Either:
- One feature `state-unified-per-session-token-reads` with one story
  per callsite cluster (portalclient, sessioncmd), or
- One story that does a clean sweep of all five callsites under the
  same per-session-first / legacy-fallback pattern.

## Notes

- Strongly suggest this lands BEFORE the first external release, since
  the bug becomes a real upgrade-path footgun once there are real users
  with both legacy tokens and bound sessions.
- Could also be implemented as a single `state.ReadCurrentBearer()`
  helper that encapsulates the per-session-first / legacy-fallback
  pattern, so each callsite is a one-line swap. That reduces drift risk
  if a future caller is added without remembering the pattern.
