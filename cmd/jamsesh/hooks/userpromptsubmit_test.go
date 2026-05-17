package hooks_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"jamsesh/cmd/jamsesh/hooks"
	"jamsesh/cmd/jamsesh/retryqueue"
)

// setHookRunGit replaces the hookRunGit package variable for the duration of
// the test and restores it on cleanup.
func setHookRunGit(t *testing.T, fn func(args ...string) (stdout, stderr string, exitCode int)) {
	t.Helper()
	orig := hooks.HookRunGit
	hooks.HookRunGit = fn
	t.Cleanup(func() { hooks.HookRunGit = orig })
}

func TestUserPromptSubmit_noSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
	t.Setenv("JAMSESH_PORTAL_URL", "http://localhost:9")
	t.Setenv("CC_SESSION_ID", "")

	in := strings.NewReader(`{"session_id":"cc","transcript_path":"","cwd":""}`)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	if err := hooks.UserPromptSubmit(ctx, nil); err != nil {
		t.Fatalf("UserPromptSubmit error: %v", err)
	}
	var result map[string]any
	if err := json.NewDecoder(&out).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ac, ok := result["additionalContext"]; ok && ac != "" {
		t.Errorf("expected no additionalContext when no session, got %q", ac)
	}
}

func TestUserPromptSubmit_fetchAndDigest(t *testing.T) {
	const (
		orgID     = "org-ups-001"
		sessionID = "sess-ups-001"
		accountID = "acct-ups-001"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	// Track git calls.
	var gitCalls [][]string
	setHookRunGit(t, func(args ...string) (string, string, int) {
		gitCalls = append(gitCalls, append([]string(nil), args...))
		return "", "", 0 // always succeed
	})

	const digestText = "## Digest\nPeer pushed 2 commits.\n"
	var capturedSince string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/digest", func(w http.ResponseWriter, r *http.Request) {
		capturedSince = r.URL.Query().Get("since")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"next_cursor":42,"text":%q}`, digestText)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := setupHookEnv(t, srv.URL, sessionID, orgID, ref, accountID)

	in := strings.NewReader(`{"session_id":"cc","transcript_path":"","cwd":""}`)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	if err := hooks.UserPromptSubmit(ctx, nil); err != nil {
		t.Fatalf("UserPromptSubmit error: %v", err)
	}

	var result map[string]any
	if err := json.NewDecoder(&out).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	ac := additionalContext(t, result)
	if !strings.Contains(ac, "Peer pushed 2 commits") {
		t.Errorf("additionalContext missing digest text; got:\n%s", ac)
	}

	// Verify git fetch was called.
	foundFetch := false
	for _, call := range gitCalls {
		if len(call) >= 2 && call[0] == "fetch" && call[1] == "session-remote" {
			foundFetch = true
			break
		}
	}
	if !foundFetch {
		t.Errorf("expected git fetch session-remote call, git calls: %v", gitCalls)
	}

	// Verify since=0 was sent (initial state).
	if capturedSince != "0" {
		t.Errorf("digest since = %q, want 0", capturedSince)
	}

	// Verify lastSeq was updated to 42.
	seqPath := filepath.Join(dir, "sessions", sessionID, "last_seen_seq")
	data, err := os.ReadFile(seqPath)
	if err != nil {
		t.Fatalf("reading last_seen_seq: %v", err)
	}
	if strings.TrimSpace(string(data)) != "42" {
		t.Errorf("last_seen_seq = %q, want 42", strings.TrimSpace(string(data)))
	}
}

func TestUserPromptSubmit_drainQueueSuccess(t *testing.T) {
	const (
		orgID     = "org-ups-002"
		sessionID = "sess-ups-002"
		accountID = "acct-ups-002"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	// Pre-populate the retry queue with one entry.
	dir := setupHookEnv(t, "http://placeholder", sessionID, orgID, ref, accountID)
	q := &retryqueue.Queue{SessionID: sessionID}
	_ = q.Enqueue(retryqueue.Entry{CommitSHA: "abc123", Attempts: 1})

	// Track git push calls; they all succeed.
	var pushCalls [][]string
	setHookRunGit(t, func(args ...string) (string, string, int) {
		if args[0] == "push" {
			pushCalls = append(pushCalls, append([]string(nil), args...))
		}
		return "", "", 0
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/digest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"next_cursor":1,"text":""}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Update JAMSESH_PORTAL_URL to real test server.
	t.Setenv("JAMSESH_PORTAL_URL", srv.URL)
	_ = dir

	in := strings.NewReader(`{"session_id":"cc","transcript_path":"","cwd":""}`)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	if err := hooks.UserPromptSubmit(ctx, nil); err != nil {
		t.Fatalf("UserPromptSubmit error: %v", err)
	}

	// Verify the queued commit was pushed.
	if len(pushCalls) == 0 {
		t.Errorf("expected at least one push call for queued commit")
	}

	// Verify queue is now empty.
	size, err := q.Size()
	if err != nil {
		t.Fatalf("queue size: %v", err)
	}
	if size != 0 {
		t.Errorf("queue size = %d, want 0 after successful drain", size)
	}
}

func TestUserPromptSubmit_drainQueueTransientReEnqueue(t *testing.T) {
	const (
		orgID     = "org-ups-003"
		sessionID = "sess-ups-003"
		accountID = "acct-ups-003"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	dir := setupHookEnv(t, "http://placeholder", sessionID, orgID, ref, accountID)
	q := &retryqueue.Queue{SessionID: sessionID}
	_ = q.Enqueue(retryqueue.Entry{CommitSHA: "deadbeef", Attempts: 1})

	// All git pushes fail transiently (exit 1, no HTTP status in stderr).
	setHookRunGit(t, func(args ...string) (string, string, int) {
		if args[0] == "push" {
			return "", "connection refused", 1
		}
		return "", "", 0
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/digest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"next_cursor":0,"text":""}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("JAMSESH_PORTAL_URL", srv.URL)
	_ = dir

	in := strings.NewReader(`{"session_id":"cc","transcript_path":"","cwd":""}`)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	_ = hooks.UserPromptSubmit(ctx, nil)

	// Commit should still be in queue (re-enqueued after transient failure).
	size, err := q.Size()
	if err != nil {
		t.Fatalf("queue size: %v", err)
	}
	if size == 0 {
		t.Error("expected commit to be re-enqueued after transient failure, queue is empty")
	}
}

func TestUserPromptSubmit_advancesLastSeq(t *testing.T) {
	const (
		orgID     = "org-ups-004"
		sessionID = "sess-ups-004"
		accountID = "acct-ups-004"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	setHookRunGit(t, func(_ ...string) (string, string, int) { return "", "", 0 })

	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/digest", func(w http.ResponseWriter, r *http.Request) {
		since := r.URL.Query().Get("since")
		// Echo the since param back so we can verify it was used.
		n, _ := strconv.ParseInt(since, 10, 64)
		next := n + 100
		fmt.Fprintf(w, `{"next_cursor":%d,"text":"digest text"}`, next)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := setupHookEnv(t, srv.URL, sessionID, orgID, ref, accountID)

	// Write an initial non-zero lastSeq.
	seqPath := filepath.Join(dir, "sessions", sessionID, "last_seen_seq")
	if err := os.WriteFile(seqPath, []byte("50"), 0o600); err != nil {
		t.Fatalf("writing last_seen_seq: %v", err)
	}

	in := strings.NewReader(`{"session_id":"cc","transcript_path":"","cwd":""}`)
	var out bytes.Buffer
	ctx := hooks.WithIO(context.Background(), in, &out)
	if err := hooks.UserPromptSubmit(ctx, nil); err != nil {
		t.Fatalf("UserPromptSubmit error: %v", err)
	}

	data, err := os.ReadFile(seqPath)
	if err != nil {
		t.Fatalf("reading last_seen_seq: %v", err)
	}
	if strings.TrimSpace(string(data)) != "150" {
		t.Errorf("last_seen_seq after call = %q, want 150", strings.TrimSpace(string(data)))
	}
}
