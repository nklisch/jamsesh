package state

import (
	"errors"
	"io/fs"
)

// migratedStub is written to the legacy token file once migration is complete.
// Its presence indicates that per-session token files are canonical.
const migratedStub = "MIGRATED_TO_PER_SESSION"

// Logger is the minimal logging interface consumed by MigrateToPerSessionTokens.
// *slog.Logger, log.Logger, and any custom type that exposes Warn satisfy it.
type Logger interface {
	Warn(msg string, args ...any)
}

// MigrateToPerSessionTokens fans out the legacy account-wide token
// (${CLAUDE_PLUGIN_DATA}/token) into per-session token files at
// ${CLAUDE_PLUGIN_DATA}/sessions/<id>/token for every session directory that
// exists under sessions/.
//
// Migration is idempotent and safe to call on every binary invocation:
//   - No-op if the legacy token file does not exist (fresh install).
//   - No-op if the legacy token file already contains the MIGRATED_TO_PER_SESSION stub.
//
// Partial-failure resilience: if writing one session's token fails, migration
// continues with the remaining sessions and logs a warning. The legacy token
// file is only replaced with the stub once ALL sessions have been handled
// without a fatal error. On partial failure the stub is not written, so the
// next invocation will retry the remaining sessions.
//
// Errors are non-fatal to the caller — MigrateToPerSessionTokens returns an
// error only for unexpected I/O failures reading the legacy file or the
// sessions directory; per-session write failures are logged and counted, but
// do not produce a return error.
func MigrateToPerSessionTokens(logger Logger) error {
	legacy, err := Read("token")
	if errors.Is(err, fs.ErrNotExist) {
		return nil // fresh install — nothing to migrate
	}
	if err != nil {
		return err
	}
	if string(legacy) == migratedStub {
		return nil // already migrated
	}

	sessions, err := ListSessions()
	if err != nil {
		return err
	}

	// If there are no sessions yet, the legacy token is still the only token
	// available (e.g. user has authed but not joined a session). Leave the
	// legacy file intact so that mcp-headers and other consumers can still use
	// it. Migration will complete on the next invocation after a session is
	// created.
	if len(sessions) == 0 {
		return nil
	}

	failCount := 0
	for _, sessID := range sessions {
		// Skip sessions that already have a per-session token (idempotent).
		if _, err := ReadSessionToken(sessID); err == nil {
			continue
		}

		if err := WriteSessionToken(sessID, legacy); err != nil {
			logger.Warn("migration: failed to write per-session token",
				"session_id", sessID,
				"err", err,
			)
			failCount++
			continue
		}
	}

	// Only mark migration complete when every session was handled successfully.
	// If any write failed, leave the stub unwritten so the next invocation retries.
	if failCount > 0 {
		return nil
	}

	return Write("token", []byte(migratedStub), 0o600)
}
