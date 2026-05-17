package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/hookio"
	"jamsesh/cmd/jamsesh/pusherr"
	"jamsesh/cmd/jamsesh/retryqueue"
)

// postToolUseInput mirrors the CC PostToolUse hook input schema.
type postToolUseInput struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   toolResponse    `json:"tool_response"`
}

// toolResponse is the subset of the CC PostToolUse tool response we need.
type toolResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// postToolUseOutput is the CC PostToolUse hook output.
type postToolUseOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// handlePostToolUse implements the post-tool-use hook logic.
// It only acts when tool_name == "Bash" AND the command starts with "git commit"
// AND exit_code == 0.
func handlePostToolUse(_ context.Context, in postToolUseInput) (postToolUseOutput, error) {
	// Filter: only care about successful Bash git commits.
	if in.ToolName != "Bash" {
		return postToolUseOutput{}, nil
	}

	var bash bashToolInput
	if len(in.ToolInput) > 0 {
		if err := json.Unmarshal(in.ToolInput, &bash); err != nil {
			return postToolUseOutput{}, nil
		}
	}
	cmd := strings.TrimLeft(bash.Command, " \t")
	if !strings.HasPrefix(cmd, "git commit") {
		return postToolUseOutput{}, nil
	}
	if in.ToolResponse.ExitCode != 0 {
		return postToolUseOutput{}, nil
	}

	// Resolve session.
	ss, err := resolveHookSession()
	if err != nil {
		return postToolUseOutput{}, err
	}
	if ss == nil {
		return postToolUseOutput{}, nil
	}

	// Get current ref (branch name).
	refOut, _, refCode := HookRunGit("rev-parse", "--abbrev-ref", "HEAD")
	if refCode != 0 {
		refOut = ss.Ref // fall back to stored ref
	}
	ref := strings.TrimSpace(refOut)

	// Attempt push with retry policy (up to 3 times).
	result := pushCommitWithRetry(ref, 3)

	switch result.Class {
	case pusherr.OK:
		// Success — agent doesn't need to know.
		return postToolUseOutput{}, nil

	case pusherr.Transient:
		// All retries exhausted — enqueue the commit for drain on next user-prompt-submit.
		sha, _, _ := HookRunGit("rev-parse", "HEAD")
		q := &retryqueue.Queue{SessionID: ss.SessionID}
		entry := retryqueue.Entry{
			CommitSHA:   strings.TrimSpace(sha),
			Attempts:    3,
			LastError:   result.Message,
			LastErrorAt: time.Now(),
		}
		if enqErr := q.Enqueue(entry); enqErr != nil {
			// Don't mask the push error; log enqueue failure to stderr.
			fmt.Printf("jamsesh: failed to enqueue retry: %v\n", enqErr)
		}
		return postToolUseOutput{
			AdditionalContext: fmt.Sprintf(
				"## jamsesh push queued for retry\nThe commit could not be pushed (transient error). It has been queued and will be retried at the start of the next turn.\nLast error: %s\n",
				result.Message,
			),
		}, nil

	case pusherr.Permanent:
		// Fail loud — surface full rejection payload so the agent can act.
		var sb strings.Builder
		sb.WriteString("## jamsesh push rejected (permanent error)\n")
		fmt.Fprintf(&sb, "Error code: %s\n", result.Code)
		fmt.Fprintf(&sb, "Message: %s\n", result.Message)
		if len(result.Details) > 0 {
			sb.WriteString("Details:\n")
			for k, v := range result.Details {
				fmt.Fprintf(&sb, "  %s: %v\n", k, v)
			}
		}
		sb.WriteString("\nThe push was rejected permanently. Review the error above and take corrective action (e.g. revert scope violations, add required trailers).\n")
		return postToolUseOutput{AdditionalContext: sb.String()}, nil
	}

	return postToolUseOutput{}, nil
}

// PostToolUse is the urfave/cli action for "jamsesh hook post-tool-use".
func PostToolUse(ctx context.Context, _ *cli.Command) error {
	if err := hookio.Run(ctx, stdinOf(ctx), stdoutOf(ctx), handlePostToolUse); err != nil {
		return fmt.Errorf("post-tool-use: %w", err)
	}
	return nil
}
