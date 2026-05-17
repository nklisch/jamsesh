package ccdriver

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
)

// Driver simulates the Claude Code plugin lifecycle by invoking the
// jamsesh binary's hook subcommands with crafted JSON stdin.
type Driver struct {
	// BinaryPath is the absolute path to the jamsesh binary under test.
	// Required.
	BinaryPath string

	// DataDir is the value of CLAUDE_PLUGIN_DATA the binary will read.
	// Required; should be a per-test tmpdir (use t.TempDir()).
	DataDir string

	// ExtraEnv is appended to the subprocess environment, allowing tests
	// to override JAMSESH_PORTAL_URL and similar variables.
	ExtraEnv []string
}

// StartSession emits the session-start hook event and returns the binary's
// response parsed from stdout.
func (d *Driver) StartSession(ctx context.Context, in SessionStartInput) (SessionStartOutput, error) {
	return runHook[SessionStartInput, SessionStartOutput](ctx, d, "session-start", in)
}

// SubmitPrompt emits the user-prompt-submit hook event and returns the
// binary's response.
func (d *Driver) SubmitPrompt(ctx context.Context, in UserPromptSubmitInput) (UserPromptSubmitOutput, error) {
	return runHook[UserPromptSubmitInput, UserPromptSubmitOutput](ctx, d, "user-prompt-submit", in)
}

// PreToolUse emits the pre-tool-use hook event and returns the binary's
// permission decision.
func (d *Driver) PreToolUse(ctx context.Context, in PreToolUseInput) (PreToolUseOutput, error) {
	return runHook[PreToolUseInput, PreToolUseOutput](ctx, d, "pre-tool-use", in)
}

// PostToolUse emits the post-tool-use hook event and returns the binary's
// response.
func (d *Driver) PostToolUse(ctx context.Context, in PostToolUseInput) (PostToolUseOutput, error) {
	return runHook[PostToolUseInput, PostToolUseOutput](ctx, d, "post-tool-use", in)
}

// Stop emits the stop hook event and returns the binary's response.
func (d *Driver) Stop(ctx context.Context, in StopInput) (StopOutput, error) {
	return runHook[StopInput, StopOutput](ctx, d, "stop", in)
}

// SessionEnd emits the session-end hook event and returns the binary's
// response.
func (d *Driver) SessionEnd(ctx context.Context, in SessionEndInput) (SessionEndOutput, error) {
	return runHook[SessionEndInput, SessionEndOutput](ctx, d, "session-end", in)
}

// runHook is the internal subprocess invoker shared by all six methods.
// It marshals in to JSON, pipes it to the binary via stdin, and unmarshals
// the stdout response into O.
func runHook[I, O any](ctx context.Context, d *Driver, subcmd string, in I) (O, error) {
	var out O
	payload, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	cmd := exec.CommandContext(ctx, d.BinaryPath, "hook", subcmd)
	cmd.Env = append(append([]string{}, d.ExtraEnv...), "CLAUDE_PLUGIN_DATA="+d.DataDir)
	cmd.Stdin = bytes.NewReader(payload)
	stdout, err := cmd.Output()
	if err != nil {
		return out, err
	}
	if len(stdout) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(stdout, &out); err != nil {
		return out, err
	}
	return out, nil
}
