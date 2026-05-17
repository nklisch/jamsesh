// Package mcpheaders implements the "mcp-headers" subcommand, which outputs
// the Authorization header JSON for Claude Code's MCP client to consume at
// connection time.
package mcpheaders

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/state"
)

// Command returns the urfave/cli command descriptor for "mcp-headers".
func Command() *cli.Command {
	return &cli.Command{
		Name:  "mcp-headers",
		Usage: "Output the Authorization header JSON for CC's MCP client",
		Action: func(ctx context.Context, _ *cli.Command) error {
			tok, err := state.ReadToken()
			if err != nil {
				fmt.Fprintln(os.Stderr, "no token found; run `jamsesh auth` first")
				os.Exit(2)
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]string{
				"Authorization": "Bearer " + tok,
			})
		},
	}
}
