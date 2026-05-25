package mcpheaders_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles the jamsesh binary into a temp dir and returns the
// path. Skips the test if compilation fails (e.g. missing dependencies in
// CI without network). Caches across subtests using t.TempDir().
func buildBinary(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()

	_ = os.Chmod(binDir, 0o700)
	binPath := filepath.Join(binDir, "jamsesh")
	cmd := exec.Command("go", "build", "-o", binPath, "jamsesh/cmd/jamsesh")
	cmd.Dir = findModRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("skipping: could not build jamsesh binary: %v\n%s", err, out)
	}
	return binPath
}

// findModRoot walks up from the test file's location to find go.mod.
func findModRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod from test directory")
		}
		dir = parent
	}
}

// writeFile is a test helper that creates a file (and any missing parent
// directories) with the given content and mode 0o600.
func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile %q: %v", path, err)
	}
}

// TestMcpHeaders_tokenPresent builds the binary and runs `jamsesh mcp-headers`
// with a real token file but no bound session. Only Authorization should be
// present.
func TestMcpHeaders_tokenPresent(t *testing.T) {
	bin := buildBinary(t)
	dataDir := t.TempDir()


	_ = os.Chmod(dataDir, 0o700)
	writeFile(t, filepath.Join(dataDir, "token"), "my-test-token")

	cmd := exec.Command(bin, "mcp-headers")
	// No CLAUDE_SESSION_ID → no Jam-Session-Id header.
	cmd.Env = append(os.Environ(), "JAMSESH_DATA_DIR="+dataDir, "CLAUDE_SESSION_ID=")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("jamsesh mcp-headers: %v (output: %s)", err, out)
	}

	var headers map[string]string
	if err := json.Unmarshal(out, &headers); err != nil {
		t.Fatalf("decoding output JSON: %v (raw: %s)", err, out)
	}
	want := "Bearer my-test-token"
	if headers["Authorization"] != want {
		t.Errorf("Authorization = %q, want %q", headers["Authorization"], want)
	}
	if _, present := headers["Jam-Session-Id"]; present {
		t.Errorf("Jam-Session-Id unexpectedly present: %q", headers["Jam-Session-Id"])
	}
}

// TestMcpHeaders_tokenAndSession verifies that when a CC instance has a bound
// session, both Authorization and Jam-Session-Id are emitted.
func TestMcpHeaders_tokenAndSession(t *testing.T) {
	bin := buildBinary(t)
	dataDir := t.TempDir()


	_ = os.Chmod(dataDir, 0o700)
	const (
		ccInstanceID = "cc-inst-abc123"
		jamSessionID = "jamsesh-session-xyz789"
	)

	writeFile(t, filepath.Join(dataDir, "token"), "my-test-token")
	// Write the instance_id binding file under sessions/<jamSessionID>/instance_id.
	writeFile(t,
		filepath.Join(dataDir, "sessions", jamSessionID, "instance_id"),
		ccInstanceID,
	)

	cmd := exec.Command(bin, "mcp-headers")
	cmd.Env = append(os.Environ(),
		"JAMSESH_DATA_DIR="+dataDir,
		"CLAUDE_SESSION_ID="+ccInstanceID,
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("jamsesh mcp-headers: %v (output: %s)", err, out)
	}

	var headers map[string]string
	if err := json.Unmarshal(out, &headers); err != nil {
		t.Fatalf("decoding output JSON: %v (raw: %s)", err, out)
	}
	if got := headers["Authorization"]; got != "Bearer my-test-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer my-test-token")
	}
	if got := headers["Jam-Session-Id"]; got != jamSessionID {
		t.Errorf("Jam-Session-Id = %q, want %q", got, jamSessionID)
	}
}

// TestMcpHeaders_tokenNoSession verifies that when a token exists but the CC
// instance has no bound session, only Authorization is emitted.
func TestMcpHeaders_tokenNoSession(t *testing.T) {
	bin := buildBinary(t)
	dataDir := t.TempDir()


	_ = os.Chmod(dataDir, 0o700)
	writeFile(t, filepath.Join(dataDir, "token"), "my-test-token")
	// sessions/ dir exists but no entry matches the instance ID.
	if err := os.MkdirAll(filepath.Join(dataDir, "sessions"), 0o700); err != nil {
		t.Fatalf("MkdirAll sessions: %v", err)
	}

	cmd := exec.Command(bin, "mcp-headers")
	cmd.Env = append(os.Environ(),
		"JAMSESH_DATA_DIR="+dataDir,
		"CLAUDE_SESSION_ID=some-unbound-instance",
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("jamsesh mcp-headers: %v (output: %s)", err, out)
	}

	var headers map[string]string
	if err := json.Unmarshal(out, &headers); err != nil {
		t.Fatalf("decoding output JSON: %v (raw: %s)", err, out)
	}
	if got := headers["Authorization"]; got != "Bearer my-test-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer my-test-token")
	}
	if _, present := headers["Jam-Session-Id"]; present {
		t.Errorf("Jam-Session-Id unexpectedly present: %q", headers["Jam-Session-Id"])
	}
}

// TestMcpHeaders_noToken verifies that a missing token file causes the binary
// to exit with code 2 and write a message to stderr.
func TestMcpHeaders_noToken(t *testing.T) {
	bin := buildBinary(t)
	dataDir := t.TempDir() // no token file

	cmd := exec.Command(bin, "mcp-headers")
	cmd.Env = append(os.Environ(), "JAMSESH_DATA_DIR="+dataDir)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit, got success")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("exit code = %d, want 2", exitErr.ExitCode())
	}
	if stderr.Len() == 0 {
		t.Error("expected stderr message, got empty")
	}
}
