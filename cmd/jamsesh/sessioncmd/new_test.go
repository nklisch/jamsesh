package sessioncmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/api/openapi"
)

// setupNewEnv creates a temp JAMSESH_DATA_DIR dir, writes a fake access
// token, and points JAMSESH_PORTAL_URL at the given test server URL.
// It returns the temp dir path.
func setupNewEnv(t *testing.T, srvURL string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)

	_ = os.Chmod(dir, 0o700)
	t.Setenv("JAMSESH_PORTAL_URL", srvURL)
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("tok-test"), 0o600); err != nil {
		t.Fatalf("writing token: %v", err)
	}
	return dir
}

// stubGitForNew overrides the package-level git function vars for the duration
// of a test, restoring them via t.Cleanup.
func stubGitForNew(t *testing.T, pushErr error) *[][]string {
	t.Helper()
	var calls [][]string

	origRunGit := runGit
	origRunGitOutput := runGitOutput
	origRunGitWithEnv := runGitWithEnv
	t.Cleanup(func() {
		runGit = origRunGit
		runGitOutput = origRunGitOutput
		runGitWithEnv = origRunGitWithEnv
	})

	runGit = func(args ...string) error {
		calls = append(calls, append([]string{"git"}, args...))
		return nil
	}
	runGitOutput = func(args ...string) (string, error) {
		calls = append(calls, append([]string{"git-output"}, args...))
		return "abc1234", nil
	}
	runGitWithEnv = func(env []string, args ...string) error {
		calls = append(calls, append([]string{"git-env"}, args...))
		return pushErr
	}

	return &calls
}

// stubIsTTY overrides isTTY for the duration of a test.
func stubIsTTY(t *testing.T, val bool) {
	t.Helper()
	orig := isTTY
	t.Cleanup(func() { isTTY = orig })
	isTTY = func(*os.File) bool { return val }
}

// writeSessionFixture writes a mock Session JSON response.
func writeSessionJSON(w http.ResponseWriter, session openapi.Session) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(session)
}

// sampleSession returns a Session struct for testing.
func sampleSession(orgID, sessionID string) openapi.Session {
	return openapi.Session{
		Id:          sessionID,
		Name:        "Test Session",
		Goal:        "Test goal",
		OrgId:       orgID,
		Scope:       `["**"]`,
		DefaultMode: openapi.SessionDefaultModeSync,
		Members: []openapi.MemberSummary{
			{AccountId: "acct-001", Role: "creator"},
		},
	}
}

func TestNewAction_happyPathSingleOrg(t *testing.T) {
	const (
		orgID     = "org-001"
		sessionID = "sess-001"
		accountID = "acct-001"
	)

	var createCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "want POST", http.StatusMethodNotAllowed)
			return
		}
		createCalled = true
		writeSessionJSON(w, sampleSession(orgID, sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := setupNewEnv(t, srv.URL)
	gitCalls := stubGitForNew(t, nil)
	stubIsTTY(t, false) // non-TTY so --org is required path; we pass --org

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{
		"jamsesh", "new",
		"--org", orgID,
		"--non-interactive",
	})
	if err != nil {
		t.Fatalf("new returned error: %v", err)
	}

	if !createCalled {
		t.Error("expected create-session POST to be called")
	}

	// Verify at least one git push call was made.
	foundPush := false
	for _, call := range *gitCalls {
		for _, arg := range call {
			if arg == "push" {
				foundPush = true
				break
			}
		}
	}
	if !foundPush {
		t.Errorf("expected git push call, got: %v", *gitCalls)
	}

	// Verify state files written.
	refPath := filepath.Join(dir, "sessions", sessionID, "ref")
	data, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("reading ref state: %v", err)
	}
	expectedRef := "jam/" + sessionID + "/" + accountID + "/main"
	if string(data) != expectedRef {
		t.Errorf("ref = %q, want %q", string(data), expectedRef)
	}

	orgIDPath := filepath.Join(dir, "sessions", sessionID, "org_id")
	orgIDData, err := os.ReadFile(orgIDPath)
	if err != nil {
		t.Fatalf("reading org_id state: %v", err)
	}
	if string(orgIDData) != orgID {
		t.Errorf("org_id = %q, want %q", string(orgIDData), orgID)
	}

	// instance_id must NOT be written (deferred to first attach).
	instancePath := filepath.Join(dir, "sessions", sessionID, "instance_id")
	if _, err := os.Stat(instancePath); err == nil {
		t.Error("instance_id file should NOT be written by jamsesh new (deferred to attach)")
	}
}

