package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
)

// TestMain_PlaygroundEnabledWithUnprotectedSlugCollision_Exits1 is an
// e2e-seam test that exercises the cmd/portal/main.go exit-1 path when
// JAMSESH_PLAYGROUND_ENABLED=true and a pre-existing unprotected org with
// slug "playground" is already in the DB.
//
// The test:
//  1. Builds the portal binary into t.TempDir().
//  2. Seeds a SQLite DB with an unprotected org at slug="playground".
//  3. Spawns the binary with JAMSESH_PLAYGROUND_ENABLED=true pointing at
//     that DB.
//  4. Asserts exit code 1 and that stderr contains both the "reserved slug"
//     phrase and the remediation hint.
func TestMain_PlaygroundEnabledWithUnprotectedSlugCollision_Exits1(t *testing.T) {
	// Build the portal binary. The test binary and the portal source share the
	// same module root, so ./cmd/portal/main.go is stable relative to the
	// module root. We resolve the module root via the source file's directory.
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "portal_test_bin")

	buildCmd := exec.Command("go", "build", "-o", binaryPath, "jamsesh/cmd/portal")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("go build portal: %v", err)
	}

	// Seed a SQLite DB on disk (not :memory:) so the subprocess can open it.
	dbPath := filepath.Join(dir, "test_seed.db")
	sqliteStore, _, err := db.Open(context.Background(), "sqlite", dbPath, db.PoolConfig{})
	if err != nil {
		t.Fatalf("open seeding sqlite db: %v", err)
	}

	// Insert an unprotected org with slug="playground". This mimics an
	// operator who created a regular org before enabling playground.
	_, err = sqliteStore.CreateOrg(context.Background(), store.CreateOrgParams{
		ID:        "org-pre-existing-unprotected",
		Name:      "Pre-existing Playground Org",
		Slug:      "playground",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		_ = sqliteStore.Close()
		t.Fatalf("seed unprotected playground org: %v", err)
	}
	_ = sqliteStore.Close()

	// Spawn the portal binary. The minimal env wires SQLite + playground.
	// No other config is needed: the process should exit 1 during playground
	// provisioning, before the server ever tries to bind.
	//
	// We pass a temp storage path and a high-numbered bind port to make intent
	// clear even though the binary should never reach those initialisation steps.
	storagePath := filepath.Join(dir, "storage")
	portalCmd := exec.Command(binaryPath)
	portalCmd.Env = []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
		"JAMSESH_DB_DRIVER=sqlite",
		"JAMSESH_DB_DSN=" + dbPath,
		"JAMSESH_PLAYGROUND_ENABLED=true",
		"JAMSESH_STORAGE=" + storagePath,
		"JAMSESH_TLS_MODE=behind_proxy",
		"JAMSESH_LOG_FORMAT=text", // easier to grep than JSON
	}

	// Capture combined output. slog.Error writes to stderr; capturing combined
	// output ensures we don't miss any routing of the log line.
	output, runErr := portalCmd.CombinedOutput()
	outputStr := string(output)

	// Verify the process exited with code 1.
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running portal binary: %v\noutput:\n%s", runErr, outputStr)
		}
	}

	if exitCode != 1 {
		t.Errorf("portal exit code = %d, want 1\noutput:\n%s", exitCode, outputStr)
	}

	// Verify the error message contains "reserved slug" — the key phrase
	// from the main.go slog.Error call and from ErrReservedSlugConflict.
	if !strings.Contains(outputStr, "reserved slug") {
		t.Errorf("portal output does not contain %q\noutput:\n%s", "reserved slug", outputStr)
	}

	// Verify the output contains the remediation hint so operators know how
	// to resolve the conflict.
	const remediationHint = "JAMSESH_PLAYGROUND_ENABLED=false"
	if !strings.Contains(outputStr, remediationHint) {
		t.Errorf("portal output does not contain remediation hint %q\noutput:\n%s", remediationHint, outputStr)
	}
}
