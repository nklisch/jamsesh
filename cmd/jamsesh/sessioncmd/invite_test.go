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
)

// setupInviteEnv creates a temp JAMSESH_DATA_DIR dir with a token file and
// points JAMSESH_PORTAL_URL at the given test server URL.
// It also writes a sessions/<sessionID>/org_id state file when orgID is non-empty.
// Returns the temp dir path.
func setupInviteEnv(t *testing.T, srvURL, sessionID, orgID string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", srvURL)
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("tok-invite-test"), 0o600); err != nil {
		t.Fatalf("writing token: %v", err)
	}
	if sessionID != "" && orgID != "" {
		sessDir := filepath.Join(dir, "sessions", sessionID)
		if err := os.MkdirAll(sessDir, 0o700); err != nil {
			t.Fatalf("creating session dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(sessDir, "org_id"), []byte(orgID), 0o600); err != nil {
			t.Fatalf("writing org_id: %v", err)
		}
	}
	return dir
}

// TestInviteAction_happyPath verifies that two emails both get POSTed to the
// org-scoped endpoint and receive a 201 each.
func TestInviteAction_happyPath(t *testing.T) {
	const (
		orgID     = "org-inv-001"
		sessionID = "sess-inv-001"
	)

	var postedEmails []string
	var capturedPaths []string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/invites",
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "want POST", http.StatusMethodNotAllowed)
				return
			}
			capturedPaths = append(capturedPaths, r.URL.Path)
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			postedEmails = append(postedEmails, body["email"])
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{}`)
		})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupInviteEnv(t, srv.URL, sessionID, orgID)

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{
		"jamsesh", "invite", sessionID, "a@x.com,b@y.com",
	})
	if err != nil {
		t.Fatalf("invite returned error: %v", err)
	}

	if len(postedEmails) != 2 {
		t.Errorf("expected 2 invite POSTs, got %d: %v", len(postedEmails), postedEmails)
	}
	for _, want := range []string{"a@x.com", "b@y.com"} {
		found := false
		for _, got := range postedEmails {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected invite for %q, not found in %v", want, postedEmails)
		}
	}

	// Verify the correct org-scoped path was used.
	expectedPath := "/api/orgs/" + orgID + "/sessions/" + sessionID + "/invites"
	for _, p := range capturedPaths {
		if p != expectedPath {
			t.Errorf("unexpected path %q, want %q", p, expectedPath)
		}
	}
}

// TestInviteAction_spaceAndCommaSeparated verifies that space-separated email
// args are treated the same as comma-separated ones.
func TestInviteAction_spaceAndCommaSeparated(t *testing.T) {
	const (
		orgID     = "org-inv-002"
		sessionID = "sess-inv-002"
	)

	var postedEmails []string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/invites",
		func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			postedEmails = append(postedEmails, body["email"])
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{}`)
		})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupInviteEnv(t, srv.URL, sessionID, orgID)

	app := buildCLIApp()
	// Three separate positional args (space-separated at the shell level).
	err := app.Run(context.Background(), []string{
		"jamsesh", "invite", sessionID, "a@x.com", "b@y.com", "c@z.com",
	})
	if err != nil {
		t.Fatalf("invite returned error: %v", err)
	}

	if len(postedEmails) != 3 {
		t.Errorf("expected 3 invite POSTs, got %d: %v", len(postedEmails), postedEmails)
	}
}

// TestInviteAction_partialFailure verifies that a partial failure reports
// the count and does not silently swallow failures.
func TestInviteAction_partialFailure(t *testing.T) {
	const (
		orgID     = "org-inv-003"
		sessionID = "sess-inv-003"
	)

	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/invites",
		func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount == 1 {
				// First invite succeeds.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				fmt.Fprintln(w, `{}`)
			} else {
				// Second invite fails.
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupInviteEnv(t, srv.URL, sessionID, orgID)

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{
		"jamsesh", "invite", sessionID, "a@x.com,b@y.com",
	})
	if err == nil {
		t.Fatal("expected error for partial failure, got nil")
	}

	// Error should mention "1 of 2 failed".
	if !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("error should report partial failure count, got: %v", err)
	}
}

// TestInviteAction_usageError verifies that missing args produce a usage error.
func TestInviteAction_usageError(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			name:    "no args at all",
			args:    []string{"jamsesh", "invite"},
			wantMsg: "usage:",
		},
		{
			name:    "session ID only, no emails",
			args:    []string{"jamsesh", "invite", "sess-abc"},
			wantMsg: "usage:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// No server needed; error fires before any network call.
			dir := t.TempDir()
			t.Setenv("JAMSESH_DATA_DIR", dir)
			t.Setenv("JAMSESH_PORTAL_URL", "http://localhost:19996")
			_ = os.WriteFile(filepath.Join(dir, "token"), []byte("tok"), 0o600)

			app := buildCLIApp()
			err := app.Run(context.Background(), tc.args)
			if err == nil {
				t.Fatal("expected usage error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.wantMsg) {
				t.Errorf("error should contain %q, got: %v", tc.wantMsg, err)
			}
		})
	}
}

// TestParseInviteEmails tests the parseInviteEmails helper with a table of inputs.
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
		// Mixed: space-joined input that inviteAction produces
		{"a@x.com,b@y.com,c@z.com", []string{"a@x.com", "b@y.com", "c@z.com"}},
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

// TestInviteAction_readsOrgFromState verifies that when --org is not provided
// but sessions/<id>/org_id exists, the correct org-scoped endpoint is called.
func TestInviteAction_readsOrgFromState(t *testing.T) {
	const (
		orgID     = "org-inv-004"
		sessionID = "sess-inv-004"
	)

	var capturedPath string
	mux := http.NewServeMux()
	// Register a wildcard handler to capture any invites path.
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/invites",
		func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{}`)
		})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// setupInviteEnv writes the org_id state file for sessionID.
	setupInviteEnv(t, srv.URL, sessionID, orgID)

	app := buildCLIApp()
	// No --org flag: must read org from state.
	err := app.Run(context.Background(), []string{
		"jamsesh", "invite", sessionID, "user@example.com",
	})
	if err != nil {
		t.Fatalf("invite returned error: %v", err)
	}

	expectedPath := "/api/orgs/" + orgID + "/sessions/" + sessionID + "/invites"
	if capturedPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, capturedPath)
	}
}

// TestInviteAction_missingOrgFails verifies that when neither --org flag nor
// session state file is present, a clear error is returned.
func TestInviteAction_missingOrgFails(t *testing.T) {
	const sessionID = "sess-inv-005-no-state"

	// No server needed; error fires before network.
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", "http://localhost:19995")
	_ = os.WriteFile(filepath.Join(dir, "token"), []byte("tok"), 0o600)
	// Deliberately do NOT write a sessions/<id>/org_id file.

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{
		"jamsesh", "invite", sessionID, "user@example.com",
	})
	if err == nil {
		t.Fatal("expected error when org is missing, got nil")
	}

	// Error should mention --org and/or the session ID.
	if !strings.Contains(err.Error(), "--org") {
		t.Errorf("error should mention --org flag, got: %v", err)
	}
	if !strings.Contains(err.Error(), sessionID) {
		t.Errorf("error should mention session ID %q, got: %v", sessionID, err)
	}
}