func TestNewAction_happyPathMultiOrg(t *testing.T) {
	const (
		orgID1    = "org-alpha"
		orgID2    = "org-beta"
		sessionID = "sess-002"
		accountID = "acct-002"
	)

	var capturedOrgID string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID1, orgID2)
	})
	mux.HandleFunc("/api/orgs/"+orgID1+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		capturedOrgID = orgID1
		writeSessionJSON(w, sampleSession(orgID1, sessionID))
	})
	mux.HandleFunc("/api/orgs/"+orgID2+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		capturedOrgID = orgID2
		writeSessionJSON(w, sampleSession(orgID2, sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := setupNewEnv(t, srv.URL)
	stubGitForNew(t, nil)
	stubIsTTY(t, true) // TTY so interactive picker runs

	// Pre-set last_org_id to orgID2 so picker defaults to it.
	if err := os.WriteFile(filepath.Join(dir, "last_org_id"), []byte(orgID2), 0o600); err != nil {
		t.Fatalf("writing last_org_id: %v", err)
	}

	// Override stdin reader to simulate pressing Enter (accepts default = orgID2).
	origReadStdinLine := readStdinLine
	t.Cleanup(func() { readStdinLine = origReadStdinLine })
	readStdinLine = func() (string, error) { return "", nil } // empty = accept default

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "new"})
	if err != nil {
		t.Fatalf("new returned error: %v", err)
	}

	if capturedOrgID != orgID2 {
		t.Errorf("expected pre-selected org %q to be used, got %q", orgID2, capturedOrgID)
	}
}

func TestNewAction_nonInteractiveRequiresOrg(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)

	_ = os.Chmod(dir, 0o700)
	t.Setenv("JAMSESH_PORTAL_URL", "http://localhost:19998")
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("tok"), 0o600); err != nil {
		t.Fatal(err)
	}

	stubIsTTY(t, false)

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "new", "--non-interactive"})
	if err == nil {
		t.Fatal("expected error when --non-interactive without --org, got nil")
	}
	if !strings.Contains(err.Error(), "--org") {
		t.Errorf("error should mention --org flag, got: %v", err)
	}
}

func TestNewAction_pushFailureLeavesSessionLive(t *testing.T) {
	const (
		orgID     = "org-003"
		sessionID = "sess-003"
		accountID = "acct-003"
	)

	var abandonCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeSessionJSON(w, sampleSession(orgID, sessionID))
	})
	// Register a catch-all to detect any abandon/delete calls.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete || strings.Contains(r.URL.Path, "abandon") {
			abandonCalled = true
		}
		http.NotFound(w, r)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupNewEnv(t, srv.URL)
	stubIsTTY(t, false)

	origRunGit := runGit
	origRunGitOutput := runGitOutput
	origRunGitWithEnv := runGitWithEnv
	t.Cleanup(func() {
		runGit = origRunGit
		runGitOutput = origRunGitOutput
		runGitWithEnv = origRunGitWithEnv
	})
	runGit = func(args ...string) error { return nil }
	runGitOutput = func(args ...string) (string, error) { return "abc1234", nil }
	runGitWithEnv = func(env []string, args ...string) error {
		return fmt.Errorf("simulated push failure: authentication failed")
	}

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "new", "--org", orgID})
	if err == nil {
		t.Fatal("expected error on push failure, got nil")
	}

	// Error message must include the retry command.
	retryPrefix := "git push"
	if !strings.Contains(err.Error(), retryPrefix) {
		t.Errorf("error should include retry git push command, got: %v", err)
	}
	if !strings.Contains(err.Error(), sessionID) {
		t.Errorf("error should include sessionID %q, got: %v", sessionID, err)
	}
	if !strings.Contains(err.Error(), "refs/heads/jam/"+sessionID+"/base") {
		t.Errorf("error should include the refspec, got: %v", err)
	}

	// No abandon call must have been made.
	if abandonCalled {
		t.Error("session should NOT be abandoned on push failure (stays live per locked decision)")
	}
}

