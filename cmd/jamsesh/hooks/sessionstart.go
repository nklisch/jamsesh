package hooks

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/hookio"
	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/cmd/jamsesh/state"
	"jamsesh/internal/api/openapi"
)

// sessionStartInput mirrors the CC SessionStart hook input schema.
type sessionStartInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
}

// sessionStartOutput is the CC SessionStart hook output.
type sessionStartOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// sessionState holds all per-session state needed by hook handlers.
type sessionState struct {
	SessionID string
	OrgID     string
	Ref       string
	AccountID string
}

// resolveHookSession maps the current CC instance to a jamsesh session and
// reads per-session state files. Returns (nil, nil) when no session is bound
// so that callers can return an empty hook response without treating it as an
// error.
func resolveHookSession() (*sessionState, error) {
	sid, err := resolveSessionID()
	if err != nil {
		// No session mapped to this CC instance — not an error, just no-op.
		return nil, nil //nolint:nilerr
	}

	readSess := func(name string) (string, error) {
		data, e := state.Read(filepath.Join("sessions", sid, name))
		if e != nil {
			return "", fmt.Errorf("hooks: reading session state %q: %w", name, e)
		}
		return strings.TrimSpace(string(data)), nil
	}

	orgID, err := readSess("org_id")
	if err != nil {
		return nil, err
	}
	ref, err := readSess("ref")
	if err != nil {
		return nil, err
	}
	accountID, err := readSess("account_id")
	if err != nil {
		return nil, err
	}
	return &sessionState{
		SessionID: sid,
		OrgID:     orgID,
		Ref:       ref,
		AccountID: accountID,
	}, nil
}

// resolveSessionID returns the jamsesh session ID for the current CC instance.
// This mirrors sessioncmd.resolveSession, kept local to avoid import cycles.
func resolveSessionID() (string, error) {
	dir, err := state.DataDir()
	if err != nil {
		return "", err
	}

	ccID := os.Getenv("CC_SESSION_ID")
	sessionsDir := filepath.Join(dir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("no sessions found; run `jamsesh join` first")
		}
		return "", fmt.Errorf("reading sessions directory: %w", err)
	}

	if ccID != "" {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			instanceFile := filepath.Join(sessionsDir, e.Name(), "instance_id")
			data, err := os.ReadFile(instanceFile)
			if err != nil {
				continue
			}
			if strings.TrimSpace(string(data)) == ccID {
				return e.Name(), nil
			}
		}
		return "", fmt.Errorf("no session mapped to CC instance %q; run `jamsesh join` first", ccID)
	}

	// Fallback: use first directory entry (single-session dev path).
	for _, e := range entries {
		if e.IsDir() {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("no sessions found; run `jamsesh join` first")
}

// buildPortalClient constructs a portalclient.Client from local state with
// the token-refresh path wired in so that 401 responses trigger a singleflight
// refresh before the request is retried.
func buildPortalClient() (*portalclient.Client, error) {
	portalURL, err := state.ReadPortalURL()
	if err != nil {
		return nil, fmt.Errorf("resolving portal URL: %w", err)
	}
	pc := &portalclient.Client{BaseURL: portalURL}
	portalclient.WireRefresh(pc)
	return pc, nil
}

// handleSessionStart implements the session-start hook logic.
func handleSessionStart(ctx context.Context, _ sessionStartInput) (sessionStartOutput, error) {
	ss, err := resolveHookSession()
	if err != nil {
		return sessionStartOutput{}, err
	}
	if ss == nil {
		return sessionStartOutput{}, nil
	}

	pc, err := buildPortalClient()
	if err != nil {
		return sessionStartOutput{}, err
	}

	// 1. Fetch account info.
	me, err := portalclient.GetJSON[openapi.MeResponse](ctx, pc, "/api/me")
	if err != nil {
		return sessionStartOutput{}, fmt.Errorf("session-start: fetching account info: %w", err)
	}

	// 2. Fetch session metadata.
	session, err := portalclient.GetJSON[openapi.Session](
		ctx, pc,
		fmt.Sprintf("/api/orgs/%s/sessions/%s", ss.OrgID, ss.SessionID),
	)
	if err != nil {
		return sessionStartOutput{}, fmt.Errorf("session-start: fetching session: %w", err)
	}

	// 3. Fetch refs.
	refsResp, err := portalclient.GetJSON[openapi.RefListResponse](
		ctx, pc,
		fmt.Sprintf("/api/orgs/%s/sessions/%s/refs", ss.OrgID, ss.SessionID),
	)
	if err != nil {
		return sessionStartOutput{}, fmt.Errorf("session-start: fetching refs: %w", err)
	}

	// 4. Fetch unresolved comments addressed to this user.
	commentsPath := fmt.Sprintf(
		"/api/orgs/%s/sessions/%s/comments?addressed_to=@%s&resolved=false",
		ss.OrgID, ss.SessionID, me.Id,
	)
	commentsResp, err := portalclient.GetJSON[openapi.CommentListResponse](ctx, pc, commentsPath)
	if err != nil {
		return sessionStartOutput{}, fmt.Errorf("session-start: fetching comments: %w", err)
	}

	// 5. Format additionalContext.
	var sb strings.Builder

	fmt.Fprintf(&sb, "## jamsesh session: %s\n", session.Name)
	fmt.Fprintf(&sb, "Goal: %s\n", session.Goal)
	if session.Scope != "" {
		fmt.Fprintf(&sb, "Scope: %s\n", session.Scope)
	}

	sb.WriteString("\n## Your refs\n")
	myCount := 0
	for _, r := range refsResp.Refs {
		if !strings.Contains(r.Ref, "/"+me.Id+"/") {
			continue
		}
		sha := r.Sha
		if len(sha) > 12 {
			sha = sha[:12]
		}
		fmt.Fprintf(&sb, "  %s @ %s [%s]\n", r.Ref, sha, r.Mode)
		myCount++
	}
	if myCount == 0 {
		fmt.Fprintf(&sb, "  %s (no tip yet)\n", ss.Ref)
	}

	sb.WriteString("\n## Peer activity\n")
	peerCount := 0
	for _, r := range refsResp.Refs {
		if strings.Contains(r.Ref, "/"+me.Id+"/") {
			continue
		}
		sha := r.Sha
		if len(sha) > 12 {
			sha = sha[:12]
		}
		fmt.Fprintf(&sb, "  %s @ %s [%s]\n", r.Ref, sha, r.Mode)
		peerCount++
	}
	if peerCount == 0 {
		sb.WriteString("  (none)\n")
	}

	sb.WriteString("\n## Unresolved comments addressed to you\n")
	if len(commentsResp.Items) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, c := range commentsResp.Items {
			fmt.Fprintf(&sb, "  [%s] %s: %s\n", c.Kind, c.AuthorId, c.Body)
		}
	}

	return sessionStartOutput{AdditionalContext: sb.String()}, nil
}

// SessionStart is the urfave/cli action for "jamsesh hook session-start".
func SessionStart(ctx context.Context, _ *cli.Command) error {
	if err := hookio.Run(ctx, stdinOf(ctx), stdoutOf(ctx), handleSessionStart); err != nil {
		return fmt.Errorf("session-start: %w", err)
	}
	return nil
}
