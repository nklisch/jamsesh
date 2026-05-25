// Package sessioncmd implements the jamsesh session subcommands: join and status.
package sessioncmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/oklog/ulid/v2"
	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/cmd/jamsesh/state"
	"jamsesh/internal/api/openapi"
)

// runGit is the function used to run git subcommands. Override in tests.
var runGit = func(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runGitOutput is like runGit but captures stdout instead of inheriting it.
var runGitOutput = func(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// JoinCommand returns the urfave/cli command descriptor for "jamsesh join".
func JoinCommand() *cli.Command {
	return &cli.Command{
		Name:      "join",
		Usage:     "Join a jamsesh session",
		ArgsUsage: "<session-id-or-url>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "as",
				Usage: "Branch name for your working ref (default: main)",
			},
			&cli.StringFlag{
				Name:  "from",
				Usage: "Commit SHA to create your branch from (optional)",
			},
		},
		Action: joinAction,
	}
}

func joinAction(ctx context.Context, cmd *cli.Command) error {
	arg := cmd.Args().First()
	if arg == "" {
		return fmt.Errorf("usage: jamsesh join <session-id-or-url>")
	}

	branch := cmd.String("as")
	if branch == "" {
		branch = "main"
	}
	fromRef := cmd.String("from")

	// 1. Auth check. Pre-binding: no session ID yet; pass "" so ReadCurrentBearer
	// uses the legacy account-wide path.
	tok, err := state.ReadCurrentBearer("")
	if err != nil || tok == "" {
		fmt.Fprintln(os.Stderr, "Not authenticated. Run `jamsesh auth` first.")
		os.Exit(1)
	}

	portalURL, err := state.ReadPortalURL()
	if err != nil {
		return fmt.Errorf("resolving portal URL: %w", err)
	}

	pc := &portalclient.Client{BaseURL: portalURL}
	portalclient.WireRefresh(pc)

	// 2. Parse the arg — may be a bare session ID, orgID/sessionID, or invite URL.
	sessionID, orgID, inviteID, inviteToken := parseSessionArg(arg, portalURL)

	// 3. Get account info (needed for the ref name).
	me, err := portalclient.GetJSON[openapi.MeResponse](ctx, pc, "/api/me")
	if err != nil {
		return fmt.Errorf("fetching account info: %w", err)
	}
	accountID := me.Id

	// 4. If orgID is empty, discover it from the user's orgs.
	if orgID == "" {
		orgID, err = findOrgForSession(ctx, pc, me, sessionID)
		if err != nil {
			return fmt.Errorf("locating session %q: %w", sessionID, err)
		}
	}

	// 5. If invite URL, accept the invite.
	if inviteID != "" {
		_, err = portalclient.PostJSON[map[string]any](
			ctx, pc,
			fmt.Sprintf("/api/orgs/%s/sessions/%s/invites/%s/accept", orgID, sessionID, inviteID),
			openapi.AcceptInviteBody{Token: inviteToken},
		)
		if err != nil {
			return fmt.Errorf("accepting invite: %w", err)
		}
	}

	// 6. Get session metadata.
	session, err := portalclient.GetJSON[openapi.Session](
		ctx, pc,
		fmt.Sprintf("/api/orgs/%s/sessions/%s", orgID, sessionID),
	)
	if err != nil {
		return fmt.Errorf("fetching session metadata: %w", err)
	}

	// 7. Clone the session bare repo.
	cloneURL := buildCloneURL(portalURL, tok, orgID, sessionID)
	localPath := sessionID + ".git"
	if err := runGit("clone", "--bare", cloneURL, localPath); err != nil {
		return fmt.Errorf("cloning session repo: %w", err)
	}

	// 8. Checkout the user's ref inside the bare clone.
	targetRef := fmt.Sprintf("jam/%s/%s/%s", sessionID, accountID, branch)
	checkoutArgs := []string{"-C", localPath, "checkout", "-b", targetRef}
	if fromRef != "" {
		checkoutArgs = append(checkoutArgs, fromRef)
	}
	if err := runGit(checkoutArgs...); err != nil {
		return fmt.Errorf("checking out ref %q: %w", targetRef, err)
	}

	// 9. Write per-session state.
	if err := writeSessionState(sessionID, orgID, targetRef, accountID); err != nil {
		return fmt.Errorf("writing session state: %w", err)
	}

	// 10. Print summary.
	fmt.Printf("Joined session %s (%s)\nYour ref:  %s\nGoal:      %s\nMode:      %s\n",
		session.Name, sessionID, targetRef, session.Goal, session.DefaultMode)
	return nil
}