func TestNewAction_inviteFlag(t *testing.T) {
	const (
		orgID     = "org-004"
		sessionID = "sess-004"
		accountID = "acct-004"
	)

	var invitedEmails []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeSessionJSON(w, sampleSession(orgID, sessionID))
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/invites", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "want POST", http.StatusMethodNotAllowed)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		invitedEmails = append(invitedEmails, body["email"])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupNewEnv(t, srv.URL)
	stubGitForNew(t, nil)
	stubIsTTY(t, false)

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{
		"jamsesh", "new",
		"--org", orgID,
		"--invite", "a@x.com,b@y.com",
	})
	if err != nil {
		t.Fatalf("new returned error: %v", err)
	}

	if len(invitedEmails) != 2 {
		t.Errorf("expected 2 invite calls, got %d: %v", len(invitedEmails), invitedEmails)
	}
	for _, expected := range []string{"a@x.com", "b@y.com"} {
		found := false
		for _, e := range invitedEmails {
			if e == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected invite to %q, not found in %v", expected, invitedEmails)
		}
	}
}

func TestNewAction_inviteFailureWarnsButSucceeds(t *testing.T) {
	const (
		orgID     = "org-005"
		sessionID = "sess-005"
		accountID = "acct-005"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeSessionJSON(w, sampleSession(orgID, sessionID))
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/invites", func(w http.ResponseWriter, r *http.Request) {
		// Always return 500 — invite fails.
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupNewEnv(t, srv.URL)
	stubGitForNew(t, nil)
	stubIsTTY(t, false)

	app := buildCLIApp()
	// Invite failure should NOT cause the command to fail (exit 0, warning only).
	err := app.Run(context.Background(), []string{
		"jamsesh", "new",
		"--org", orgID,
		"--invite", "fail@x.com",
	})
	if err != nil {
		t.Errorf("invite failure should not fail the create; got error: %v", err)
	}
}

func TestNewAction_emptyRepo(t *testing.T) {
	const (
		orgID     = "org-006"
		sessionID = "sess-006"
		accountID = "acct-006"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeSessionJSON(w, sampleSession(orgID, sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupNewEnv(t, srv.URL)
	stubIsTTY(t, false)

	// runGit succeeds for rev-parse --git-dir but runGitOutput fails for HEAD
	origRunGit := runGit
	origRunGitOutput := runGitOutput
	origRunGitWithEnv := runGitWithEnv
	t.Cleanup(func() {
		runGit = origRunGit
		runGitOutput = origRunGitOutput
		runGitWithEnv = origRunGitWithEnv
	})
	runGit = func(args ...string) error { return nil }
	runGitOutput = func(args ...string) (string, error) {
		// Simulate an empty repo with no commits
		return "", fmt.Errorf("fatal: ambiguous argument 'HEAD'")
	}
	runGitWithEnv = func(env []string, args ...string) error { return nil }

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "new", "--org", orgID})
	if err == nil {
		t.Fatal("expected error for empty repo, got nil")
	}
	if !strings.Contains(err.Error(), "no commits") {
		t.Errorf("error should mention 'no commits', got: %v", err)
	}
}

func TestNewAction_defaultName(t *testing.T) {
	const (
		orgID     = "org-007"
		sessionID = "sess-007"
		accountID = "acct-007"
	)

	var capturedName string
	before := time.Now().Unix()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		var body openapi.CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			capturedName = body.Name
		}
		writeSessionJSON(w, sampleSession(orgID, sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupNewEnv(t, srv.URL)
	stubGitForNew(t, nil)
	stubIsTTY(t, false)

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{
		"jamsesh", "new",
		"--org", orgID,
		// No --name flag
	})
	if err != nil {
		t.Fatalf("new returned error: %v", err)
	}

	after := time.Now().Unix()

	if !strings.HasPrefix(capturedName, "jam-") {
		t.Errorf("default name should have form jam-<timestamp>, got %q", capturedName)
	}
	tsStr := strings.TrimPrefix(capturedName, "jam-")
	var ts int64
	if _, err := fmt.Sscanf(tsStr, "%d", &ts); err != nil {
		t.Errorf("timestamp portion of default name is not numeric: %q", tsStr)
	}
	if ts < before || ts > after+1 {
		t.Errorf("timestamp %d is outside expected range [%d, %d]", ts, before, after+1)
	}
}

func TestNewAction_scopeNormalization(t *testing.T) {
	const (
		orgID     = "org-008"
		sessionID = "sess-008"
		accountID = "acct-008"
	)

	var capturedScope string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		var body openapi.CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			capturedScope = body.Scope
		}
		writeSessionJSON(w, sampleSession(orgID, sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupNewEnv(t, srv.URL)
	stubGitForNew(t, nil)
	stubIsTTY(t, false)

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{
		"jamsesh", "new",
		"--org", orgID,
		"--scope", "docs/**",
	})
	if err != nil {
		t.Fatalf("new returned error: %v", err)
	}

	// Single glob should be normalized to JSON array.
	expected := `["docs/**"]`
	if capturedScope != expected {
		t.Errorf("scope = %q, want %q", capturedScope, expected)
	}
}

// TestNormalizeScope tests the normalizeScope helper in isolation.
func TestNormalizeScope(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{`**`, `["**"]`, false},
		{`docs/**`, `["docs/**"]`, false},
		{`["docs/**","src/*.go"]`, `["docs/**","src/*.go"]`, false},
		{`["a","b","c"]`, `["a","b","c"]`, false},
		{`[invalid json`, "", true},
	}

	for _, tc := range tests {
		got, err := normalizeScope(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeScope(%q): want error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeScope(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeScope(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---- Playground tests ----

// setupPlaygroundEnv creates a temp JAMSESH_DATA_DIR dir WITHOUT an OAuth
// token file (playground is unauthenticated) and points JAMSESH_PORTAL_URL at
// the given test server URL. Returns the temp dir path.
func setupPlaygroundEnv(t *testing.T, srvURL string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)

	_ = os.Chmod(dir, 0o700)
	t.Setenv("JAMSESH_PORTAL_URL", srvURL)
	// Deliberately NO token file — playground must not require auth.
	return dir
}

// samplePlaygroundResp returns a PlaygroundSessionCreated response for testing.
func samplePlaygroundResp(sessionID string) openapi.PlaygroundSessionCreated {
	return openapi.PlaygroundSessionCreated{
		Bearer:    "anon-bearer-abc123",
		Nickname:  "amber-otter",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Session: openapi.PlaygroundSessionSummary{
			Id:            sessionID,
			Name:          "playground-" + sessionID[:6],
			Goal:          "",
			OrgId:         "org_playground",
			Scope:         `["**"]`,
			Status:        openapi.PlaygroundSessionSummaryStatusActive,
			MembersCount:  1,
			CreatedAt:     time.Now(),
			HardCapAt:     time.Now().Add(24 * time.Hour),
			IdleTimeoutAt: time.Now().Add(30 * time.Minute),
		},
	}
}

// writePlaygroundJSON writes a PlaygroundSessionCreated JSON response at 201.
func writePlaygroundJSON(w http.ResponseWriter, resp openapi.PlaygroundSessionCreated) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// TestPlaygroundAction_happyPath verifies the full playground happy path:
// POST /api/playground/sessions with no auth header, bearer stored, push
// uses the received bearer, and per-session state files written.
func TestPlaygroundAction_happyPath(t *testing.T) {
	const sessionID = "sess-pg-001"

	var (
		createCalled    bool
		capturedAuthHdr string
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/playground/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "want POST", http.StatusMethodNotAllowed)
			return
		}
		createCalled = true
		capturedAuthHdr = r.Header.Get("Authorization")
		writePlaygroundJSON(w, samplePlaygroundResp(sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := setupPlaygroundEnv(t, srv.URL)
	gitCalls := stubGitForNew(t, nil)

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "new", "--playground"})
	if err != nil {
		t.Fatalf("new --playground returned error: %v", err)
	}

	if !createCalled {
		t.Error("expected POST /api/playground/sessions to be called")
	}

	// The create request must NOT carry an Authorization header.
	if capturedAuthHdr != "" {
		t.Errorf("playground create must send no auth header, got: %q", capturedAuthHdr)
	}

	// At least one push call must have been made.
	foundPush := false
	for _, call := range *gitCalls {
		for _, arg := range call {
			if arg == "push" {
				foundPush = true
				break
			}
		}
	}
	if !foundPush {
		t.Errorf("expected git push call, git calls: %v", *gitCalls)
	}

	// Per-session state files must have been written.
	orgIDPath := filepath.Join(dir, "sessions", sessionID, "org_id")
	orgIDData, err := os.ReadFile(orgIDPath)
	if err != nil {
		t.Fatalf("reading org_id state: %v", err)
	}
	if string(orgIDData) != "org_playground" {
		t.Errorf("org_id = %q, want %q", string(orgIDData), "org_playground")
	}

	// Bearer must have been stored in per-session token file.
	tokenPath := filepath.Join(dir, "sessions", sessionID, "token")
	tokenData, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("reading session token: %v", err)
	}
	if string(tokenData) != "anon-bearer-abc123" {
		t.Errorf("session token = %q, want %q", string(tokenData), "anon-bearer-abc123")
	}
}

// TestPlaygroundAction_shareURLShape verifies that the share URL printed to
// stdout by "jamsesh new --playground" has the correct portal path format:
// <baseURL>/playground/s/<sessionID>/join
func TestPlaygroundAction_shareURLShape(t *testing.T) {
	const sessionID = "sess-pg-url-001"

	mux := http.NewServeMux()
	mux.HandleFunc("/api/playground/sessions", func(w http.ResponseWriter, r *http.Request) {
		writePlaygroundJSON(w, samplePlaygroundResp(sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupPlaygroundEnv(t, srv.URL)
	stubGitForNew(t, nil)

	app := buildCLIApp()
	var output string
	output = captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{"jamsesh", "new", "--playground"}); err != nil {
			t.Fatalf("new --playground returned error: %v", err)
		}
	})

	expectedURL := srv.URL + "/playground/s/" + sessionID + "/join"
	if !strings.Contains(output, expectedURL) {
		t.Errorf("share URL %q not found in output; got:\n%s", expectedURL, output)
	}
	// Confirm the old bare-session-id form is absent.
	oldURL := srv.URL + "/playground/" + sessionID
	if strings.Contains(output, oldURL) && !strings.Contains(output, "/playground/s/") {
		t.Errorf("old share URL form %q should not appear in output; got:\n%s", oldURL, output)
	}
}

// TestPlaygroundAction_nicknameWritten verifies that the server-minted nickname
// from the PlaygroundSessionCreated response is persisted to the per-session
// nickname sidecar file so "jamsesh status" can display it without re-fetching
// (PlaygroundSessionSummary does not include the nickname field).
func TestPlaygroundAction_nicknameWritten(t *testing.T) {
	const (
		sessionID        = "sess-pg-nick-001"
		expectedNickname = "amber-otter"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/playground/sessions", func(w http.ResponseWriter, r *http.Request) {
		resp := samplePlaygroundResp(sessionID)
		resp.Nickname = expectedNickname
		writePlaygroundJSON(w, resp)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := setupPlaygroundEnv(t, srv.URL)
	stubGitForNew(t, nil)

	app := buildCLIApp()
	if err := app.Run(context.Background(), []string{"jamsesh", "new", "--playground"}); err != nil {
		t.Fatalf("new --playground returned error: %v", err)
	}

	nicknamePath := filepath.Join(dir, "sessions", sessionID, "nickname")
	data, err := os.ReadFile(nicknamePath)
	if err != nil {
		t.Fatalf("nickname file not written: %v", err)
	}
	if string(data) != expectedNickname {
		t.Errorf("nickname = %q, want %q", string(data), expectedNickname)
	}
}

// TestPlaygroundAction_namePassthrough verifies that --name "demo" is sent in
// the create request body.
func TestPlaygroundAction_namePassthrough(t *testing.T) {
	const sessionID = "sess-pg-002"

	var capturedName string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/playground/sessions", func(w http.ResponseWriter, r *http.Request) {
		var body openapi.CreatePlaygroundSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			capturedName = body.Name
		}
		writePlaygroundJSON(w, samplePlaygroundResp(sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupPlaygroundEnv(t, srv.URL)
	stubGitForNew(t, nil)

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{
		"jamsesh", "new", "--playground", "--name", "demo",
	})
	if err != nil {
		t.Fatalf("new --playground --name demo returned error: %v", err)
	}

	if capturedName != "demo" {
		t.Errorf("expected name %q in create request, got %q", "demo", capturedName)
	}
}

// TestPlaygroundAction_mutuallyExclusiveWithOrg verifies that combining
// --playground and --org returns a clear error.
func TestPlaygroundAction_mutuallyExclusiveWithOrg(t *testing.T) {
	// No server needed — error fires before any network call.
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)

	_ = os.Chmod(dir, 0o700)
	t.Setenv("JAMSESH_PORTAL_URL", "http://localhost:19997")
	// Write a token so buildPortalClient doesn't also error (guard fires first,
	// but be defensive to keep the test focused).
	_ = os.WriteFile(filepath.Join(dir, "token"), []byte("tok"), 0o600)

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{
		"jamsesh", "new", "--playground", "--org", "org-foo",
	})
	if err == nil {
		t.Fatal("expected error for --playground --org, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention mutually exclusive, got: %v", err)
	}
}

// TestPlaygroundAction_pushFailureLeavesSessionLiveWithRetry verifies the
// push-failure contract for the playground path:
//   - The returned error is the typed wrapPlaygroundPushError (not a generic
//     error), identified by the sentinel phrase "playground session … is live
//     with base_sha: null".
//   - The error message includes a full retry command with the remote URL and
//     the session-specific refspec so the user can push manually.
//   - No abandon/delete API call is made — the portal session stays live.
//   - The per-session bearer token file is written before the push attempt, so
//     the user can authenticate the manual retry.
//
// Note: org_id and ref state files are written by writePlaygroundSessionState,
// which is only reached on a successful push. They are therefore absent after a
// push failure — the token alone is sufficient to authenticate the retry push.
func TestPlaygroundAction_pushFailureLeavesSessionLiveWithRetry(t *testing.T) {
	const (
		sessionID  = "sess-pg-push-fail-001"
		anonBearer = "anon-bearer-retry-test"
	)

	var abandonCalled bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/playground/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "want POST", http.StatusMethodNotAllowed)
			return
		}
		resp := samplePlaygroundResp(sessionID)
		resp.Bearer = anonBearer
		writePlaygroundJSON(w, resp)
	})
	// Catch-all: detect any attempt to abandon/delete the session on push failure.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete || strings.Contains(r.URL.Path, "abandon") {
			abandonCalled = true
		}
		http.NotFound(w, r)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := setupPlaygroundEnv(t, srv.URL)

	// Inject a simulated push failure; all other git ops succeed.
	stubGitForNew(t, fmt.Errorf("simulated push failure: connection reset"))

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "new", "--playground"})
	if err == nil {
		t.Fatal("expected error on playground push failure, got nil")
	}

	// --- Error shape assertions ---

	// Must carry the playground-specific sentinel phrase from wrapPlaygroundPushError.
	if !strings.Contains(err.Error(), "playground session") {
		t.Errorf("error should identify a playground session, got: %v", err)
	}
	if !strings.Contains(err.Error(), "base_sha: null") {
		t.Errorf("error should mention base_sha: null to describe the live session state, got: %v", err)
	}
	if !strings.Contains(err.Error(), sessionID) {
		t.Errorf("error should include sessionID %q, got: %v", sessionID, err)
	}

	// Must include a ready-to-run retry command.
	// The portal's git smart-HTTP route is /git/{orgID}/{sessionID}.git/...
	// (see internal/portal/githttp/handler.go:90); playground sessions live
	// under the reserved "org_playground" org.
	expectedRemoteURL := strings.TrimRight(srv.URL, "/") + "/git/org_playground/" + sessionID + ".git"
	expectedRefspec := "refs/heads/jam/" + sessionID + "/base"
	if !strings.Contains(err.Error(), "git push") {
		t.Errorf("error should include a git push retry command, got: %v", err)
	}
	if !strings.Contains(err.Error(), expectedRemoteURL) {
		t.Errorf("retry command should include remote URL %q, got: %v", expectedRemoteURL, err)
	}
	if !strings.Contains(err.Error(), expectedRefspec) {
		t.Errorf("retry command should include refspec %q, got: %v", expectedRefspec, err)
	}

	// --- Session-stays-live assertion ---

	if abandonCalled {
		t.Error("session must NOT be abandoned on push failure (stays live per locked decision)")
	}

	// --- Per-session state assertions ---

	// The bearer token is written before the push attempt and must survive the failure,
	// so the user can authenticate their manual retry without re-creating the session.
	tokenPath := filepath.Join(dir, "sessions", sessionID, "token")
	tokenData, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("session token must be written before push (for retry auth), but reading %q failed: %v", tokenPath, err)
	}
	if string(tokenData) != anonBearer {
		t.Errorf("session token = %q, want %q", string(tokenData), anonBearer)
	}

	// org_id and ref are written by writePlaygroundSessionState, which is only
	// reached after a successful push.  They are absent after a push failure —
	// this is intentional: the retry push does not require them, and they will
	// be written if the user retries successfully via a follow-up jamsesh
	// command.  Asserting their absence here makes the behaviour explicit and
	// will catch any accidental reordering that silently writes them early.
	orgIDPath := filepath.Join(dir, "sessions", sessionID, "org_id")
	if _, statErr := os.Stat(orgIDPath); statErr == nil {
		t.Errorf("org_id state file should NOT be written on push failure (writePlaygroundSessionState not reached); found at %q", orgIDPath)
	}
	refPath := filepath.Join(dir, "sessions", sessionID, "ref")
	if _, statErr := os.Stat(refPath); statErr == nil {
		t.Errorf("ref state file should NOT be written on push failure (writePlaygroundSessionState not reached); found at %q", refPath)
	}
}

