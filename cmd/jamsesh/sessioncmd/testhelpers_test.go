package sessioncmd

import (
	"context"

	"github.com/urfave/cli/v3"
)

// buildCLIApp returns a minimal CLI app containing all session subcommands,
// suitable for driving in unit tests.
func buildCLIApp() *cli.Command {
	return &cli.Command{
		Name: "jamsesh",
		Commands: []*cli.Command{
			JoinCommand(),
			NewCommand(),
			InviteCommand(),
			StatusCommand(),
			ForkCommand(),
			ModeCommand(),
		},
		// Suppress default error printing so tests can assert on returned errors.
		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {},
	}
}
