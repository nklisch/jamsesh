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

// TestMcpHeaders_tokenPresent builds the binary and runs `jamsesh mcp-headers`
// with a real token file, verifying the JSON output.
func TestMcpHeaders_tokenPresent(t *testing.T) {
	bin := buildBinary(t)
	dataDir := t.TempDir()

	// Write a token file.
	if err := os.WriteFile(filepath.Join(dataDir, "token"), []byte("my-test-token"), 0o600); err != nil {
		t.Fatalf("WriteFile token: %v", err)
	}

	cmd := exec.Command(bin, "mcp-headers")
	cmd.Env = append(os.Environ(), "CLAUDE_PLUGIN_DATA="+dataDir)
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
}

// TestMcpHeaders_noToken verifies that a missing token file causes the binary
// to exit with code 2 and write a message to stderr.
func TestMcpHeaders_noToken(t *testing.T) {
	bin := buildBinary(t)
	dataDir := t.TempDir() // no token file

	cmd := exec.Command(bin, "mcp-headers")
	cmd.Env = append(os.Environ(), "CLAUDE_PLUGIN_DATA="+dataDir)
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