// TestPlaygroundAction_pushUsesBearerNotOAuthToken verifies that the push
// injects the just-received bearer (not the account-wide OAuth token) via
// the -c http.extraHeader argument. The bearer is Base64-encoded inside
// the Basic auth value: "Authorization: Basic base64(jamsesh:<bearer>)".
func TestPlaygroundAction_pushUsesBearerNotOAuthToken(t *testing.T) {
	const (
		sessionID  = "sess-pg-003"
		anonBearer = "anon-bearer-xyz789"
		oauthToken = "oauth-tok-should-not-appear"
	)

	// Expected Basic auth credential: "jamsesh:<bearer>" Base64-encoded.
	expectedB64 := base64.StdEncoding.EncodeToString([]byte("jamsesh:" + anonBearer))

	mux := http.NewServeMux()
	mux.HandleFunc("/api/playground/sessions", func(w http.ResponseWriter, r *http.Request) {
		resp := samplePlaygroundResp(sessionID)
		resp.Bearer = anonBearer
		writePlaygroundJSON(w, resp)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Set up env WITHOUT an OAuth token — playground must not need one.
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)

	_ = os.Chmod(dir, 0o700)
	t.Setenv("JAMSESH_PORTAL_URL", srv.URL)

	// Capture all git-with-env calls so we can inspect the extraHeader arg.
	var capturedGitArgs [][]string

	origRunGit := runGit
	origRunGitOutput := runGitOutput
	origRunGitWithEnv := runGitWithEnv
	t.Cleanup(func() {
		runGit = origRunGit
		runGitOutput = origRunGitOutput
		runGitWithEnv = origRunGitWithEnv
	})
	runGit = func(args ...string) error { return nil }
	runGitOutput = func(args ...string) (string, error) { return "abc1234", nil }
	runGitWithEnv = func(env []string, args ...string) error {
		capturedGitArgs = append(capturedGitArgs, append([]string(nil), args...))
		return nil
	}

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "new", "--playground"})
	if err != nil {
		t.Fatalf("new --playground returned error: %v", err)
	}

	// Find the -c http.extraHeader=... argument in the push call.
	// The header is injected as Base64 Basic auth: "Authorization: Basic <b64>".
	foundBearerInPush := false
	for _, callArgs := range capturedGitArgs {
		for _, arg := range callArgs {
			if strings.Contains(arg, "http.extraHeader=") {
				// The header must contain the anon bearer's Base64 encoding.
				if strings.Contains(arg, expectedB64) {
					foundBearerInPush = true
				}
				// The OAuth token must NOT appear in any push header.
				oauthB64 := base64.StdEncoding.EncodeToString([]byte("jamsesh:" + oauthToken))
				if strings.Contains(arg, oauthB64) || strings.Contains(arg, oauthToken) {
					t.Errorf("OAuth token leaked into playground push header: %q", arg)
				}
			}
		}
	}
	if !foundBearerInPush {
		t.Errorf("expected anon bearer (b64: %q) in push extraHeader, git calls: %v",
			expectedB64, capturedGitArgs)
	}
}

