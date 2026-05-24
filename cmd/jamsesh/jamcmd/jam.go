// Package jamcmd provides the "jam" parent subcommand, which is the
// intent-driven entry point for the /jamsesh:jam skill. It dispatches to the
// existing sessioncmd.NewCommand and sessioncmd.JoinCommand actions via
// urfave/cli v3's nested-commands pattern.
//
// NOTE: JamCommand calls sessioncmd.NewCommand() and sessioncmd.JoinCommand()
// to obtain fresh *cli.Command instances. This is intentional — urfave/cli v3
// sets a parent pointer on each command during setup, so the same instance
// cannot safely appear under two different parents. Calling the constructors
// again produces semantically identical commands (same Name, Flags, Action)
// without sharing state.
package jamcmd

import (
	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/sessioncmd"
)

// JamCommand returns the urfave/cli command descriptor for "jamsesh jam".
// It acts as an intent-driven entry point: the sub-subcommands it exposes
// ("new" and "join") mirror the top-level jamsesh subcommands of the same
// name, allowing agents to use `jamsesh jam new` or `jamsesh jam join`
// interchangeably with `jamsesh new` / `jamsesh join`.
func JamCommand() *cli.Command {
	return &cli.Command{
		Name:  "jam",
		Usage: "Create, join, or manage a jam session (intent-driven entry)",
		Commands: []*cli.Command{
			// Fresh instances — same Name/Flags/Action as the top-level
			// registrations but separate *cli.Command values so urfave/cli
			// can assign each one its own parent pointer during setup.
			sessioncmd.NewCommand(),
			sessioncmd.JoinCommand(),
		},
	}
}
