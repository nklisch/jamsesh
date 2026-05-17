// Package auth implements the "auth" subcommand for the jamsesh CLI.
// OAuth browser and device-code flows land in the next story
// (epic-cc-plugin-binary-foundation-oauth-browser-and-device).
package auth

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// Command returns the urfave/cli command descriptor for "auth".
func Command() *cli.Command {
	return &cli.Command{
		Name:  "auth",
		Usage: "Authenticate with the jamsesh portal (OAuth)",
		Action: func(_ context.Context, _ *cli.Command) error {
			fmt.Println("auth subcommand lands in the next story")
			return nil
		},
	}
}
