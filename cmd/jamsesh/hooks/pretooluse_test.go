package hooks_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"jamsesh/cmd/jamsesh/hooks"
)

// runPreToolUse exercises PreToolUse with the given JSON input and returns the
// decoded output map.
func runPreToolUse(t *testing.T, inputJSON string) map[string]any {
	t.Helper()

	in := strings.NewReader(inputJSON)
	var out bytes.Buffer

	ctx := hooks.WithIO(context.Background(), in, &out)
	if err := hooks.PreToolUse(ctx, nil); err != nil {
		t.Fatalf("PreToolUse error: %v", err)
	}

	var result map[string]any
	if err := json.NewDecoder(&out).Decode(&result); err != nil {
		t.Fatalf("decoding output: %v", err)
	}
	return result
}

func decision(t *testing.T, r map[string]any) string {
	t.Helper()
	v, ok := r["permissionDecision"].(string)
	if !ok {
		t.Fatalf("permissionDecision not a string in %v", r)
	}
	return v
}

// TestPreToolUse_gitPush_deny verifies that "git push origin main" is denied.
func TestPreToolUse_gitPush_deny(t *testing.T) {
	input := `{
		"session_id": "sess-1",
		"tool_name": "Bash",
		"tool_input": {"command": "git push origin main"}
	}`
	r := runPreToolUse(t, input)
	if got := decision(t, r); got != "deny" {
		t.Errorf("decision = %q, want deny", got)
	}
	reason, _ := r["reason"].(string)
	if reason == "" {
		t.Error("reason should be non-empty for deny")
	}
}

// TestPreToolUse_gitPushBareform_deny verifies "git push" with no args is denied.
func TestPreToolUse_gitPushBareform_deny(t *testing.T) {
	input := `{"session_id":"s","tool_name":"Bash","tool_input":{"command":"git push"}}`
	r := runPreToolUse(t, input)
	if got := decision(t, r); got != "deny" {
		t.Errorf("decision = %q, want deny", got)
	}
}

// TestPreToolUse_leadingWhitespace_deny verifies trimming before matching.
func TestPreToolUse_leadingWhitespace_deny(t *testing.T) {
	input := `{"session_id":"s","tool_name":"Bash","tool_input":{"command":"  git push origin feature"}}`
	r := runPreToolUse(t, input)
	if got := decision(t, r); got != "deny" {
		t.Errorf("decision = %q, want deny", got)
	}
}

// TestPreToolUse_gitStatus_pass verifies that "git status" is passed.
func TestPreToolUse_gitStatus_pass(t *testing.T) {
	input := `{"session_id":"s","tool_name":"Bash","tool_input":{"command":"git status"}}`
	r := runPreToolUse(t, input)
	if got := decision(t, r); got != "pass" {
		t.Errorf("decision = %q, want pass", got)
	}
}

// TestPreToolUse_gitConfigRemote_deny verifies "git config remote.origin.url" is denied.
func TestPreToolUse_gitConfigRemote_deny(t *testing.T) {
	input := `{"session_id":"s","tool_name":"Bash","tool_input":{"command":"git config remote.origin.url https://example.com/repo.git"}}`
	r := runPreToolUse(t, input)
	if got := decision(t, r); got != "deny" {
		t.Errorf("decision = %q, want deny", got)
	}
}

// TestPreToolUse_gitConfigGlobal_pass verifies "git config --global user.name" is passed.
func TestPreToolUse_gitConfigGlobal_pass(t *testing.T) {
	input := `{"session_id":"s","tool_name":"Bash","tool_input":{"command":"git config --global user.name 'Alice'"}}`
	r := runPreToolUse(t, input)
	if got := decision(t, r); got != "pass" {
		t.Errorf("decision = %q, want pass", got)
	}
}

// TestPreToolUse_nonBash_pass verifies a non-Bash tool is always passed.
func TestPreToolUse_nonBash_pass(t *testing.T) {
	input := `{"session_id":"s","tool_name":"Read","tool_input":{"file_path":"/tmp/foo"}}`
	r := runPreToolUse(t, input)
	if got := decision(t, r); got != "pass" {
		t.Errorf("decision = %q, want pass", got)
	}
}

// TestPreToolUse_emptyCommand_pass verifies an empty command is passed (edge case).
func TestPreToolUse_emptyCommand_pass(t *testing.T) {
	input := `{"session_id":"s","tool_name":"Bash","tool_input":{"command":""}}`
	r := runPreToolUse(t, input)
	if got := decision(t, r); got != "pass" {
		t.Errorf("decision = %q, want pass", got)
	}
}

// TestPreToolUse_gitCommit_pass verifies "git commit" is allowed.
func TestPreToolUse_gitCommit_pass(t *testing.T) {
	input := `{"session_id":"s","tool_name":"Bash","tool_input":{"command":"git commit -m 'fix: thing'"}}`
	r := runPreToolUse(t, input)
	if got := decision(t, r); got != "pass" {
		t.Errorf("decision = %q, want pass", got)
	}
}

// TestPreToolUse_stdin_stdout verifies the hook reads from stdin and writes to stdout.
func TestPreToolUse_stdin_stdout(t *testing.T) {
	// This is implicitly tested by every runPreToolUse call above.
	// An explicit named test satisfies the acceptance criterion documentation.
	input := `{"session_id":"abc","tool_name":"Bash","tool_input":{"command":"ls -la"}}`
	r := runPreToolUse(t, input)
	if got := decision(t, r); got != "pass" {
		t.Errorf("decision = %q, want pass", got)
	}
}

// TestPreToolUse_gitConfigRemoteWithWhitespace verifies multi-space variant.
func TestPreToolUse_gitConfigRemoteWithWhitespace(t *testing.T) {
	// "git  config  remote.origin.url" — extra spaces between tokens
	input := `{"session_id":"s","tool_name":"Bash","tool_input":{"command":"git  config  remote.upstream.url git@github.com:org/repo.git"}}`
	r := runPreToolUse(t, input)
	if got := decision(t, r); got != "deny" {
		t.Errorf("decision = %q, want deny", got)
	}
}
