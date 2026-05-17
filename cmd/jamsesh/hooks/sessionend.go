package hooks

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/hookio"
)

// sessionEndInput mirrors the CC SessionEnd hook input schema.
type sessionEndInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
}

// sessionEndOutput is the CC SessionEnd hook output.
// v1: empty — the hook is a no-op. The field set may grow in future stories
// to post a presence-offline event to the portal.
type sessionEndOutput struct{}

// handleSessionEnd is the v1 no-op implementation of the session-end hook.
// It receives the CC SessionEnd payload and returns an empty response.
//
// v1 contract: does nothing. Future iterations will optionally POST a
// presence-offline event to the portal when credentials are available.
// Local session state is NOT deleted — it survives across CC sessions so
// that retry queues and cursors are available if the session resumes.
func handleSessionEnd(_ context.Context, _ sessionEndInput) (sessionEndOutput, error) {
	return sessionEndOutput{}, nil
}

// SessionEnd is the urfave/cli action for "jamsesh hook session-end".
// v1 no-op: reads the CC SessionEnd JSON from stdin, returns an empty JSON
// object to stdout.
func SessionEnd(ctx context.Context, _ *cli.Command) error {
	if err := hookio.Run(ctx, stdinOf(ctx), stdoutOf(ctx), handleSessionEnd); err != nil {
		return fmt.Errorf("session-end: %w", err)
	}
	return nil
}
