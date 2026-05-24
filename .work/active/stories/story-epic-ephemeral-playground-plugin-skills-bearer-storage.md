---
id: story-epic-ephemeral-playground-plugin-skills-bearer-storage
kind: story
stage: review
tags: [plugin]
parent: feature-epic-ephemeral-playground-plugin-skills
depends_on: []
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
