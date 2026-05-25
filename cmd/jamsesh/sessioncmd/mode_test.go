package sessioncmd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/sessioncmd"
)

func TestModeCommand_writesStateFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("CC_SESSION_ID", "cc-inst-mode-test")

	// Set up session directory.
	sessDir := filepath.Join(dir, "sessions", "mode-session")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("creating session dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "instance_id"), []byte("cc-inst-mode-test"), 0o600); err != nil {
		t.Fatalf("writing instance_id: %v", err)
	}

	app := &cli.Command{
		Commands: []*cli.Command{sessioncmd.ModeCommand()},
	}

	// Set mode to sync.
	if err := app.Run(context.Background(), []string{"jamsesh", "mode", "sync"}); err != nil {
		t.Fatalf("mode sync: %v", err)
	}

	// Verify state file content.
	modeFile := filepath.Join(sessDir, "mode")
	got, err := os.ReadFile(modeFile)
	if err != nil {
		t.Fatalf("reading mode file: %v", err)
	}
	if string(got) != "sync" {
		t.Errorf("mode file content = %q, want %q", string(got), "sync")
	}

	// Now flip to isolated.
	if err := app.Run(context.Background(), []string{"jamsesh", "mode", "isolated"}); err != nil {
		t.Fatalf("mode isolated: %v", err)
	}

	got, err = os.ReadFile(modeFile)
	if err != nil {
		t.Fatalf("reading mode file after flip: %v", err)
	}
	if string(got) != "isolated" {
		t.Errorf("mode file content = %q, want %q", string(got), "isolated")
	}
}

func TestModeCommand_invalidMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	// Set up minimal session so we get past resolveSession.
	sessDir := filepath.Join(dir, "sessions", "s1")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("creating session dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "instance_id"), []byte("cc-inst-invalid"), 0o600); err != nil {
		t.Fatalf("writing instance_id: %v", err)
	}
	t.Setenv("CC_SESSION_ID", "cc-inst-invalid")

	app := &cli.Command{
		Commands: []*cli.Command{sessioncmd.ModeCommand()},
	}
	err := app.Run(context.Background(), []string{"jamsesh", "mode", "turbo"})
	if err == nil {
		t.Fatal("expected error for invalid mode, got nil")
	}
}

func TestModeCommand_noSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("CC_SESSION_ID", "nonexistent")

	app := &cli.Command{
		Commands: []*cli.Command{sessioncmd.ModeCommand()},
	}
	err := app.Run(context.Background(), []string{"jamsesh", "mode", "sync"})
	if err == nil {
		t.Fatal("expected error when no session found, got nil")
	}
}
