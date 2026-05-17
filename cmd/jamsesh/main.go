package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/auth"
	"jamsesh/cmd/jamsesh/hooks"
	"jamsesh/cmd/jamsesh/mcpheaders"
	"jamsesh/cmd/jamsesh/sessioncmd"
	"jamsesh/internal/buildinfo"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	app := &cli.Command{
		Name:    "jamsesh",
		Usage:   "Local client for the jamsesh portal",
		Version: buildinfo.Version,
		Commands: []*cli.Command{
			auth.Command(),
			mcpheaders.Command(),
			hookCommand(),
			sessioncmd.JoinCommand(),
			sessioncmd.StatusCommand(),
			sessioncmd.ForkCommand(),
			sessioncmd.ModeCommand(),
		},
	}

	if err := app.Run(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// hookCommand returns the "hook" parent command with all six CC lifecycle-hook
// subcommands registered. session-start, user-prompt-submit, post-tool-use,
// and stop are stubs until the sibling story
// (epic-cc-plugin-hooks-fetch-push-and-stop-hooks) lands.
func hookCommand() *cli.Command {
	return &cli.Command{
		Name:  "hook",
		Usage: "CC lifecycle-hook subcommands (invoked by the Claude Code plugin runtime)",
		Commands: []*cli.Command{
			{
				Name:   "session-start",
				Usage:  "Fired once when a CC session begins (stub)",
				Action: hooks.SessionStart,
			},
			{
				Name:   "user-prompt-submit",
				Usage:  "Fired before each agent turn (stub)",
				Action: hooks.UserPromptSubmit,
			},
			{
				Name:   "pre-tool-use",
				Usage:  "Gates Bash tool invocations; denies git push and git config remote.*",
				Action: hooks.PreToolUse,
			},
			{
				Name:   "post-tool-use",
				Usage:  "Fires after each Bash call; pushes git commits (stub)",
				Action: hooks.PostToolUse,
			},
			{
				Name:   "stop",
				Usage:  "Fires when the agent yields to the human (stub)",
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
