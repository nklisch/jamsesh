package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// withDataDir sets JAMSESH_DATA_DIR to dir for the duration of the test
// and restores the original value (or unsets) on cleanup.
func withDataDir(t *testing.T, dir string) {
	t.Helper()
	orig, had := os.LookupEnv("JAMSESH_DATA_DIR")
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("JAMSESH_DATA_DIR", orig)
		} else {
			_ = os.Unsetenv("JAMSESH_DATA_DIR")
		}
	})
}

// withPluginData is an alias for withDataDir retained for test-helper compatibility.
func withPluginData(t *testing.T, dir string) {
	t.Helper()
	withDataDir(t, dir)
}

// TestDataDir_envOverride asserts JAMSESH_DATA_DIR is honoured when set.
func TestDataDir_envOverride(t *testing.T) {
	want := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", want)
	got, err := DataDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
}

// TestDataDir_xdgDefault asserts the XDG fallback path when JAMSESH_DATA_DIR is unset.
func TestDataDir_xdgDefault(t *testing.T) {
	t.Setenv("JAMSESH_DATA_DIR", "")
	xdgBase := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgBase)
	got, err := DataDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(xdgBase, "jamsesh")
	if got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
	// Directory must have been created.
	if _, statErr := os.Stat(got); statErr != nil {
		t.Errorf("DataDir() did not create directory: %v", statErr)
	}
}

// TestReadWrite_roundTrip verifies a basic write-then-read round-trip.
func TestReadWrite_roundTrip(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	payload := []byte("hello-world")
	if err := Write("myfile", payload, 0o600); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Read("myfile")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("Read() = %q, want %q", got, payload)
	}
}

// TestRead_notExist verifies that Read returns fs.ErrNotExist when the file
// is absent.
func TestRead_notExist(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	_, err := Read("nonexistent")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read missing file: got %v, want fs.ErrNotExist chain", err)
	}
}

