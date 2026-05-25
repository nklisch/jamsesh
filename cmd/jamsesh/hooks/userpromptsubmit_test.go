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
	"time"

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
	t.Setenv("JAMSESH_DATA_DIR", dir)

	_ = os.Chmod(dir, 0o700)
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
	setupHookEnv(t, "http://placeholder", sessionID, orgID, ref, accountID)
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

	setupHookEnv(t, "http://placeholder", sessionID, orgID, ref, accountID)
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

// TestUserPromptSubmit_destructionWarningUrgent verifies that when the digest
// includes a playground.destruction_warning in urgent_events, the hook renders
// it prominently in the additionalContext before the regular digest text.
func TestUserPromptSubmit_destructionWarningUrgent(t *testing.T) {
	const (
		orgID     = "org-ups-005"
		sessionID = "sess-ups-005"
		accountID = "acct-ups-005"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	setHookRunGit(t, func(_ ...string) (string, string, int) { return "", "", 0 })

	// Fix the ends_at time so the test assertion is deterministic.
	endsAt := time.Date(2026, 5, 24, 14, 30, 0, 0, time.UTC)
	endsAtStr := endsAt.Format(time.RFC3339)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/digest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Respond with a digest that contains an urgent playground destruction warning.
		fmt.Fprintf(w, `{
			"next_cursor": 99,
			"text": "## Digest\nNo peer activity.\n",
			"urgent_events": [
				{
					"reason": "idle_timeout",
					"ends_at": %q,
					"remaining_seconds": 287,
					"session_id": %q
				}
			]
		}`, endsAtStr, sessionID)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupHookEnv(t, srv.URL, sessionID, orgID, ref, accountID)

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

	// Warning section must appear before the regular digest text.
	urgentIdx := strings.Index(ac, "⚠️")
	digestIdx := strings.Index(ac, "## Digest")
	if urgentIdx < 0 {
		t.Errorf("additionalContext missing ⚠️ warning; got:\n%s", ac)
	}
	if digestIdx < 0 {
		t.Errorf("additionalContext missing digest text; got:\n%s", ac)
	}
	if urgentIdx >= 0 && digestIdx >= 0 && urgentIdx > digestIdx {
		t.Errorf("warning (index %d) should appear before digest text (index %d); got:\n%s",
			urgentIdx, digestIdx, ac)
	}

	// Check the human-readable duration (287s = 4 min 47 sec).
	if !strings.Contains(ac, "4 min 47 sec") {
		t.Errorf("additionalContext missing human duration '4 min 47 sec'; got:\n%s", ac)
	}

	// Check the reason and ends_at.
	if !strings.Contains(ac, "idle_timeout") {
		t.Errorf("additionalContext missing reason 'idle_timeout'; got:\n%s", ac)
	}
	if !strings.Contains(ac, endsAtStr) {
		t.Errorf("additionalContext missing ends_at %q; got:\n%s", endsAtStr, ac)
	}

	// Check the finalize instruction.
	if !strings.Contains(ac, "jamsesh finalize --local") {
		t.Errorf("additionalContext missing finalize instruction; got:\n%s", ac)
	}
}

// TestUserPromptSubmit_nonPlaygroundDigestUnchanged is a regression test: a
// digest with no urgent_events must produce exactly the same additionalContext
// as before this change (no spurious urgent section injected).
func TestUserPromptSubmit_nonPlaygroundDigestUnchanged(t *testing.T) {
	const (
		orgID     = "org-ups-006"
		sessionID = "sess-ups-006"
		accountID = "acct-ups-006"
	)
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	setHookRunGit(t, func(_ ...string) (string, string, int) { return "", "", 0 })

	const digestText = "## Digest\nPeer pushed 1 commit.\n"

	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/digest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Regular durable-session digest: no urgent_events field at all.
		fmt.Fprintf(w, `{"next_cursor":10,"text":%q}`, digestText)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupHookEnv(t, srv.URL, sessionID, orgID, ref, accountID)

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

	// No warning section should appear.
	if strings.Contains(ac, "⚠️") {
		t.Errorf("non-playground digest must not contain ⚠️ warning; got:\n%s", ac)
	}
	if strings.Contains(ac, "Playground session ending") {
		t.Errorf("non-playground digest must not contain destruction warning; got:\n%s", ac)
	}
	// Regular text must be present.
	if !strings.Contains(ac, "Peer pushed 1 commit") {
		t.Errorf("additionalContext missing expected digest text; got:\n%s", ac)
	}
}
