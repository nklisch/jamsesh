package sessioncmd

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/internal/osopen"
	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/cmd/jamsesh/state"
	"jamsesh/internal/api/openapi"
)

// openSilent is the token-safe browser-open seam used by mintAndOpenResume.
// Tests override it to capture the URL without actually launching a browser.
// It must NEVER print the URL on any code path.
var openSilent = osopen.OpenSilent

// mintAndOpenResume mints a single-use resume token via the portal and opens
// the returned resume_url in the browser using the token-safe OpenSilent seam.
//
// pc MUST be constructed with SessionID set to the target session so that the
// per-session bearer (anonymous for playground, OAuth for durable) is used to
// authenticate the mint request. Do NOT call buildPortalClient() for the
// playground path.
//
// On success the resume URL is opened silently (never printed). On error the
// function returns the error and opens nothing. The message printed to stdout
// is intentionally token-free.
func mintAndOpenResume(ctx context.Context, pc *portalclient.Client, orgID, sessionID string) error {
	resp, err := portalclient.PostJSON[openapi.SessionResumeResponse](
		ctx, pc, "/api/session-resumes",
		openapi.SessionResumeRequest{OrgId: orgID, SessionId: sessionID},
	)
	if err != nil {
		return fmt.Errorf("minting resume token: %w", err)
	}

	// Validate the response before opening anything: a mismatched session_id or
	// empty resume_url means we cannot safely proceed.
	if resp.SessionId != sessionID {
		return fmt.Errorf("resume response session_id mismatch: got %q, want %q", resp.SessionId, sessionID)
	}
	if resp.ResumeUrl == "" {
		return fmt.Errorf("resume response contained an empty resume_url")
	}

	// Token-free user message — the URL (which carries #rt=<token>) is
	// never printed to stdout or stderr anywhere in this function.
	fmt.Println("Opening your session in the browser (resume link expires in 60s)…")

	// Open the portal-returned URL verbatim. OpenSilent never prints the URL.
	return openSilent(resp.ResumeUrl)
}

// ResumeCommand returns the urfave/cli command descriptor for "jamsesh resume".
func ResumeCommand() *cli.Command {
	return &cli.Command{
		Name:      "resume",
		Usage:     "Open a jamsesh session in the browser as your CLI identity",
		ArgsUsage: "[session-id]",
		Action:    resumeAction,
	}
}

// resumeAction implements the session-id resolution and mint+open logic for
// "jamsesh resume [session-id]".
func resumeAction(ctx context.Context, cmd *cli.Command) error {
	portalURL, err := state.ReadPortalURL()
	if err != nil {
		return fmt.Errorf("resolving portal URL: %w", err)
	}

	sessionID, err := resolveResumeSession(cmd)
	if err != nil {
		return err
	}

	orgID, _ := readSessionState(sessionID)
	if orgID == "" {
		return fmt.Errorf("no org_id found for session %q; run `jamsesh status` to inspect sessions", sessionID)
	}

	// Playground sessions use an anonymous per-session bearer — they never have
	// a durable OAuth account token. Guard explicitly: if the per-session bearer
	// is missing, fail with a clear local error rather than letting portalclient
	// silently fall back to the legacy account token, which would be the wrong
	// credential class and could attempt a mint with a durable OAuth token.
	if orgID == playgroundOrgID {
		if _, err := state.ReadSessionToken(sessionID); err != nil {
			return fmt.Errorf(
				"no playground credential for session %q; run `jamsesh status` to inspect sessions",
				sessionID,
			)
		}
		// Build a per-session client with NO refresh wiring — anon playground
		// bearers are non-refreshable.
		pc := &portalclient.Client{
			BaseURL:   portalURL,
			SessionID: sessionID,
		}
		return mintAndOpenResume(ctx, pc, orgID, sessionID)
	}

	pc := &portalclient.Client{
		BaseURL:   portalURL,
		SessionID: sessionID,
	}
	portalclient.WireRefresh(pc)

	return mintAndOpenResume(ctx, pc, orgID, sessionID)
}

// resolveResumeSession determines which session to resume.
//
// Resolution order:
//  1. Explicit <session-id> argument → use it directly.
//  2. Bare (no arg): check state.CurrentSessionID() — uses CLAUDE_SESSION_ID-based
//     binding (write-consistent, NOT ResolveSession — see backlog
//     cli-resolvesession-env-var-mismatch). If mapped, return it.
//  3. If CLAUDE_SESSION_ID is set but unmapped → error with jamsesh status hint.
//  4. Outside CC context (CLAUDE_SESSION_ID unset): enumerate sessions.
//     - Exactly one session → resume it (unambiguous).
//     - Zero or multiple sessions → error with jamsesh status hint.
func resolveResumeSession(cmd *cli.Command) (string, error) {
	// 1. Explicit session-id argument.
	if cmd.NArg() > 0 {
		return cmd.Args().First(), nil
	}

	// 2. Try the write-consistent CLAUDE_SESSION_ID-based resolver.
	if sessionID, ok := state.CurrentSessionID(); ok {
		return sessionID, nil
	}

	// 3. If CLAUDE_SESSION_ID is present but unmapped, require explicit id.
	if os.Getenv("CLAUDE_SESSION_ID") != "" {
		return "", fmt.Errorf(
			"this Claude Code instance is not mapped to any jamsesh session\n" +
				"hint: run `jamsesh status` to see sessions and then use `jamsesh resume <session-id>`",
		)
	}

	// 4. Outside CC context: enumerate sessions and resume if exactly one.
	sessions, err := state.ListSessions()
	if err != nil {
		return "", fmt.Errorf("listing sessions: %w", err)
	}
	switch len(sessions) {
	case 0:
		return "", fmt.Errorf(
			"no sessions found\n" +
				"hint: run `jamsesh status` to see sessions and then use `jamsesh resume <session-id>`",
		)
	case 1:
		return sessions[0], nil
	default:
		return "", fmt.Errorf(
			"multiple sessions found — specify which one to resume\n"+
				"hint: run `jamsesh status` to see sessions and then use `jamsesh resume <session-id>`",
		)
	}
}