// parseSessionArg parses the raw CLI argument into (sessionID, orgID, inviteID, inviteToken).
// Supported forms:
//   - bare session UUID: "550e8400-e29b-41d4-a716-446655440000"
//   - orgID/sessionID:   "orgabc/sessabc"
//   - invite URL:        "https://portal/join?org=<orgID>&session=<sid>&invite=<iid>&token=<tok>"
func parseSessionArg(arg, portalURL string) (sessionID, orgID, inviteID, inviteToken string) {
	// Try parsing as a URL.
	u, err := url.Parse(arg)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		q := u.Query()
		sessionID = q.Get("session")
		orgID = q.Get("org")
		inviteID = q.Get("invite")
		inviteToken = q.Get("token")
		// Fallback: path-based session ID (e.g. /join/<sessionID>)
		if sessionID == "" {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			for i, p := range parts {
				if p == "join" && i+1 < len(parts) {
					sessionID = parts[i+1]
					break
				}
			}
		}
		return
	}

	// orgID/sessionID form.
	if idx := strings.Index(arg, "/"); idx > 0 {
		orgID = arg[:idx]
		sessionID = arg[idx+1:]
		return
	}

	// Bare session ID.
	sessionID = arg
	return
}

// findOrgForSession iterates the user's org memberships and returns the first
// orgID that owns the given sessionID. It calls GET /api/orgs/{orgID}/sessions/{sessionID}
// for each org in the user's membership list.
func findOrgForSession(ctx context.Context, pc *portalclient.Client, me openapi.MeResponse, sessionID string) (string, error) {
	for _, m := range me.Orgs {
		_, err := portalclient.GetJSON[openapi.Session](
			ctx, pc,
			fmt.Sprintf("/api/orgs/%s/sessions/%s", m.Id, sessionID),
		)
		if err == nil {
			return m.Id, nil
		}
		// 404 → try next org; any other error → bail.
		if !strings.Contains(err.Error(), "404") {
			return "", err
		}
	}
	return "", fmt.Errorf("session %q not found in any of your orgs", sessionID)
}

// buildCloneURL injects the bearer token into the portal URL as HTTP basic auth
// so git can authenticate without a credential helper.
func buildCloneURL(portalURL, token, orgID, sessionID string) string {
	u, err := url.Parse(portalURL)
	if err != nil {
		// Shouldn't happen; fall back to the raw URL.
		return portalURL + "/git/" + orgID + "/" + sessionID + ".git"
	}
	u.User = url.UserPassword("x-access-token", token)
	u.Path = strings.TrimRight(u.Path, "/") + "/git/" + orgID + "/" + sessionID + ".git"
	return u.String()
}

// writeSessionState persists the per-session state files under
// ${data-dir}/sessions/<sessionID>/.
func writeSessionState(sessionID, orgID, targetRef, accountID string) error {
	dir, err := state.DataDir()
	if err != nil {
		return err
	}
	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		return fmt.Errorf("creating session state dir: %w", err)
	}

	// Use CLAUDE_SESSION_ID if available; otherwise generate a ULID.
	instanceID := os.Getenv("CLAUDE_SESSION_ID")
	if instanceID == "" {
		instanceID = ulid.Make().String()
	}

	writes := []struct {
		name string
		val  string
	}{
		{"ref", targetRef},
		{"instance_id", instanceID},
		{"last_seen_seq", "0"},
		{"account_id", accountID},
		{"org_id", orgID},
	}
	for _, w := range writes {
		p := filepath.Join("sessions", sessionID, w.name)
		if err := state.Write(p, []byte(w.val), 0o600); err != nil {
			return fmt.Errorf("writing session state %q: %w", w.name, err)
		}
	}
	return nil
}
