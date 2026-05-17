package hooks_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"jamsesh/cmd/jamsesh/hooks"
	"jamsesh/cmd/jamsesh/retryqueue"
)

// buildBashCommitInput constructs a CC PostToolUse JSON input for a Bash
// "git commit" invocation with the given exit code.
func buildBashCommitInput(command string, exitCode int) string {
	bashInput, _ := json.Marshal(map[string]string{"command": command})
	return string(mustMarshal(map[string]any{
		"session_id":     "cc-sess",
		"tool_name":      "Bash",
		"tool_input":     json.RawMessage(bashInput),
		"tool_response":  map[string]any{"exit_code": exitCode},
	}))
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// runPostToolUse drives the PostToolUse action and returns the decoded output.
func runPostToolUse(t *testing.T, inputJSON string) map[string]any {
	t.Helper()
	in := strings.NewReader(inputJSON)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	if err := hooks.PostToolUse(ctx, nil); err != nil {
		t.Fatalf("PostToolUse error: %v", err)
	}
	var result map[string]any
	if err := json.NewDecoder(&out).Decode(&result); err != nil {
		t.Fatalf("decoding output: %v\nraw: %s", err, out.String())
	}
	return result
}

func TestPostToolUse_nonBash_noOp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)

	input := `{"session_id":"cc","tool_name":"Read","tool_input":{"file_path":"/tmp/f"},"tool_response":{"exit_code":0}}`
	r := runPostToolUse(t, input)
	if ac, ok := r["additionalContext"]; ok && ac != "" {
		t.Errorf("expected empty output for non-Bash tool, got %q", ac)
	}
}

func TestPostToolUse_gitStatus_noOp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)

	input := buildBashCommitInput("git status", 0)
	// Command is "git status", not "git commit" → should no-op.
	r := runPostToolUse(t, input)
	if ac, ok := r["additionalContext"]; ok && ac != "" {
		t.Errorf("expected no-op for non-commit command, got %q", ac)
	}
}

func TestPostToolUse_gitCommitFailed_noOp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)

	input := buildBashCommitInput("git commit -m 'fix: thing'", 1) // exit_code=1 → failed commit
	r := runPostToolUse(t, input)
	if ac, ok := r["additionalContext"]; ok && ac != "" {
		t.Errorf("expected no-op for failed commit, got %q", ac)
	}
}

func TestPostToolUse_gitCommit_success_noSession(t *testing.T) {
	// Successful git commit but no jamsesh session → no-op.
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
	t.Setenv("CC_SESSION_ID", "")

	setHookRunGit(t, func(_ ...string) (string, string, int) { return "", "", 0 })

	input := buildBashCommitInput("git commit -m 'feat: thing'", 0)
	r := runPostToolUse(t, input)
	if ac, ok := r["additionalContext"]; ok && ac != "" {
		t.Errorf("expected no-op with no session, got %q", ac)
	}
}

func TestPostToolUse_gitCommit_pushSuccess(t *testing.T) {
	const (
		orgID     = "org-ptu-001"
		sessionID = "sess-ptu-001"
		accountID = "acct-ptu-001"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	var pushArgs []string
	setHookRunGit(t, func(args ...string) (string, string, int) {
		if len(args) > 0 && args[0] == "rev-parse" {
			if len(args) > 1 && args[1] == "--abbrev-ref" {
				return ref, "", 0
			}
			return "abc1234567890", "", 0
		}
		if len(args) > 0 && args[0] == "push" {
			pushArgs = append([]string(nil), args...)
			return "", "", 0 // success
		}
		return "", "", 0
	})

	setupHookEnv(t, "http://placeholder", sessionID, orgID, ref, accountID)

	input := buildBashCommitInput("git commit -m 'feat: great'", 0)
	r := runPostToolUse(t, input)

	// On success, no additionalContext.
	if ac, ok := r["additionalContext"]; ok && ac != "" {
		t.Errorf("expected empty additionalContext on push success, got %q", ac)
	}
	// Verify push was called.
	if len(pushArgs) == 0 {
		t.Error("expected push call, got none")
	}
}

func TestPostToolUse_gitCommit_transientAllFail_enqueues(t *testing.T) {
	const (
		orgID     = "org-ptu-002"
		sessionID = "sess-ptu-002"
		accountID = "acct-ptu-002"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	setHookRunGit(t, func(args ...string) (string, string, int) {
		if args[0] == "rev-parse" {
			if len(args) > 1 && args[1] == "--abbrev-ref" {
				return ref, "", 0
			}
			return "deadbeef1234", "", 0
		}
		if args[0] == "push" {
			return "", "connection refused", 1 // transient
		}
		return "", "", 0
	})

	setupHookEnv(t, "http://placeholder", sessionID, orgID, ref, accountID)

	input := buildBashCommitInput("git commit -m 'wip'", 0)
	r := runPostToolUse(t, input)

	ac, _ := r["additionalContext"].(string)
	if !strings.Contains(ac, "queued for retry") {
		t.Errorf("expected 'queued for retry' in additionalContext, got:\n%s", ac)
	}

	// Verify commit was enqueued.
	q := &retryqueue.Queue{SessionID: sessionID}
	size, err := q.Size()
	if err != nil {
		t.Fatalf("queue size: %v", err)
	}
	if size == 0 {
		t.Error("expected commit to be enqueued after transient failure")
	}
}

func TestPostToolUse_gitCommit_permanentFail_failLoud(t *testing.T) {
	const (
		orgID     = "org-ptu-003"
		sessionID = "sess-ptu-003"
		accountID = "acct-ptu-003"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	// Permanent error: HTTP 422 in stderr + JSON error envelope.
	permanentStderr := `error: 422 Unprocessable Entity
{"error":"push.scope_violation","message":"path outside scope","details":{"path":"secret.txt"}}`

	setHookRunGit(t, func(args ...string) (string, string, int) {
		if args[0] == "rev-parse" {
			if len(args) > 1 && args[1] == "--abbrev-ref" {
				return ref, "", 0
			}
			return "abc123", "", 0
		}
		if args[0] == "push" {
			return "", permanentStderr, 1
		}
		return "", "", 0
	})

	setupHookEnv(t, "http://placeholder", sessionID, orgID, ref, accountID)

	input := buildBashCommitInput("git commit -m 'bad'", 0)
	r := runPostToolUse(t, input)

	ac, _ := r["additionalContext"].(string)
	if !strings.Contains(ac, "permanent") {
		t.Errorf("expected 'permanent' in additionalContext, got:\n%s", ac)
	}
	if !strings.Contains(ac, "push.scope_violation") {
		t.Errorf("expected error code in additionalContext, got:\n%s", ac)
	}

	// Permanent failures should NOT be enqueued.
	q := &retryqueue.Queue{SessionID: sessionID}
	size, _ := q.Size()
	if size != 0 {
		t.Errorf("permanent failure should not be enqueued; queue size = %d", size)
	}
}
