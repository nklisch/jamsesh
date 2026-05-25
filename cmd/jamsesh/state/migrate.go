package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
// (${data-dir}/token) into per-session token files at
// ${data-dir}/sessions/<id>/token for every session directory that
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

// DetectCCManagedLegacyData inspects the environment for the pre-rename
// CLAUDE_PLUGIN_DATA variable and, if set, checks whether that directory
// contains state files (token, refresh_token, sessions/) that the new
// JAMSESH_DATA_DIR path is missing.
//
// (idea-data-dir-migration-helper)
//
// On detection: logs a structured warning naming both paths and the
// suggested `mv` invocation. Does NOT auto-move — a silent move could
// surprise self-hosters who manage shared directories, and the cost of a
// manual mv-then-restart is small for a one-time upgrade.
//
// Idempotent: safe to call on every binary invocation. Returns true when
// a warning was emitted (the caller can use this to gate a one-time
// notice in interactive flows), false otherwise.
func DetectCCManagedLegacyData(logger Logger) (warned bool) {
	ccPath := os.Getenv("CLAUDE_PLUGIN_DATA")
	if ccPath == "" {
		return false
	}
	ccPath = filepath.Clean(ccPath)

	// If the new data dir is the same path the old env pointed at, there's
	// nothing to migrate — they're using the same directory.
	newPath, err := DataDir()
	if err != nil {
		return false
	}
	if filepath.Clean(newPath) == ccPath {
		return false
	}

	// Does the old path actually have state worth migrating?
	hasContent, err := hasMigratableState(ccPath)
	if err != nil || !hasContent {
		return false
	}

	// Is the new path effectively empty (no token, no sessions)? If the user
	// already migrated manually, don't nag.
	newHasContent, _ := hasMigratableState(newPath)
	if newHasContent {
		return false
	}

	logger.Warn(
		"detected legacy CLAUDE_PLUGIN_DATA directory with state that has not been migrated to JAMSESH_DATA_DIR; "+
			"to migrate, move the directory contents and restart your session",
		"legacy_path", ccPath,
		"new_path", newPath,
		"suggested_command", fmt.Sprintf("mv %q/* %q/ && rmdir %q", ccPath, newPath, ccPath),
	)
	return true
}

// hasMigratableState reports whether dir contains at least one of
// `token`, `refresh_token`, or a non-empty `sessions/` subdirectory.
// Used by DetectCCManagedLegacyData to decide whether the legacy path is
// worth flagging.
func hasMigratableState(dir string) (bool, error) {
	for _, name := range []string{"token", "refresh_token"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	sessions := filepath.Join(dir, "sessions")
	entries, err := os.ReadDir(sessions)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() {
			return true, nil
		}
	}
	return false, nil
}
