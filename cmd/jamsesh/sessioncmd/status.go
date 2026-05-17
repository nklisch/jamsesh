package sessioncmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
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
	SessionID string           `json:"session_id"`
	Name      string           `json:"name"`
	Goal      string           `json:"goal"`
	Mode      string           `json:"mode"`
	YourRef   string           `json:"your_ref"`
	Refs      []openapi.Ref    `json:"refs"`
	Comments  []openapi.Comment `json:"unresolved_comments"`
}

func statusAction(ctx context.Context, cmd *cli.Command) error {
	asJSON := cmd.Bool("json")

	// Resolve current session.
	sessionID, orgID, yourRef, err := resolveCurrentSession()
	if err != nil {
		return fmt.Errorf("resolving current session: %w", err)
	}

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

	// If orgID is empty, discover it.
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

// resolveCurrentSession finds the active session ID, orgID, and your current
// ref by consulting JAMSESH_SESSION_ID env or scanning the sessions/ state dir.
// It returns ("", "", "", err) if no session is found.
func resolveCurrentSession() (sessionID, orgID, yourRef string, err error) {
	// 1. Check explicit env override.
	if sid := os.Getenv("JAMSESH_SESSION_ID"); sid != "" {
		sessionID = sid
	}

	// 2. Match by CLAUDE_SESSION_ID instance_id stored in state.
	instanceID := os.Getenv("CLAUDE_SESSION_ID")

	dir, err := state.PluginDataDir()
	if err != nil {
		if sessionID != "" {
			// We have a session ID; proceed without orgID — caller will discover it.
			return sessionID, "", "", nil
		}
		return "", "", "", err
	}

	sessionsDir := filepath.Join(dir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if sessionID != "" {
			return sessionID, "", "", nil
		}
		if err2, ok := err.(*fs.PathError); ok && strings.Contains(err2.Error(), "no such file") {
			return "", "", "", fmt.Errorf("no sessions found; run `jamsesh join` first")
		}
		return "", "", "", fmt.Errorf("reading sessions dir: %w", err)
	}

	// If CLAUDE_SESSION_ID is set, look for a matching instance_id.
	if instanceID != "" && sessionID == "" {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			iidPath := filepath.Join(sessionsDir, e.Name(), "instance_id")
			data, rerr := os.ReadFile(iidPath)
			if rerr != nil {
				continue
			}
			if strings.TrimSpace(string(data)) == instanceID {
				sessionID = e.Name()
				break
			}
		}
	}

	// If still no session ID, fall back to the most recently modified session dir.
	if sessionID == "" {
		var latestEntry os.DirEntry
		var latestTime int64
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			info, rerr := e.Info()
			if rerr != nil {
				continue
			}
			if info.ModTime().Unix() > latestTime {
				latestTime = info.ModTime().Unix()
				latestEntry = e
			}
		}
		if latestEntry == nil {
			return "", "", "", fmt.Errorf("no sessions found; run `jamsesh join` first")
		}
		sessionID = latestEntry.Name()
	}

	// Read ref and orgID from state files.
	refData, _ := os.ReadFile(filepath.Join(sessionsDir, sessionID, "ref"))
	yourRef = strings.TrimSpace(string(refData))

	orgData, _ := os.ReadFile(filepath.Join(sessionsDir, sessionID, "org_id"))
	orgID = strings.TrimSpace(string(orgData))

	return sessionID, orgID, yourRef, nil
}

// truncate shortens s to at most n runes, appending "…" if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
