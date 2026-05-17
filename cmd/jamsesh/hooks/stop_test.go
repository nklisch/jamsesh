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

// runStop drives the Stop action with the given JSON input and returns decoded output.
// It does NOT call os.Exit — callers that test the wedged-queue path need a
// hook env with a queue size > 10, but stop's exit is guarded by the queue
// size check happening AFTER the push. Tests that just want to verify
// stop's auto-commit and push behaviour can use a push that succeeds.
func runStop(t *testing.T, inputJSON string) map[string]any {
	t.Helper()
	in := strings.NewReader(inputJSON)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	if err := hooks.Stop(ctx, nil); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
	var result map[string]any
	if err := json.NewDecoder(&out).Decode(&result); err != nil {
		t.Fatalf("decoding output: %v\nraw: %s", err, out.String())
	}
	return result
}

const stopInput = `{"session_id":"cc","transcript_path":""}`

func TestStop_noSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
	t.Setenv("CC_SESSION_ID", "")

	setHookRunGit(t, func(_ ...string) (string, string, int) { return "", "", 0 })

	r := runStop(t, stopInput)
	_ = r // no assertions — just verify it doesn't error
}

func TestStop_cleanTree_noAutoCommit(t *testing.T) {
	const (
		orgID     = "org-stop-001"
		sessionID = "sess-stop-001"
		accountID = "acct-stop-001"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	var gitCalls [][]string
	setHookRunGit(t, func(args ...string) (string, string, int) {
		gitCalls = append(gitCalls, append([]string(nil), args...))
		// "status --porcelain" returns empty → clean tree
		return "", "", 0
	})

	setupHookEnv(t, "http://placeholder", sessionID, orgID, ref, accountID)

	r := runStop(t, stopInput)
	_ = r

	// Verify "git commit" was NOT called (tree was clean).
	for _, call := range gitCalls {
		if len(call) > 0 && call[0] == "commit" {
			t.Errorf("git commit should not be called on a clean tree; git calls: %v", gitCalls)
		}
	}
	// Verify "git push" WAS called.
	foundPush := false
	for _, call := range gitCalls {
		if len(call) > 0 && call[0] == "push" {
			foundPush = true
			break
		}
	}
	if !foundPush {
		t.Errorf("expected git push call on clean tree; git calls: %v", gitCalls)
	}
}

func TestStop_dirtyTree_autoCommit(t *testing.T) {
	const (
		orgID     = "org-stop-002"
		sessionID = "sess-stop-002"
		accountID = "acct-stop-002"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	var commitArgs []string
	var addCalled bool
	callCount := 0
	setHookRunGit(t, func(args ...string) (string, string, int) {
		callCount++
		if args[0] == "status" {
			// Return non-empty → dirty tree
			return " M modified-file.go", "", 0
		}
		if args[0] == "add" {
			addCalled = true
			return "", "", 0
		}
		if args[0] == "commit" {
			commitArgs = append([]string(nil), args...)
			return "", "", 0
		}
		// rev-parse for ref + push
		if args[0] == "rev-parse" {
			if len(args) > 1 && args[1] == "--abbrev-ref" {
				return ref, "", 0
			}
			return "abc123", "", 0
		}
		return "", "", 0
	})

	setupHookEnv(t, "http://placeholder", sessionID, orgID, ref, accountID)

	r := runStop(t, stopInput)
	_ = r

	if !addCalled {
		t.Error("expected git add -A to be called on dirty tree")
	}
	if len(commitArgs) == 0 {
		t.Error("expected git commit to be called on dirty tree")
	}
	// Verify the auto-commit message contains the expected marker.
	commitMsg := ""
	for i, a := range commitArgs {
		if a == "-m" && i+1 < len(commitArgs) {
			commitMsg = commitArgs[i+1]
		}
	}
	if !strings.Contains(commitMsg, "jamsesh auto-commit") {
		t.Errorf("auto-commit message = %q, want to contain 'jamsesh auto-commit'", commitMsg)
	}
	// Verify --trailer is included.
	hasTrailer := false
	for _, a := range commitArgs {
		if strings.Contains(a, "Jam-Auto-Commit") {
			hasTrailer = true
		}
	}
	if !hasTrailer {
		t.Errorf("expected Jam-Auto-Commit trailer in commit args: %v", commitArgs)
	}
}

func TestStop_transientPush_enqueues(t *testing.T) {
	const (
		orgID     = "org-stop-003"
		sessionID = "sess-stop-003"
		accountID = "acct-stop-003"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	setHookRunGit(t, func(args ...string) (string, string, int) {
		if args[0] == "status" {
			return "", "", 0 // clean tree
		}
		if args[0] == "rev-parse" {
			if len(args) > 1 && args[1] == "--abbrev-ref" {
				return ref, "", 0
			}
			return "deadbeef", "", 0
		}
		if args[0] == "push" {
			return "", "connection refused", 1 // transient
		}
		return "", "", 0
	})

	setupHookEnv(t, "http://placeholder", sessionID, orgID, ref, accountID)

	in := strings.NewReader(stopInput)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	// Stop calls os.Exit(1) only if queue > 10, so we just call it normally.
	_ = hooks.Stop(ctx, nil)

	// The commit should have been enqueued.
	q := &retryqueue.Queue{SessionID: sessionID}
	size, err := q.Size()
	if err != nil {
		t.Fatalf("queue size: %v", err)
	}
	if size == 0 {
		t.Error("expected commit to be enqueued after transient push failure in stop")
	}
}

func TestStop_permanentPush_doesNotEnqueue(t *testing.T) {
	const (
		orgID     = "org-stop-004"
		sessionID = "sess-stop-004"
		accountID = "acct-stop-004"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	permanentStderr := `error: 422 Unprocessable Entity
{"error":"push.scope_violation","message":"path outside scope"}`

	setHookRunGit(t, func(args ...string) (string, string, int) {
		if args[0] == "status" {
			return "", "", 0
		}
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

	in := strings.NewReader(stopInput)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	_ = hooks.Stop(ctx, nil)

	// Permanent failure: should NOT be enqueued.
	q := &retryqueue.Queue{SessionID: sessionID}
	size, _ := q.Size()
	if size != 0 {
		t.Errorf("permanent push failure should not be enqueued; queue size = %d", size)
	}
}
