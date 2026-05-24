package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// testLogger is a minimal Logger that records Warn calls for test assertions.
type testLogger struct {
	warnings []logEntry
}

type logEntry struct {
	msg  string
	args []any
}

func (l *testLogger) Warn(msg string, args ...any) {
	l.warnings = append(l.warnings, logEntry{msg: msg, args: args})
}

// mkSessionDir creates the sessions/<id>/ subdirectory in dir and returns its path.
func mkSessionDir(t *testing.T, dir, sessID string) string {
	t.Helper()
	p := filepath.Join(dir, "sessions", sessID)
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatalf("mkSessionDir(%q): %v", sessID, err)
	}
	return p
}

// TestMigrate_freshInstall verifies that migration is a no-op when there is no
// legacy token file (fresh install).
func TestMigrate_freshInstall(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)
	logger := &testLogger{}

	if err := MigrateToPerSessionTokens(logger); err != nil {
		t.Fatalf("MigrateToPerSessionTokens: unexpected error: %v", err)
	}
	if len(logger.warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(logger.warnings))
	}
	// No stub should have been created.
	_, err := os.Stat(filepath.Join(dir, "token"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected token file to remain absent, got stat err: %v", err)
	}
}

// TestMigrate_alreadyMigrated verifies that migration is a no-op when the stub
// is already present (subsequent invocation after successful migration).
func TestMigrate_alreadyMigrated(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)
	logger := &testLogger{}

	// Write the stub.
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(migratedStub), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create a session that should NOT get a token written during this call.
	mkSessionDir(t, dir, "sess-1")

	if err := MigrateToPerSessionTokens(logger); err != nil {
		t.Fatalf("MigrateToPerSessionTokens: unexpected error: %v", err)
	}
	if len(logger.warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(logger.warnings))
	}

	// The per-session token for sess-1 must not have been written.
	_, err := os.Stat(filepath.Join(dir, "sessions", "sess-1", "token"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("per-session token should be absent, got stat err: %v", err)
	}
}

// TestMigrate_successfulFanOut verifies that a real legacy token is fanned out
// to all session directories and the stub is left at the legacy path.
func TestMigrate_successfulFanOut(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)
	logger := &testLogger{}

	const legacyTok = "legacy-bearer-abc"
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(legacyTok), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mkSessionDir(t, dir, "sess-a")
	mkSessionDir(t, dir, "sess-b")

	if err := MigrateToPerSessionTokens(logger); err != nil {
		t.Fatalf("MigrateToPerSessionTokens: unexpected error: %v", err)
	}
	if len(logger.warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(logger.warnings), logger.warnings)
	}

	// Both per-session tokens must match the legacy value.
	for _, sessID := range []string{"sess-a", "sess-b"} {
		got, err := os.ReadFile(filepath.Join(dir, "sessions", sessID, "token"))
		if err != nil {
			t.Errorf("reading per-session token for %s: %v", sessID, err)
			continue
		}
		if string(got) != legacyTok {
			t.Errorf("per-session token for %s = %q, want %q", sessID, got, legacyTok)
		}
	}

	// Legacy token must now contain the stub.
	stub, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		t.Fatalf("reading legacy token: %v", err)
	}
	if string(stub) != migratedStub {
		t.Errorf("legacy token = %q, want %q", stub, migratedStub)
	}
}

// TestMigrate_noSessions verifies that migration with a legacy token but zero
// session directories still completes and writes the stub.
func TestMigrate_noSessions(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)
	logger := &testLogger{}

	const legacyTok = "token-no-sessions"
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(legacyTok), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// No sessions directory created — simulates a user who has a legacy token
	// but has never bound a session.

	if err := MigrateToPerSessionTokens(logger); err != nil {
		t.Fatalf("MigrateToPerSessionTokens: unexpected error: %v", err)
	}
	if len(logger.warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(logger.warnings))
	}

	stub, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		t.Fatalf("reading legacy token after migration: %v", err)
	}
	if string(stub) != migratedStub {
		t.Errorf("legacy token = %q, want %q", stub, migratedStub)
	}
}

