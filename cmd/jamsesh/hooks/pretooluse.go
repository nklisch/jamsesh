// Package hooks implements the CC lifecycle-hook subcommands for jamsesh.
// Each subcommand is invoked by Claude Code at a specific point in the agent
// turn lifecycle and communicates over stdin/stdout using the hookio scaffold.
package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/hookio"
)

// preToolUseInput mirrors the CC PreToolUse hook input schema.
// https://docs.anthropic.com/en/docs/claude-code/hooks
type preToolUseInput struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
}

// bashToolInput is the subset of the Bash tool_input we care about.
type bashToolInput struct {
	Command string `json:"command"`
}

// preToolUseOutput is the CC PreToolUse hook output schema.
type preToolUseOutput struct {
	PermissionDecision string `json:"permissionDecision"`
	Reason             string `json:"reason,omitempty"`
}

// reGitConfigRemote matches "git config remote.<anything>" (any whitespace
// between tokens).
var reGitConfigRemote = regexp.MustCompile(`^git\s+config\s+remote\.`)

// handlePreToolUse is the core logic for the pre-tool-use hook.
// It gates Bash invocations:
//   - "git push" (any form) → deny
//   - "git config remote.*" → deny
//   - everything else → pass (no opinion)
func handlePreToolUse(_ context.Context, in preToolUseInput) (preToolUseOutput, error) {
	if in.ToolName != "Bash" {
		return preToolUseOutput{PermissionDecision: "pass"}, nil
	}

	var bash bashToolInput
	if len(in.ToolInput) > 0 {
		if err := json.Unmarshal(in.ToolInput, &bash); err != nil {
			// If we cannot parse the Bash input, pass rather than block.
			return preToolUseOutput{PermissionDecision: "pass"}, nil
		}
	}

	// Trim leading whitespace before pattern matching (CC may indent).
	cmd := strings.TrimLeft(bash.Command, " \t")

	if strings.HasPrefix(cmd, "git push") {
		return preToolUseOutput{
			PermissionDecision: "deny",
			Reason:             "jamsesh: push is gated; commits push automatically via post-tool-use",
		}, nil
	}
	if reGitConfigRemote.MatchString(cmd) {
		return preToolUseOutput{
			PermissionDecision: "deny",
			Reason:             "jamsesh: git config remote.* is managed by jamsesh; direct modification is not permitted",
		}, nil
	}

	return preToolUseOutput{PermissionDecision: "pass"}, nil
}

// PreToolUse is the urfave/cli action for "jamsesh hook pre-tool-use".
// It reads a JSON PreToolUse payload from stdin and writes a JSON permission
// decision to stdout.
func PreToolUse(ctx context.Context, _ *cli.Command) error {
	if err := hookio.Run(ctx, stdinOf(ctx), stdoutOf(ctx), handlePreToolUse); err != nil {
		return fmt.Errorf("pre-tool-use: %w", err)
	}
	return nil
}
