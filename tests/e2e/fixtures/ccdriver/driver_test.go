package ccdriver_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"jamsesh/tests/e2e/fixtures/ccdriver"
)

// TestRunHookInheritsHostPath verifies that runHook passes the host environment
// to the subprocess. It uses a shell script as the fake binary: the script
// checks for a sentinel env var set via ExtraEnv and writes valid JSON when
// found, confirming both host env inheritance and ExtraEnv override work.
func TestRunHookInheritsHostPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script test is POSIX-only")
	}

	// Verify PATH is non-empty in the test process itself.
	if os.Getenv("PATH") == "" {
		t.Fatal("host PATH is empty — test environment is misconfigured")
	}

	// Script 1: assert PATH is non-empty (inherited from host).
	// Writes valid JSON on success so the Driver doesn't return a parse error.
	pathCheckBin := filepath.Join(t.TempDir(), "path_check")
	pathCheckScript := "#!/bin/sh\ncat > /dev/null\nif [ -n \"$PATH\" ]; then\n  printf '{}'\nfi\n"
	if err := os.WriteFile(pathCheckBin, []byte(pathCheckScript), 0o755); err != nil {
		t.Fatalf("write path_check script: %v", err)
	}

	d := &ccdriver.Driver{
		BinaryPath: pathCheckBin,
		DataDir:    t.TempDir(),
	}
	if _, err := d.SessionEnd(context.Background(), ccdriver.SessionEndInput{
		SessionID:      "test-path-001",
		TranscriptPath: filepath.Join(t.TempDir(), "transcript.json"),
	}); err != nil {
		t.Fatalf("PATH not inherited by subprocess: %v", err)
	}

	// Script 2: assert ExtraEnv variables take effect (and thus can override
	// host env values, satisfying the "ExtraEnv takes precedence" criterion).
	sentinelBin := filepath.Join(t.TempDir(), "sentinel_check")
	sentinelScript := "#!/bin/sh\ncat > /dev/null\nif [ \"$JAMSESH_TEST_SENTINEL\" = \"expected_value\" ]; then\n  printf '{}'\nfi\n"
	if err := os.WriteFile(sentinelBin, []byte(sentinelScript), 0o755); err != nil {
		t.Fatalf("write sentinel script: %v", err)
	}

	d2 := &ccdriver.Driver{
		BinaryPath: sentinelBin,
		DataDir:    t.TempDir(),
		ExtraEnv:   []string{"JAMSESH_TEST_SENTINEL=expected_value"},
	}
	if _, err := d2.SessionEnd(context.Background(), ccdriver.SessionEndInput{
		SessionID:      "test-extra-env-001",
		TranscriptPath: filepath.Join(t.TempDir(), "transcript.json"),
	}); err != nil {
		t.Fatalf("ExtraEnv not forwarded to subprocess: %v", err)
	}

	// Script 3: confirm that CLAUDE_PLUGIN_DATA is present (appended last,
	// so it always overrides any conflicting ExtraEnv value).
	pluginDataBin := filepath.Join(t.TempDir(), "plugin_data_check")
	pluginDataScript := "#!/bin/sh\ncat > /dev/null\nif [ -n \"$CLAUDE_PLUGIN_DATA\" ]; then\n  printf '{}'\nfi\n"
	if err := os.WriteFile(pluginDataBin, []byte(pluginDataScript), 0o755); err != nil {
		t.Fatalf("write plugin_data script: %v", err)
	}

	d3 := &ccdriver.Driver{
		BinaryPath: pluginDataBin,
		DataDir:    t.TempDir(),
	}
	if _, err := d3.SessionEnd(context.Background(), ccdriver.SessionEndInput{
		SessionID:      "test-plugin-data-001",
		TranscriptPath: filepath.Join(t.TempDir(), "transcript.json"),
	}); err != nil {
		t.Fatalf("CLAUDE_PLUGIN_DATA not forwarded to subprocess: %v", err)
	}

}
