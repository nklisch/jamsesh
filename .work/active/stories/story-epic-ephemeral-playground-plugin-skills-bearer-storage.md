---
id: story-epic-ephemeral-playground-plugin-skills-bearer-storage
kind: story
stage: implementing
tags: [plugin]
parent: feature-epic-ephemeral-playground-plugin-skills
depends_on: [story-foundation-doc-drift-bearer-storage-architecture]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Unified per-session bearer storage + migration

## Scope

Story 2 of the parent feature. Migrates the legacy account-wide
`${CLAUDE_PLUGIN_DATA}/token` to per-session
`${CLAUDE_PLUGIN_DATA}/sessions/<id>/token` files. Adds:

- `state.ReadSessionToken(sessionID)` and
  `state.WriteSessionToken(sessionID, token)` helpers
- `state.MigrateToPerSessionTokens(logger)` one-shot idempotent migration
- `main.go` call site invoking the migration at startup
- Update refresh-flow callsite to write new access tokens to per-session
  paths instead of the legacy account-wide path

The legacy `token` file gets replaced with a `MIGRATED_TO_PER_SESSION`
stub after successful fan-out. Refresh tokens stay at the legacy
`refresh_token` path (account-wide; not per-session).

Full design in the parent feature body's "Story 2" section.

## Files delivered

- `cmd/jamsesh/state/state.go` (extend)
- `cmd/jamsesh/state/migrate.go` (new)
- `cmd/jamsesh/state/state_test.go` (extend)
- `cmd/jamsesh/state/migrate_test.go` (new)
- `cmd/jamsesh/main.go` (modify) — call migration on startup
- `cmd/jamsesh/portalclient/refresh.go` (modify if exists; otherwise
  wherever the refresh-flow callsite lives) — write refreshed access
  token to per-session path

## Acceptance criteria

See parent feature body's "Story 2 acceptance criteria" section.

## Notes

- Migration is idempotent — safe to run on every binary invocation
  (cheap stub-check on the legacy file).
- Partial-failure resilient — if one session's per-session write fails,
  the others succeed; next invocation retries the failed ones (legacy
  file isn't replaced with stub until ALL sessions succeed).
- Don't fail the binary's main action on migration error — log warning
  and continue.
- The refresh-flow callsite is a small change but easy to miss. The
  parent feature body's Risks section calls this out — verify during
  implementation that no code path writes to the legacy `token` file
  after this story lands (other than the stub-replacement write at
  migration time).

## Implementation notes

- `state.go` extended with `ReadSessionToken(sessionID)`, `WriteSessionToken(sessionID, token)` (creates session dir via `os.MkdirAll`), and `ListSessions()` (enumerates subdirs; returns nil on missing sessions dir).
- `state/migrate.go` introduces `Logger interface{ Warn(msg string, args ...any) }` and `MigrateToPerSessionTokens(logger Logger)`. The `migratedStub` constant (`"MIGRATED_TO_PER_SESSION"`) is unexported; only the helpers in the same package compare against it.
- `state/migrate_test.go` covers 7 branches: fresh install, already-migrated stub, successful fan-out to 2 sessions, no-sessions (empty sessions dir), partial failure (chmod-based write block on one session dir), idempotent after success, and skip-already-migrated-session.
- `main.go` uses an inline `stderrLogger` struct that satisfies `state.Logger`; migration errors are printed as warnings and do not stop the binary.
- `portalclient/refresh.go`: `doRefresh` now writes the new access token via `state.WriteSessionToken(sessID, ...)` when `state.CurrentSessionID()` returns a bound session; falls back to the legacy `state.WriteToken(...)` for unbound invocations (e.g. standalone auth flows). Refresh tokens remain at the account-wide `refresh_token` path — unchanged.
- All existing tests pass; 24 total tests in `cmd/jamsesh/state/...`, full suite green.

## Review (2026-05-23)

**Verdict**: Request changes (one blocker — foundation-doc drift)

**Summary**: The migration helper itself is well-built — idempotent,
partial-failure-resilient, atomic writes, comprehensive test coverage
(7 branches in `migrate_test.go`, 5 in `state_test.go`). The deliberate
deviation from the original design — adding a zero-sessions guard so
the legacy token is preserved when no session is bound — is the right
call and is documented in the test comment. All explicit acceptance
criteria in the story body are met. Two concerns surfaced from the
broader unified-storage contract: one blocker (foundation-doc drift)
and one important cross-cutting cleanup that the story scope did not
include.

**Blockers**:
- **Foundation-doc drift in `docs/ARCHITECTURE.md`**: lines 124-133
  describe `jamsesh auth` and `jamsesh mcp-headers` writing/reading
  `${CLAUDE_PLUGIN_DATA}/token` as the canonical token path. After
  this story lands, that file may be a `MIGRATED_TO_PER_SESSION` stub
  and the canonical path is `sessions/<id>/token`. The local-state
  layout diagram (lines 135-147) is also missing the per-session
  files. Per the rolling-foundation principle (a hard project rule),
  this must be rolled forward.
  - Item: `story-foundation-doc-drift-bearer-storage-architecture`
    (created at `.work/active/stories/`, stage:implementing)
  - Declared as a `depends_on` of this story so it is unblocked first.

**Important**:
- **Remaining `state.ReadToken()` callers will get the stub
  post-migration**: `portalclient/client.go:attachBearer`,
  `sessioncmd/{new,fork,join}.go` still call `state.ReadToken()`.
  After migration runs, they receive the literal string
  `MIGRATED_TO_PER_SESSION` and send it as a bearer. The zero-sessions
  guard in `migrate.go` narrows the blast radius (no migration runs
  until at least one session is bound), so for pre-launch this is a
  latent bug, not a live one. Should still be cleaned up before the
  first external release.
  - Item: `idea-state-readtoken-callers-post-migration-cleanup`
    (created in backlog)

**Nits**:
- `state/migrate.go` line 14 doc-comment claims `*slog.Logger` and
  `log.Logger` satisfy `Logger`. `*slog.Logger.Warn(msg string, args
  ...any)` is correct, but the stdlib `log.Logger` has no `Warn`
  method. The comment is slightly misleading. Trivial copy-edit.
- `state/state.go:131-141` `WriteSessionToken` creates the directory
  via `os.MkdirAll` then delegates to the package-level `Write`, which
  reaches `os.CreateTemp(dir, ...)` rooted at the plugin data dir
  rather than the session subdir. The temp file is therefore created
  one level above the target before the rename moves it into the
  session subdir. Cross-directory `os.Rename` on the same filesystem
  is fine and atomic on POSIX, but it means the temp-cleanup path
  leaks a `.jamsesh-write-*` file into the plugin data dir on rename
  failure rather than into the session subdir. Not behavior-breaking,
  just a small surprise if anyone audits the dir during a failed
  rename. No action required.

**Notes**: The deliberate zero-sessions deviation (lines 54-61 of
migrate.go and the `TestMigrate_noSessions` test docstring) is a
better design than the originally-spec'd "always write stub" behavior.
Worth flagging in the parent feature's review notes so future
unified-storage work follows the same "preserve legacy when no session
context exists" intuition.
