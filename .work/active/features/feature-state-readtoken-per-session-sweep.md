---
id: feature-state-readtoken-per-session-sweep
kind: feature
stage: drafting
tags: [plugin, cleanup, refactor]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Sweep remaining `state.ReadToken()` callers to per-session reads

## Brief

Following the bearer-storage migration (which now stubs the legacy
`${CLAUDE_PLUGIN_DATA}/token` file with `MIGRATED_TO_PER_SESSION`
after fan-out), 5 binary call-sites still call `state.ReadToken()`
and would receive the literal stub string post-migration. For the
intersection of `(legacy token exists) AND (≥1 bound session) AND
(these specific subcommands invoked after migration)`, the binary
sends `Authorization: Bearer MIGRATED_TO_PER_SESSION` and the
portal rejects with 401. Pre-launch this is latent (no installed
base); it becomes a real upgrade-path footgun the moment the first
external release goes out.

This feature sweeps all 5 callsites to the per-session-first /
legacy-fallback pattern, plus introduces a
`state.ReadCurrentBearer()` helper to reduce drift risk for future
callsites.

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

## Design notes (for /agile-workflow:feature-design)

Decisions for the design pass:

1. **One story vs per-callsite-cluster.** Either:
   - One story that does a clean sweep of all five callsites under
     the same per-session-first / legacy-fallback pattern (simpler;
     single PR, single review).
   - Two stories: one per callsite cluster (portalclient,
     sessioncmd) (smaller blast radius per PR; lets review focus on
     one surface at a time).

   Leaning toward the helper-extraction approach (next point) which
   makes the per-callsite work a one-line swap regardless of split.

2. **Introduce `state.ReadCurrentBearer()` helper.** Encapsulate the
   per-session-first / legacy-fallback pattern. Each callsite becomes
   a one-line swap from `state.ReadToken()` to
   `state.ReadCurrentBearer()`. Reduces drift risk for future
   callsites. The helper takes the bound session id (when known) and
   returns the per-session bearer if present, else falls back to the
   legacy token (which after migration is the stub — caller decides
   how to surface that failure).

3. **Auth-write callsites stay on legacy path.** `auth/browser.go:187`
   and `auth/device.go:106` write to the legacy path pre-binding —
   that is intentional and correct. After a write, the next binary
   invocation re-migrates.

## Acceptance (rollup)

- All 5 callsites swapped to `state.ReadCurrentBearer()` (or
  equivalent per-session-first reader)
- `state.ReadCurrentBearer()` helper lives in
  `cmd/jamsesh/state/` alongside existing helpers
- `git grep -n "state.ReadToken" -- 'cmd/'` returns only the
  auth-write callsites
- Tests cover both branches of the new helper (per-session present
  vs falls back to legacy)
- This lands BEFORE the first external release (release v0.4.0 or
  whichever is the first publicly distributed)
