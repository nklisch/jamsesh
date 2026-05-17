package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// withPluginData sets CLAUDE_PLUGIN_DATA to dir for the duration of the test
// and restores the original value (or unsets) on cleanup.
func withPluginData(t *testing.T, dir string) {
	t.Helper()
	orig, had := os.LookupEnv("CLAUDE_PLUGIN_DATA")
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("CLAUDE_PLUGIN_DATA", orig)
		} else {
			_ = os.Unsetenv("CLAUDE_PLUGIN_DATA")
		}
	})
}

// TestPluginDataDir_unset asserts an error when CLAUDE_PLUGIN_DATA is absent.
func TestPluginDataDir_unset(t *testing.T) {
	t.Setenv("CLAUDE_PLUGIN_DATA", "")
	_, err := PluginDataDir()
	if err == nil {
		t.Fatal("expected error when CLAUDE_PLUGIN_DATA is empty, got nil")
	}
}

// TestPluginDataDir_set asserts the env value is returned as-is.
func TestPluginDataDir_set(t *testing.T) {
	want := "/tmp/fake-plugin-data"
	t.Setenv("CLAUDE_PLUGIN_DATA", want)
	got, err := PluginDataDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("PluginDataDir() = %q, want %q", got, want)
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
