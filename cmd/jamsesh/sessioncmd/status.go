package sessioncmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/cmd/jamsesh/state"
	"jamsesh/internal/api/openapi"
)

// StatusCommand returns the urfave/cli command descriptor for "jamsesh status".
func StatusCommand() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show current session status",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output status as JSON instead of human-readable text",
			},
		},
		Action: statusAction,
	}
}

// statusOutput is the structured form emitted by --json.
type statusOutput struct {
	SessionID string            `json:"session_id"`
	Name      string            `json:"name"`
	Goal      string            `json:"goal"`
	Mode      string            `json:"mode"`
	YourRef   string            `json:"your_ref"`
	Refs      []openapi.Ref     `json:"refs"`
	Comments  []openapi.Comment `json:"unresolved_comments"`
}

func statusAction(ctx context.Context, cmd *cli.Command) error {
	asJSON := cmd.Bool("json")

	// Resolve the current session ID via the shared helper in session.go.
	sessionID, err := ResolveSession()
	if err != nil {
		return fmt.Errorf("resolving current session: %w", err)
	}

	// Read orgID and your ref from per-session state files.
	orgID, yourRef := readSessionState(sessionID)

	portalURL, err := state.ReadPortalURL()
	if err != nil {
		return fmt.Errorf("resolving portal URL: %w", err)
	}
	pc := &portalclient.Client{BaseURL: portalURL}

	// Fetch me (for account ID used to filter comments).
	me, err := portalclient.GetJSON[openapi.MeResponse](ctx, pc, "/api/me")
	if err != nil {
		return fmt.Errorf("fetching account info: %w", err)
	}

	// If orgID is empty (not stored in state), discover it from user's orgs.
	if orgID == "" {
		orgID, err = findOrgForSession(ctx, pc, me, sessionID)
		if err != nil {
			return fmt.Errorf("locating session %q: %w", sessionID, err)
		}
	}

	// Fetch session metadata.
	session, err := portalclient.GetJSON[openapi.Session](
		ctx, pc,
		fmt.Sprintf("/api/orgs/%s/sessions/%s", orgID, sessionID),
	)
	if err != nil {
		return fmt.Errorf("fetching session metadata: %w", err)
	}

	// Fetch refs.
	refList, err := portalclient.GetJSON[openapi.RefListResponse](
		ctx, pc,
		fmt.Sprintf("/api/orgs/%s/sessions/%s/refs", orgID, sessionID),
	)
	if err != nil {
		return fmt.Errorf("fetching session refs: %w", err)
	}

	// Fetch unresolved comments addressed to this user.
	addressedTo := "@" + me.Id
	commentList, err := portalclient.GetJSON[openapi.CommentListResponse](
		ctx, pc,
		fmt.Sprintf("/api/orgs/%s/sessions/%s/comments?addressed_to=%s&resolved=false",
			orgID, sessionID, addressedTo),
	)
	if err != nil {
		return fmt.Errorf("fetching comments: %w", err)
	}

	out := statusOutput{
		SessionID: sessionID,
		Name:      session.Name,
		Goal:      session.Goal,
		Mode:      string(session.DefaultMode),
		YourRef:   yourRef,
		Refs:      refList.Refs,
		Comments:  commentList.Items,
	}

	if asJSON {
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	// Human-readable output.
	fmt.Printf("Session:  %s (%s)\n", out.Name, out.SessionID)
	fmt.Printf("Goal:     %s\n", out.Goal)
	fmt.Printf("Mode:     %s\n", out.Mode)
	fmt.Printf("Your ref: %s\n", out.YourRef)
	fmt.Println("Tree:")
	for _, r := range out.Refs {
		sha := r.Sha
		if len(sha) > 7 {
			sha = sha[:7]
		}
		fmt.Printf("  %s @ %s [%s]\n", r.Ref, sha, r.Mode)
	}
	if len(out.Comments) > 0 {
		fmt.Printf("Unresolved comments addressed to you (%d):\n", len(out.Comments))
		for _, c := range out.Comments {
			fmt.Printf("  [%s] %s: %s\n", c.Kind, c.Id, truncate(c.Body, 80))
		}
	} else {
		fmt.Println("No unresolved comments addressed to you.")
	}
	return nil
}

// readSessionState reads orgID and your ref from the per-session state directory.
// Missing files silently return empty strings; callers must handle empty orgID.
func readSessionState(sessionID string) (orgID, yourRef string) {
	dir, err := state.PluginDataDir()
	if err != nil {
		return "", ""
	}
	sessDir := filepath.Join(dir, "sessions", sessionID)

	if data, err := os.ReadFile(filepath.Join(sessDir, "org_id")); err == nil {
		orgID = strings.TrimSpace(string(data))
	}
	if data, err := os.ReadFile(filepath.Join(sessDir, "ref")); err == nil {
		yourRef = strings.TrimSpace(string(data))
	}
	return orgID, yourRef
}

// truncate shortens s to at most n runes, appending "…" if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
