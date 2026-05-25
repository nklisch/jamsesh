package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/auth"
	"jamsesh/cmd/jamsesh/finalizecmd"
	"jamsesh/cmd/jamsesh/hooks"
	"jamsesh/cmd/jamsesh/jamcmd"
	"jamsesh/cmd/jamsesh/mcpheaders"
	"jamsesh/cmd/jamsesh/sessioncmd"
	"jamsesh/cmd/jamsesh/state"
	"jamsesh/internal/buildinfo"
)

// stderrLogger satisfies state.Logger using stderr output.
type stderrLogger struct{}

func (stderrLogger) Warn(msg string, args ...any) {
	fmt.Fprint(os.Stderr, "warning: ", msg)
	for i := 0; i+1 < len(args); i += 2 {
		fmt.Fprintf(os.Stderr, " %v=%v", args[i], args[i+1])
	}
	fmt.Fprintln(os.Stderr)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Migrate legacy account-wide token to per-session storage on every startup.
	// Non-fatal: a warning is logged on error; the binary continues normally.
	if err := state.MigrateToPerSessionTokens(stderrLogger{}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: token migration encountered errors: %v\n", err)
	}

	// Detect users who haven't migrated their data dir from the legacy
	// CLAUDE_PLUGIN_DATA path to JAMSESH_DATA_DIR. Best-effort + advisory —
	// emits a structured warning with the suggested `mv` command but does
	// not auto-move. (idea-data-dir-migration-helper)
	state.DetectCCManagedLegacyData(stderrLogger{})

	app := &cli.Command{
		Name:    "jamsesh",
		Usage:   "Local client for the jamsesh portal",
		Version: buildinfo.String(),
		Commands: []*cli.Command{
			auth.Command(),
			mcpheaders.Command(),
			hookCommand(),
			sessioncmd.NewCommand(),
			sessioncmd.InviteCommand(),
			sessioncmd.JoinCommand(),
			sessioncmd.StatusCommand(),
			sessioncmd.ForkCommand(),
			sessioncmd.ModeCommand(),
			finalizecmd.FinalizeCommand(),
			finalizecmd.FinalizeRunCommand(),
			jamcmd.JamCommand(),
		},
	}

	if err := app.Run(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// hookCommand returns the "hook" parent command with all six CC lifecycle-hook
// subcommands registered.
func hookCommand() *cli.Command {
	return &cli.Command{
		Name:  "hook",
		Usage: "CC lifecycle-hook subcommands (invoked by the Claude Code plugin runtime)",
		Commands: []*cli.Command{
			{
				Name:   "session-start",
				Usage:  "Fired once when a CC session begins; emits session context as additionalContext",
				Action: hooks.SessionStart,
			},
			{
				Name:   "user-prompt-submit",
				Usage:  "Fired before each agent turn; fetches, drains retry queue, emits digest",
				Action: hooks.UserPromptSubmit,
			},
			{
				Name:   "pre-tool-use",
				Usage:  "Gates Bash tool invocations; denies git push and git config remote.*",
				Action: hooks.PreToolUse,
			},
			{
				Name:   "post-tool-use",
				Usage:  "Fires after each Bash call; pushes git commits with retry",
				Action: hooks.PostToolUse,
			},
			{
				Name:   "stop",
				Usage:  "Fires when the agent yields to the human; auto-commits and pushes",
				Action: hooks.Stop,
			},
			{
				Name:   "session-end",
				Usage:  "Fired when a CC session ends; v1 no-op",
				Action: hooks.SessionEnd,
			},
		},
	}
}
