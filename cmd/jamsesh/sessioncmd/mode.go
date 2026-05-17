package sessioncmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/state"
)

// ModeCommand returns the urfave/cli command descriptor for "jamsesh mode".
func ModeCommand() *cli.Command {
	return &cli.Command{
		Name:      "mode",
		Usage:     "Set the current session's mode (sync or isolated)",
		ArgsUsage: "<sync|isolated>",
		Action:    modeAction,
	}
}

func modeAction(_ context.Context, cmd *cli.Command) error {
	newMode := cmd.Args().First()
	if newMode != "sync" && newMode != "isolated" {
		return fmt.Errorf("mode must be 'sync' or 'isolated', got %q", newMode)
	}

	sessionID, err := ResolveSession()
	if err != nil {
		return err
	}

	// Write the mode to the per-session local state file.
	modePath := filepath.Join("sessions", sessionID, "mode")
	if err := state.Write(modePath, []byte(newMode), 0o600); err != nil {
		return fmt.Errorf("writing local mode state: %w", err)
	}

	fmt.Printf("Local mode set to %s.\n", newMode)
	fmt.Println("Note: server-side mode change pending v1 follow-up. The portal still uses the session's default_mode for now.")
	return nil
}
