package sessioncmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/cmd/jamsesh/state"
)

// InviteCommand returns the urfave/cli command descriptor for "jamsesh invite".
func InviteCommand() *cli.Command {
	return &cli.Command{
		Name:      "invite",
		Usage:     "Invite one or more emails to an existing session",
		ArgsUsage: "<session-id> <email1>[,<email2>,...] [<email3> ...]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "org", Usage: "Override org ID (default: read from session state)"},
		},
		Action: inviteAction,
	}
}

// inviteAction is the urfave/cli action for "jamsesh invite".
func inviteAction(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) < 2 {
		return fmt.Errorf("usage: jamsesh invite <session-id> <emails>")
	}
	sessionID := args[0]
	// Accept both comma-separated and space-separated emails by joining
	// all remaining args with commas before parsing.
	emails := parseInviteEmails(strings.Join(args[1:], ","))
	if len(emails) == 0 {
		return errors.New("no emails to send")
	}

	// Resolve orgID: --org flag wins, then session state file.
	orgID := cmd.String("org")
	if orgID == "" {
		b, err := state.Read("sessions/" + sessionID + "/org_id")
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("no --org flag and no org_id in session state for %s: session not found locally (run `jamsesh new` first or pass --org)", sessionID)
			}
			return fmt.Errorf("no --org flag and no org_id in session state for %s: %w", sessionID, err)
		}
		orgID = strings.TrimSpace(string(b))
	}

	pc, err := buildPortalClient()
	if err != nil {
		return err
	}
	return sendInvitesIfRequested(ctx, pc, orgID, sessionID, emails)
}

// parseInviteEmails splits a comma-separated email string, trimming whitespace
// and discarding empty entries.
func parseInviteEmails(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// sendInvitesIfRequested sends portal invites to the given email addresses.
// It is best-effort: all emails are attempted regardless of individual failures.
// The caller is responsible for deciding whether partial failures are fatal.
func sendInvitesIfRequested(ctx context.Context, pc *portalclient.Client, orgID, sessionID string, emails []string) error {
	var firstErr error
	var failedCount int
	for _, email := range emails {
		path := fmt.Sprintf("/api/orgs/%s/sessions/%s/invites",
			url.PathEscape(orgID), url.PathEscape(sessionID))
		// InviteRequest.Email is openapi_types.Email which is just a string alias.
		body := map[string]string{"email": email}
		_, err := portalclient.PostJSON[map[string]any](ctx, pc, path, body)
		if err != nil {
			failedCount++
			if firstErr == nil {
				firstErr = err
			}
			fmt.Fprintf(os.Stderr, "  invite %s: FAILED — %v\n", email, err)
			continue
		}
		fmt.Fprintf(os.Stdout, "  invite %s: sent\n", email)
	}
	if firstErr != nil {
		return fmt.Errorf("%d of %d invites failed (first error: %w)", failedCount, len(emails), firstErr)
	}
	return nil
}