// TestMigrate_partialFailure verifies that when writing one session's token
// fails, the other sessions still succeed, the warning is logged, and the
// legacy stub is NOT written (allowing the next invocation to retry).
func TestMigrate_partialFailure(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)
	logger := &testLogger{}

	const legacyTok = "token-partial-fail"
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(legacyTok), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// sess-ok will succeed.
	mkSessionDir(t, dir, "sess-ok")

	// sess-fail: create the directory, then place an UNWRITABLE file where the
	// token would be written, so WriteSessionToken fails.
	failDir := mkSessionDir(t, dir, "sess-fail")
	tokenPath := filepath.Join(failDir, "token")
	if err := os.WriteFile(tokenPath, []byte("existing"), 0o000); err != nil {
		t.Fatalf("creating read-only token file: %v", err)
	}
	// Also make the directory itself unwritable so the temp-file rename fails.
	if err := os.Chmod(failDir, 0o500); err != nil {
		t.Fatalf("chmod session dir: %v", err)
	}
	// Restore permissions after the test so t.TempDir cleanup can succeed.
	t.Cleanup(func() {
		_ = os.Chmod(failDir, 0o700)
		_ = os.Chmod(tokenPath, 0o600)
	})

	if err := MigrateToPerSessionTokens(logger); err != nil {
		t.Fatalf("MigrateToPerSessionTokens: unexpected error: %v", err)
	}

	// Exactly one warning should have been logged.
	if len(logger.warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(logger.warnings), logger.warnings)
	}

	// sess-ok must have its token.
	got, err := os.ReadFile(filepath.Join(dir, "sessions", "sess-ok", "token"))
	if err != nil {
		t.Errorf("reading sess-ok token: %v", err)
	} else if string(got) != legacyTok {
		t.Errorf("sess-ok token = %q, want %q", got, legacyTok)
	}

	// The stub must NOT have been written because sess-fail failed.
	stub, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		t.Fatalf("reading legacy token: %v", err)
	}
	if string(stub) == migratedStub {
		t.Error("legacy token was replaced with stub despite partial failure — retry would be lost")
	}
}

// TestMigrate_idempotentAfterSuccess verifies that calling MigrateToPerSessionTokens
// twice in a row is safe: the second call sees the stub and does nothing.
func TestMigrate_idempotentAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)
	logger := &testLogger{}

	const legacyTok = "idempotent-bearer"
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(legacyTok), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mkSessionDir(t, dir, "sess-x")

	// First call — migrates.
	if err := MigrateToPerSessionTokens(logger); err != nil {
		t.Fatalf("first MigrateToPerSessionTokens: %v", err)
	}

	// Second call — must be a no-op.
	if err := MigrateToPerSessionTokens(logger); err != nil {
		t.Fatalf("second MigrateToPerSessionTokens: %v", err)
	}
	if len(logger.warnings) != 0 {
		t.Errorf("expected no warnings across both calls, got %d", len(logger.warnings))
	}

	// Token still correct.
	got, err := os.ReadFile(filepath.Join(dir, "sessions", "sess-x", "token"))
	if err != nil {
		t.Fatalf("reading per-session token: %v", err)
	}
	if string(got) != legacyTok {
		t.Errorf("per-session token = %q, want %q", got, legacyTok)
	}
}

// TestMigrate_skipAlreadyMigratedSession verifies that per-session tokens
// already written are not overwritten on a re-run.
func TestMigrate_skipAlreadyMigratedSession(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)
	logger := &testLogger{}

	const legacyTok = "old-legacy-token"
	const existingTok = "already-written-token"
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(legacyTok), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Pre-write a per-session token for sess-pre (simulates a prior partial run).
	sessDir := mkSessionDir(t, dir, "sess-pre")
	if err := os.WriteFile(filepath.Join(sessDir, "token"), []byte(existingTok), 0o600); err != nil {
		t.Fatalf("WriteFile existing token: %v", err)
	}

	if err := MigrateToPerSessionTokens(logger); err != nil {
		t.Fatalf("MigrateToPerSessionTokens: %v", err)
	}

	// Pre-existing per-session token must not be overwritten.
	got, err := os.ReadFile(filepath.Join(dir, "sessions", "sess-pre", "token"))
	if err != nil {
		t.Fatalf("reading per-session token: %v", err)
	}
	if string(got) != existingTok {
		t.Errorf("per-session token = %q, want %q (should not be overwritten)", got, existingTok)
	}
}