// TestWrite_atomicNoTempLeakage verifies that after a successful Write the
// temp file does not remain in the data directory.
func TestWrite_atomicNoTempLeakage(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	if err := Write("token", []byte("abc123"), 0o600); err != nil {
		t.Fatalf("Write: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		// The only file should be "token"; no temp files should survive.
		if name != "token" {
			t.Errorf("unexpected file in data dir after Write: %q", name)
		}
	}
}

// TestWrite_mode0600 verifies that WriteToken creates a file with mode 0600.
func TestWrite_mode0600(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	if err := WriteToken("mytoken"); err != nil {
		t.Fatalf("WriteToken: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "token"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	got := info.Mode().Perm()
	if got != 0o600 {
		t.Errorf("token file mode = %04o, want %04o", got, 0o600)
	}
}

// TestReadToken_trimWhitespace verifies that ReadToken strips surrounding
// whitespace (e.g. trailing newline written by some editors/scripts).
func TestReadToken_trimWhitespace(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	raw := "mytoken\n"
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tok, err := ReadToken()
	if err != nil {
		t.Fatalf("ReadToken: %v", err)
	}
	if tok != "mytoken" {
		t.Errorf("ReadToken() = %q, want %q", tok, "mytoken")
	}
}

// TestReadToken_notExist verifies ReadToken propagates fs.ErrNotExist.
func TestReadToken_notExist(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	_, err := ReadToken()
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadToken missing token: got %v, want fs.ErrNotExist chain", err)
	}
}

// TestReadRefreshToken_roundTrip verifies write+read of the refresh token.
func TestReadRefreshToken_roundTrip(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	const rt = "refresh-xyz"
	if err := WriteRefreshToken(rt); err != nil {
		t.Fatalf("WriteRefreshToken: %v", err)
	}
	got, err := ReadRefreshToken()
	if err != nil {
		t.Fatalf("ReadRefreshToken: %v", err)
	}
	if got != rt {
		t.Errorf("ReadRefreshToken() = %q, want %q", got, rt)
	}
}

// TestReadPortalURL_envPrecedence verifies JAMSESH_PORTAL_URL overrides file.
func TestReadPortalURL_envPrecedence(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	// Write a portal_url file.
	if err := os.WriteFile(filepath.Join(dir, "portal_url"), []byte("https://from-file.example.com"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("JAMSESH_PORTAL_URL", "https://from-env.example.com")
	got, err := ReadPortalURL()
	if err != nil {
		t.Fatalf("ReadPortalURL: %v", err)
	}
	if got != "https://from-env.example.com" {
		t.Errorf("ReadPortalURL() = %q, want env value", got)
	}
}

// TestReadPortalURL_fileOverDefault verifies file beats the built-in default.
func TestReadPortalURL_fileOverDefault(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)
	t.Setenv("JAMSESH_PORTAL_URL", "") // ensure env is empty

	const fileURL = "https://self-hosted.example.com"
	if err := os.WriteFile(filepath.Join(dir, "portal_url"), []byte(fileURL), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ReadPortalURL()
	if err != nil {
		t.Fatalf("ReadPortalURL: %v", err)
	}
	if got != fileURL {
		t.Errorf("ReadPortalURL() = %q, want %q", got, fileURL)
	}
}

// TestReadPortalURL_default verifies the built-in default is returned when
// neither env nor file is present.
func TestReadPortalURL_default(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)
	t.Setenv("JAMSESH_PORTAL_URL", "")

	got, err := ReadPortalURL()
	if err != nil {
		t.Fatalf("ReadPortalURL: %v", err)
	}
	if got != DefaultPortalURL {
		t.Errorf("ReadPortalURL() = %q, want default %q", got, DefaultPortalURL)
	}
}

// TestReadSessionToken_notExist verifies ReadSessionToken propagates
// fs.ErrNotExist when the per-session token file is absent.
func TestReadSessionToken_notExist(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	_, err := ReadSessionToken("sess-abc")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadSessionToken missing: got %v, want fs.ErrNotExist chain", err)
	}
}

// TestWriteSessionToken_roundTrip verifies that WriteSessionToken creates the
// parent directory and writes a readable token at mode 0600.
func TestWriteSessionToken_roundTrip(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	const sessID = "sess-xyz"
	payload := []byte("my-bearer-token")

	if err := WriteSessionToken(sessID, payload); err != nil {
		t.Fatalf("WriteSessionToken: %v", err)
	}

	got, err := ReadSessionToken(sessID)
	if err != nil {
		t.Fatalf("ReadSessionToken: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("ReadSessionToken() = %q, want %q", got, payload)
	}

	// Verify mode 0600.
	info, err := os.Stat(filepath.Join(dir, "sessions", sessID, "token"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("per-session token mode = %04o, want %04o", got, 0o600)
	}
}

// TestWriteSessionToken_createsDir verifies that WriteSessionToken creates the
// sessions/<id>/ directory even when it does not already exist.
func TestWriteSessionToken_createsDir(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	// Ensure no sessions directory exists yet.
	sessionsDir := filepath.Join(dir, "sessions")
	if _, err := os.Stat(sessionsDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected sessions dir absent, got: %v", err)
	}

	if err := WriteSessionToken("new-sess", []byte("tok")); err != nil {
		t.Fatalf("WriteSessionToken: %v", err)
	}

	_, err := ReadSessionToken("new-sess")
	if err != nil {
		t.Fatalf("ReadSessionToken after create: %v", err)
	}
}

// TestListSessions_empty verifies ListSessions returns nil/empty when there is
// no sessions directory.
func TestListSessions_empty(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	ids, err := ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ListSessions() = %v, want empty", ids)
	}
}

// TestReadCurrentBearer_PerSessionHit verifies that ReadCurrentBearer returns
// the per-session token when sessionID is set and the per-session file exists.
func TestReadCurrentBearer_PerSessionHit(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	const sessID = "sess-hit"
	if err := WriteSessionToken(sessID, []byte("per-session-bearer\n")); err != nil {
		t.Fatalf("WriteSessionToken: %v", err)
	}

	got, err := ReadCurrentBearer(sessID)
	if err != nil {
		t.Fatalf("ReadCurrentBearer: %v", err)
	}
	// Whitespace must be trimmed.
	if got != "per-session-bearer" {
		t.Errorf("ReadCurrentBearer() = %q, want %q", got, "per-session-bearer")
	}
}

// TestReadCurrentBearer_PerSessionMiss_FallsBackToLegacy verifies that when the
// per-session token is absent the helper falls back to the legacy token.
func TestReadCurrentBearer_PerSessionMiss_FallsBackToLegacy(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	// Write only the legacy token — no per-session file for "sess-miss".
	if err := WriteToken("legacy-bearer"); err != nil {
		t.Fatalf("WriteToken: %v", err)
	}

	got, err := ReadCurrentBearer("sess-miss")
	if err != nil {
		t.Fatalf("ReadCurrentBearer: %v", err)
	}
	if got != "legacy-bearer" {
		t.Errorf("ReadCurrentBearer() = %q, want %q", got, "legacy-bearer")
	}
}

// TestReadCurrentBearer_EmptySessionID_UsesLegacy verifies that passing an
// empty sessionID bypasses per-session lookup entirely and reads the legacy token.
func TestReadCurrentBearer_EmptySessionID_UsesLegacy(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	if err := WriteToken("legacy-only"); err != nil {
		t.Fatalf("WriteToken: %v", err)
	}

	got, err := ReadCurrentBearer("")
	if err != nil {
		t.Fatalf("ReadCurrentBearer: %v", err)
	}
	if got != "legacy-only" {
		t.Errorf("ReadCurrentBearer() = %q, want %q", got, "legacy-only")
	}
}

// TestReadCurrentBearer_PostMigrationStub_PassesThrough verifies that when no
// sessionID is provided and the legacy token is the migration stub, the stub is
// returned as-is (interpreting the stub is the caller's responsibility).
func TestReadCurrentBearer_PostMigrationStub_PassesThrough(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	const stub = "MIGRATED_TO_PER_SESSION"
	if err := WriteToken(stub); err != nil {
		t.Fatalf("WriteToken: %v", err)
	}

	got, err := ReadCurrentBearer("")
	if err != nil {
		t.Fatalf("ReadCurrentBearer: %v", err)
	}
	if got != stub {
		t.Errorf("ReadCurrentBearer() = %q, want stub %q", got, stub)
	}
}

// ---------------------------------------------------------------------------
// Per-session token isolation: ReadCurrentBearer call-site contract
// ---------------------------------------------------------------------------
//
// feature-state-readtoken-per-session-sweep extended ReadCurrentBearer so that
// every call-site that has a bound session ID reads the per-session file at
// sessions/<id>/token rather than the legacy account-wide token file.
//
// The tests below verify the isolation contract:
//   - Tokens written for session-A are NOT visible when session-B is requested.
//   - The legacy fallback is only taken when the per-session file is absent.
//   - An empty sessionID always bypasses per-session lookup entirely.
//
// The naming convention mirrors the callsites swept in the feature:
//   - "status" callsite: ReadSessionToken(sessID)  — direct per-session read
//   - "fork / new (bound)" callsite: ReadCurrentBearer(sessID) — per-session preferred
//   - "join / new (pre-binding)" callsite: ReadCurrentBearer("") — legacy only

// TestReadCurrentBearer_SessionIsolation verifies that tokens written for
// session-A are NOT returned when ReadCurrentBearer is called with session-B's ID.
// This is the core isolation invariant: per-session tokens must be scoped.
func TestReadCurrentBearer_SessionIsolation(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	const (
		sessA  = "sess-isolation-a"
		sessB  = "sess-isolation-b"
		tokA   = "bearer-for-session-a"
		tokB   = "bearer-for-session-b"
	)

	// Write distinct per-session tokens for both sessions.
	if err := WriteSessionToken(sessA, []byte(tokA)); err != nil {
		t.Fatalf("WriteSessionToken(A): %v", err)
	}
	if err := WriteSessionToken(sessB, []byte(tokB)); err != nil {
		t.Fatalf("WriteSessionToken(B): %v", err)
	}

	// Session-A lookup must return tokA, not tokB.
	gotA, err := ReadCurrentBearer(sessA)
	if err != nil {
		t.Fatalf("ReadCurrentBearer(A): %v", err)
	}
	if gotA != tokA {
		t.Errorf("session-A: got %q, want %q", gotA, tokA)
	}

	// Session-B lookup must return tokB, not tokA.
	gotB, err := ReadCurrentBearer(sessB)
	if err != nil {
		t.Fatalf("ReadCurrentBearer(B): %v", err)
	}
	if gotB != tokB {
		t.Errorf("session-B: got %q, want %q", gotB, tokB)
	}

	// Cross-check: neither session leaks the other's token.
	if gotA == gotB {
		t.Errorf("isolation failure: session-A and session-B returned the same token %q", gotA)
	}
}

// TestReadCurrentBearer_BoundCallsite_PrefersPerSession models the "fork/new
// bound-session" callsite pattern: state.ReadCurrentBearer(sessionID) where
// sessionID is non-empty. The per-session file must win over the legacy file.
func TestReadCurrentBearer_BoundCallsite_PrefersPerSession(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	const sessID = "sess-bound-callsite"

	// Write a legacy token (simulates a pre-migration install).
	if err := WriteToken("legacy-account-token"); err != nil {
		t.Fatalf("WriteToken: %v", err)
	}
	// Write a per-session token for sessID (simulates post-migration state).
	if err := WriteSessionToken(sessID, []byte("per-session-token")); err != nil {
		t.Fatalf("WriteSessionToken: %v", err)
	}

	got, err := ReadCurrentBearer(sessID)
	if err != nil {
		t.Fatalf("ReadCurrentBearer: %v", err)
	}
	// The per-session token must shadow the legacy one.
	if got != "per-session-token" {
		t.Errorf("bound callsite: got %q, want per-session-token", got)
	}
}

// TestReadCurrentBearer_PreBindingCallsite_UsesLegacy models the "join/new
// pre-binding" callsite pattern: state.ReadCurrentBearer("") where no session
// is bound yet. Must fall back to the legacy token.
func TestReadCurrentBearer_PreBindingCallsite_UsesLegacy(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	// Legacy token present; no session-specific tokens exist.
	if err := WriteToken("pre-binding-bearer"); err != nil {
		t.Fatalf("WriteToken: %v", err)
	}

	got, err := ReadCurrentBearer("")
	if err != nil {
		t.Fatalf("ReadCurrentBearer: %v", err)
	}
	if got != "pre-binding-bearer" {
		t.Errorf("pre-binding callsite: got %q, want pre-binding-bearer", got)
	}
}

// TestReadSessionToken_StatusCallsite_PerSessionDirect models the "status"
// callsite: state.ReadSessionToken(sessID) is called directly (not via
// ReadCurrentBearer). Verifies the per-session contract: token is isolated
// per session and missing returns fs.ErrNotExist.
func TestReadSessionToken_StatusCallsite_PerSessionDirect(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	const (
		sessID  = "sess-status-callsite"
		payload = "status-bearer-token"
	)

	// Pre-condition: token absent.
	_, err := ReadSessionToken(sessID)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("before write: want fs.ErrNotExist, got %v", err)
	}

	// Write per-session token.
	if err := WriteSessionToken(sessID, []byte(payload)); err != nil {
		t.Fatalf("WriteSessionToken: %v", err)
	}

	// Direct per-session read must return the written payload.
	got, err := ReadSessionToken(sessID)
	if err != nil {
		t.Fatalf("ReadSessionToken: %v", err)
	}
	if string(got) != payload {
		t.Errorf("ReadSessionToken() = %q, want %q", got, payload)
	}
}

// TestReadCurrentBearer_MultiSession_EachIsolated runs a table of sessions and
// verifies that ReadCurrentBearer returns the correct per-session token for
// each, with no cross-contamination. This exercises the sweep across multiple
// concurrent bindings — the scenario the feature-state-readtoken-per-session-sweep
// was designed for.
func TestReadCurrentBearer_MultiSession_EachIsolated(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	sessions := []struct {
		id    string
		token string
	}{
		{"sess-multi-alpha", "tok-alpha"},
		{"sess-multi-beta", "tok-beta"},
		{"sess-multi-gamma", "tok-gamma"},
	}

	// Write distinct per-session tokens.
	for _, s := range sessions {
		if err := WriteSessionToken(s.id, []byte(s.token)); err != nil {
			t.Fatalf("WriteSessionToken(%s): %v", s.id, err)
		}
	}

	// Verify each session returns only its own token.
	for _, s := range sessions {
		got, err := ReadCurrentBearer(s.id)
		if err != nil {
			t.Fatalf("ReadCurrentBearer(%s): %v", s.id, err)
		}
		if got != s.token {
			t.Errorf("session %s: got %q, want %q", s.id, got, s.token)
		}
	}
}

// TestListSessions_returnsSubdirs verifies ListSessions enumerates session
// subdirectory names and ignores non-directory entries.
func TestListSessions_returnsSubdirs(t *testing.T) {
	dir := t.TempDir()
	withPluginData(t, dir)

	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create two session subdirs and one stray file.
	for _, name := range []string{"sess-1", "sess-2"} {
		if err := os.Mkdir(filepath.Join(sessionsDir, name), 0o700); err != nil {
			t.Fatalf("Mkdir %s: %v", name, err)
		}
	}
	// Stray file — must NOT appear in results.
	if err := os.WriteFile(filepath.Join(sessionsDir, "stray.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ids, err := ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	want := map[string]bool{"sess-1": true, "sess-2": true}
	if len(ids) != len(want) {
		t.Fatalf("ListSessions() = %v, want %v", ids, want)
	}
	for _, id := range ids {
		if !want[id] {
			t.Errorf("unexpected session id %q in ListSessions()", id)
		}
	}
}
