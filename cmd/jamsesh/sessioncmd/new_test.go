package sessioncmd

import (
	"context"
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

// setupNewEnv creates a temp CLAUDE_PLUGIN_DATA dir, writes a fake access
// token, and points JAMSESH_PORTAL_URL at the given test server URL.
// It returns the temp dir path.
func setupNewEnv(t *testing.T, srvURL string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
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
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
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

// TestParseInviteEmails tests the parseInviteEmails helper.
func TestParseInviteEmails(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a@x.com", []string{"a@x.com"}},
		{"a@x.com,b@y.com", []string{"a@x.com", "b@y.com"}},
		{"a@x.com, b@y.com , c@z.com", []string{"a@x.com", "b@y.com", "c@z.com"}},
		{"a@x.com,,b@y.com", []string{"a@x.com", "b@y.com"}},
		{"", []string{}},
		{"  ,  ", []string{}},
	}

	for _, tc := range tests {
		got := parseInviteEmails(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("parseInviteEmails(%q) = %v (len %d), want %v (len %d)",
				tc.input, got, len(got), tc.want, len(tc.want))
			continue
		}
		for i, e := range tc.want {
			if got[i] != e {
				t.Errorf("parseInviteEmails(%q)[%d] = %q, want %q", tc.input, i, got[i], e)
			}
		}
	}
}
