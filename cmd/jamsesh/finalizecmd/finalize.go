package finalizecmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/cmd/jamsesh/sessioncmd"
	"jamsesh/cmd/jamsesh/state"
)

// FinalizeCommand returns the urfave/cli descriptor for `jamsesh finalize`.
// Default behaviour opens the portal's curation view in the user's
// browser. `--local` fetches the plan headlessly and prints it to
// stdout (for users who curated from another device or want the script
// without leaving the terminal).
func FinalizeCommand() *cli.Command {
	return &cli.Command{
		Name:  "finalize",
		Usage: "Open the portal's finalize view (default) or fetch the plan locally (--local)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "local",
				Usage: "Fetch and print the plan to stdout instead of opening a browser",
			},
		},
		Action: finalizeAction,
	}
}

func finalizeAction(ctx context.Context, cmd *cli.Command) error {
	sessionID, err := sessioncmd.ResolveSession()
	if err != nil {
		return err
	}

	portalURL, err := state.ReadPortalURL()
	if err != nil {
		return fmt.Errorf("resolving portal URL: %w", err)
	}

	if cmd.Bool("local") {
		return finalizeLocal(ctx, portalURL, sessionID)
	}
	return finalizeBrowser(portalURL, sessionID)
}

// finalizeBrowser opens the curation view in the user's browser and
// always prints the URL to stdout so headless users (and tests) have a
// copyable string even when the open call succeeds.
func finalizeBrowser(portalURL, sessionID string) error {
	url := strings.TrimRight(portalURL, "/") + "/sessions/" + sessionID + "/finalize"
	fmt.Fprintf(os.Stdout, "Opening finalize view: %s\n", url)
	// openURL is a package var that defaults to platform xdg-open / open
	// / rundll32. Failures degrade to a printed URL inside openURL
	// itself, so we just surface unexpected errors.
	if err := openURL(url); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}
	return nil
}

// finalizeLocal calls GET /api/orgs/<org>/sessions/<sid>/finalize-plan
// without a lock_id and prints the summary + script body to stdout.
// The portal returns 409 if no lock is currently held; we surface that
// as a friendly hint pointing at the portal.
func finalizeLocal(ctx context.Context, portalURL, sessionID string) error {
	orgID, err := readOrgIDForSession(sessionID)
	if err != nil {
		return err
	}
	pc := &portalclient.Client{BaseURL: portalURL}
	plan, err := fetchPlan(ctx, pc, orgID, sessionID, "")
	if err != nil {
		if strings.Contains(err.Error(), "409") {
			return fmt.Errorf("no finalize lock is currently held; open the portal first to start a finalize session")
		}
		return fmt.Errorf("fetching plan: %w", err)
	}
	printPlanSummary(os.Stdout, plan)
	fmt.Fprintf(os.Stdout, "\n# Script preview (not executed):\n\n")
	return printScript(os.Stdout, plan)
}

// readOrgIDForSession reads the org_id sidecar file written by
// `jamsesh join`. Returns an error if the file is missing — this
// state is required for any portal call that is session-scoped.
func readOrgIDForSession(sessionID string) (string, error) {
	dir, err := state.PluginDataDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(dir + "/sessions/" + sessionID + "/org_id")
	if err != nil {
		return "", fmt.Errorf("reading org_id for session %s: %w (run `jamsesh join` first)", sessionID, err)
	}
	return strings.TrimSpace(string(data)), nil
}
