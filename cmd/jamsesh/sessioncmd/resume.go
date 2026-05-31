package sessioncmd

import (
	"context"
	"fmt"

	"jamsesh/cmd/jamsesh/internal/osopen"
	"jamsesh/cmd/jamsesh/portalclient"
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
