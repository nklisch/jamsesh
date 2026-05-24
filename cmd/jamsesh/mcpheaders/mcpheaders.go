// Package mcpheaders implements the "mcp-headers" subcommand, which outputs
// the Authorization header JSON for Claude Code's MCP client to consume at
// connection time.
package mcpheaders

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/state"
)

// Command returns the urfave/cli command descriptor for "mcp-headers".
func Command() *cli.Command {
	return &cli.Command{
		Name:  "mcp-headers",
		Usage: "Output the Authorization header JSON for CC's MCP client",
		Action: func(ctx context.Context, _ *cli.Command) error {
			headers := map[string]string{}

			// Include Jam-Session-Id when this CC instance has a bound session.
			// Absent binding is safe: single-instance portal ignores the header;
			// clustered-mode router falls back to round-robin for unrouted MCP calls.
			if sessID, ok := state.CurrentSessionID(); ok {
				headers["Jam-Session-Id"] = sessID

				// Per-session bearer storage (Story 2 unified storage model):
				// when a session is bound, read the per-session token rather than
				// the legacy account-wide file (which may be the migration stub).
				if tok, err := state.ReadSessionToken(sessID); err == nil {
					headers["Authorization"] = "Bearer " + strings.TrimSpace(string(tok))
					return json.NewEncoder(os.Stdout).Encode(headers)
				}
				// Per-session token missing: fall through to legacy token below.
			}

			// Fallback: legacy account-wide token (used before per-session storage
			// was introduced, and for mcp connections without a bound session).
			// Pre-binding: pass "" so ReadCurrentBearer uses the legacy path.
			tok, err := state.ReadCurrentBearer("")
			if err != nil {
				fmt.Fprintln(os.Stderr, "no token found; run `jamsesh auth` first")
				os.Exit(2)
			}
			headers["Authorization"] = "Bearer " + tok

			return json.NewEncoder(os.Stdout).Encode(headers)
		},
	}
}
