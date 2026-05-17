package sessioncmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/mcpclient"
	"jamsesh/cmd/jamsesh/state"
)

// forkResult is the expected shape of the MCP fork tool's StructuredContent.
type forkResult struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

// ForkCommand returns the urfave/cli command descriptor for "jamsesh fork".
func ForkCommand() *cli.Command {
	return &cli.Command{
		Name:      "fork",
		Usage:     "Fork from a commit via the portal MCP fork tool",
		ArgsUsage: "<commit-sha>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "as",
				Usage: "Target branch name for the fork",
			},
			&cli.StringFlag{
				Name:  "mode",
				Usage: "Fork mode: sync or isolated (default: server picks)",
			},
		},
		Action: forkAction,
	}
}

func forkAction(ctx context.Context, cmd *cli.Command) error {
	targetSHA := cmd.Args().First()
	if targetSHA == "" {
		return fmt.Errorf("usage: jamsesh fork <commit-sha>")
	}
	branch := cmd.String("as")
	mode := cmd.String("mode")

	if mode != "" && mode != "sync" && mode != "isolated" {
		return fmt.Errorf("--mode must be 'sync' or 'isolated'")
	}

	sessionID, err := resolveSession()
	if err != nil {
		return err
	}

	portalURL, err := state.ReadPortalURL()
	if err != nil {
		return fmt.Errorf("resolving portal URL: %w", err)
	}
	token, err := state.ReadToken()
	if err != nil {
		return fmt.Errorf("reading token: %w; run `jamsesh auth` first", err)
	}

	client := &mcpclient.Client{
		PortalURL: portalURL,
		Token:     token,
		HTTP:      http.DefaultClient,
	}

	args := map[string]any{
		"session_id":        sessionID,
		"target_commit_sha": targetSHA,
	}
	if branch != "" {
		args["target_ref"] = branch
	}
	if mode != "" {
		args["mode"] = mode
	}

	raw, err := client.CallTool(ctx, "fork", args)
	if err != nil {
		return fmt.Errorf("fork MCP call failed: %w", err)
	}

	var fr forkResult
	if err := json.Unmarshal(raw, &fr); err != nil {
		return fmt.Errorf("parsing fork result: %w", err)
	}

	// Pull the new ref into the local repo.
	if fr.Ref != "" {
		if err := runGit("fetch", "session-remote", fr.Ref); err != nil {
			// Non-fatal: the fork was created server-side; local fetch failure
			// is recoverable. Print a warning but don't fail the command.
			fmt.Printf("Warning: fork created but local fetch failed: %v\n", err)
			fmt.Printf("Re-run: git fetch session-remote %s\n", fr.Ref)
		}
	}

	fmt.Printf("Forked: %s -> %s\n", fr.Ref, fr.SHA)
	return nil
}
