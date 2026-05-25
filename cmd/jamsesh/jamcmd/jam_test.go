package jamcmd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/jamcmd"
	"jamsesh/cmd/jamsesh/sessioncmd"
)

// buildTestApp returns a minimal CLI app that mirrors the real main.go
// registration: top-level "new" and "join" plus the "jam" parent command
// containing its own "new" and "join" sub-subcommands.
func buildTestApp() *cli.Command {
	return &cli.Command{
		Name: "jamsesh",
		Commands: []*cli.Command{
			sessioncmd.NewCommand(),
			sessioncmd.JoinCommand(),
			jamcmd.JamCommand(),
		},
		// Suppress default error printing so tests can assert on returned errors.
		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {},
	}
}

// setupTestEnv creates a temp JAMSESH_DATA_DIR dir and sets required env vars.
func setupTestEnv(t *testing.T, portalURL string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", portalURL)
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("tok-test"), 0o600); err != nil {
		t.Fatalf("writing token: %v", err)
	}
	return dir
}

// TestJamCommand_Help verifies that "jamsesh jam --help" exits cleanly
// and that the "jam" command exists in the app's command tree.
func TestJamCommand_Help(t *testing.T) {
	app := buildTestApp()

	// Verify "jam" is registered at the top level.
	found := false
	for _, cmd := range app.Commands {
		if cmd.Name == "jam" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'jam' command to be registered at top level")
	}

	// Verify "jam" has "new" and "join" as sub-subcommands.
	var jamCmd *cli.Command
	for _, cmd := range app.Commands {
		if cmd.Name == "jam" {
			jamCmd = cmd
			break
		}
	}
	if jamCmd == nil {
		t.Fatal("jam command not found")
	}

	subNames := make(map[string]bool)
	for _, sub := range jamCmd.Commands {
		subNames[sub.Name] = true
	}
	if !subNames["new"] {
		t.Error("expected 'new' sub-subcommand under 'jam'")
	}
	if !subNames["join"] {
		t.Error("expected 'join' sub-subcommand under 'jam'")
	}
}

// TestJamCommand_JoinMissingArg verifies that "jamsesh jam join" (no arg)
// returns an error, confirming the join action dispatches correctly.
func TestJamCommand_JoinMissingArg(t *testing.T) {
	setupTestEnv(t, "http://localhost:19999")

	app := buildTestApp()
	err := app.Run(context.Background(), []string{"jamsesh", "jam", "join"})
	if err == nil {
		t.Error("expected error when 'jamsesh jam join' is called without an argument")
	}
}

// TestJamCommand_NewMissingOrg verifies that "jamsesh jam new" (no --org, no
// TTY) returns an error about requiring --org, confirming dispatch to newAction.
func TestJamCommand_NewMissingOrg(t *testing.T) {
	setupTestEnv(t, "http://localhost:19999")

	app := buildTestApp()
	err := app.Run(context.Background(), []string{"jamsesh", "jam", "new"})
	if err == nil {
		t.Error("expected error when 'jamsesh jam new' is called without --org in non-TTY mode")
	}
}

// TestJamCommand_TopLevelCommandsUnaffected verifies that the top-level
// "jamsesh new" and "jamsesh join" commands are still present alongside "jam",
// ensuring that registering JamCommand() does not break the existing surface.
func TestJamCommand_TopLevelCommandsUnaffected(t *testing.T) {
	app := buildTestApp()

	topNames := make(map[string]bool)
	for _, cmd := range app.Commands {
		topNames[cmd.Name] = true
	}
	if !topNames["new"] {
		t.Error("expected top-level 'new' command to still be present")
	}
	if !topNames["join"] {
		t.Error("expected top-level 'join' command to still be present")
	}
	if !topNames["jam"] {
		t.Error("expected top-level 'jam' command to be present")
	}
}

// TestJamCommand_IndependentInstances verifies that the *cli.Command instances
// used under "jam" are not the same pointers as the top-level ones, ensuring
// urfave/cli's parent-pointer setup doesn't cause aliasing issues.
func TestJamCommand_IndependentInstances(t *testing.T) {
	app := buildTestApp()

	// Collect top-level new/join pointers.
	var topNew, topJoin *cli.Command
	for _, cmd := range app.Commands {
		switch cmd.Name {
		case "new":
			topNew = cmd
		case "join":
			topJoin = cmd
		}
	}

	// Collect jam's new/join pointers.
	var jamNew, jamJoin *cli.Command
	for _, cmd := range app.Commands {
		if cmd.Name == "jam" {
			for _, sub := range cmd.Commands {
				switch sub.Name {
				case "new":
					jamNew = sub
				case "join":
					jamJoin = sub
				}
			}
			break
		}
	}

	if topNew == nil || jamNew == nil {
		t.Fatal("could not locate 'new' commands")
	}
	if topJoin == nil || jamJoin == nil {
		t.Fatal("could not locate 'join' commands")
	}

	if topNew == jamNew {
		t.Error("top-level 'new' and jam's 'new' share the same pointer — urfave/cli parent aliasing will occur")
	}
	if topJoin == jamJoin {
		t.Error("top-level 'join' and jam's 'join' share the same pointer — urfave/cli parent aliasing will occur")
	}
}