// ---- --open flag tests ----

// stubOpenURL overrides the openURL seam for the duration of a test and
// returns a pointer to the captured URL slice. Restored via t.Cleanup.
// Do NOT use t.Parallel in tests that call this helper.
func stubOpenURL(t *testing.T) *[]string {
	t.Helper()
	var captured []string
	orig := openURL
	t.Cleanup(func() { openURL = orig })
	openURL = func(rawURL string) error {
		captured = append(captured, rawURL)
		return nil
	}
	return &captured
}

// TestNewAction_openFlagDurable verifies that --open causes openURL to be called
// with the durable session-view URL after a successful "jamsesh new --org X --open".
func TestNewAction_openFlagDurable(t *testing.T) {
	const (
		orgID     = "org-open-001"
		sessionID = "sess-open-001"
		accountID = "acct-open-001"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeSessionJSON(w, sampleSession(orgID, sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupNewEnv(t, srv.URL)
	stubGitForNew(t, nil)
	stubIsTTY(t, false)
	captured := stubOpenURL(t)

	app := buildCLIApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "new", "--org", orgID, "--open",
	}); err != nil {
		t.Fatalf("new returned error: %v", err)
	}

	expectedURL := srv.URL + "/orgs/" + orgID + "/sessions/" + sessionID
	if len(*captured) != 1 || (*captured)[0] != expectedURL {
		t.Errorf("openURL captured = %v, want [%q]", *captured, expectedURL)
	}
}

// TestNewAction_noOpenFlagDurable verifies that omitting --open does NOT call openURL.
func TestNewAction_noOpenFlagDurable(t *testing.T) {
	const (
		orgID     = "org-open-002"
		sessionID = "sess-open-002"
		accountID = "acct-open-002"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeSessionJSON(w, sampleSession(orgID, sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupNewEnv(t, srv.URL)
	stubGitForNew(t, nil)
	stubIsTTY(t, false)
	captured := stubOpenURL(t)

	app := buildCLIApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "new", "--org", orgID,
	}); err != nil {
		t.Fatalf("new returned error: %v", err)
	}

	if len(*captured) != 0 {
		t.Errorf("openURL should not be called without --open, got: %v", *captured)
	}
}

// TestNewAction_openFlagPlayground verifies that --open causes openURL to be
// called with the playground join URL after "jamsesh new --playground --open".
func TestNewAction_openFlagPlayground(t *testing.T) {
	const sessionID = "sess-pg-open-001"

	mux := http.NewServeMux()
	mux.HandleFunc("/api/playground/sessions", func(w http.ResponseWriter, r *http.Request) {
		writePlaygroundJSON(w, samplePlaygroundResp(sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupPlaygroundEnv(t, srv.URL)
	stubGitForNew(t, nil)
	captured := stubOpenURL(t)

	app := buildCLIApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "new", "--playground", "--open",
	}); err != nil {
		t.Fatalf("new --playground --open returned error: %v", err)
	}

	expectedURL := srv.URL + "/playground/s/" + sessionID + "/join"
	if len(*captured) != 1 || (*captured)[0] != expectedURL {
		t.Errorf("openURL captured = %v, want [%q]", *captured, expectedURL)
	}
}

// TestNewAction_noOpenFlagPlayground verifies that omitting --open on the
// playground path does NOT call openURL.
func TestNewAction_noOpenFlagPlayground(t *testing.T) {
	const sessionID = "sess-pg-open-002"

	mux := http.NewServeMux()
	mux.HandleFunc("/api/playground/sessions", func(w http.ResponseWriter, r *http.Request) {
		writePlaygroundJSON(w, samplePlaygroundResp(sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupPlaygroundEnv(t, srv.URL)
	stubGitForNew(t, nil)
	captured := stubOpenURL(t)

	app := buildCLIApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "new", "--playground",
	}); err != nil {
		t.Fatalf("new --playground returned error: %v", err)
	}

	if len(*captured) != 0 {
		t.Errorf("openURL should not be called without --open, got: %v", *captured)
	}
}

// TestNewAction_openFlagDurable_summaryUnchanged is a golden-string guard that
// verifies the summary output is identical after extracting sessionViewURL.
func TestNewAction_openFlagDurable_summaryUnchanged(t *testing.T) {
	const (
		orgID     = "org-open-003"
		sessionID = "sess-open-003"
		accountID = "acct-open-003"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeSessionJSON(w, sampleSession(orgID, sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupNewEnv(t, srv.URL)
	stubGitForNew(t, nil)
	stubIsTTY(t, false)
	stubOpenURL(t) // suppress real browser launch

	app := buildCLIApp()
	output := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{
			"jamsesh", "new", "--org", orgID,
		}); err != nil {
			t.Fatalf("new returned error: %v", err)
		}
	})

	expectedSessionURL := srv.URL + "/orgs/" + orgID + "/sessions/" + sessionID
	if !strings.Contains(output, expectedSessionURL) {
		t.Errorf("session URL %q not found in output:\n%s", expectedSessionURL, output)
	}
}

// TestNewAction_openFlagPlayground_summaryUnchanged is a golden-string guard
// that verifies the playground summary output is identical after extracting
// playgroundJoinURL.
func TestNewAction_openFlagPlayground_summaryUnchanged(t *testing.T) {
	const sessionID = "sess-pg-open-003"

	mux := http.NewServeMux()
	mux.HandleFunc("/api/playground/sessions", func(w http.ResponseWriter, r *http.Request) {
		writePlaygroundJSON(w, samplePlaygroundResp(sessionID))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupPlaygroundEnv(t, srv.URL)
	stubGitForNew(t, nil)
	stubOpenURL(t) // suppress real browser launch

	app := buildCLIApp()
	output := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{
			"jamsesh", "new", "--playground",
		}); err != nil {
			t.Fatalf("new --playground returned error: %v", err)
		}
	})

	expectedURL := srv.URL + "/playground/s/" + sessionID + "/join"
	if !strings.Contains(output, expectedURL) {
		t.Errorf("playground join URL %q not found in output:\n%s", expectedURL, output)
	}
}
