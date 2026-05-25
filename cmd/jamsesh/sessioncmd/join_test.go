package sessioncmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"jamsesh/internal/api/openapi"
)

// setupJoinEnv creates a temp JAMSESH_DATA_DIR dir, writes a fake access
// token, and points JAMSESH_PORTAL_URL at the given test server URL.
func setupJoinEnv(t *testing.T, srvURL string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", srvURL)
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("tok-test"), 0o600); err != nil {
		t.Fatalf("writing token: %v", err)
	}
	return dir
}

// writeMeJSON writes a /api/me response as raw JSON to avoid
// openapi_types.Email marshal validation failures.
func writeMeJSON(w http.ResponseWriter, accountID string, orgIDs ...string) {
	w.Header().Set("Content-Type", "application/json")
	orgs := ""
	for i, id := range orgIDs {
		if i > 0 {
			orgs += ","
		}
		orgs += fmt.Sprintf(`{"id":%q,"name":"Org %s","role":"member","slug":"org"}`, id, id)
	}
	fmt.Fprintf(w, `{"id":%q,"display_name":"Test","email":"test@example.com","orgs":[%s]}`, accountID, orgs)
}

func TestParseSessionArg_bareID(t *testing.T) {
	sid, oid, iid, itok := parseSessionArg("sess-123", "https://portal.example.com")
	if sid != "sess-123" {
		t.Errorf("sessionID = %q, want %q", sid, "sess-123")
	}
	if oid != "" || iid != "" || itok != "" {
		t.Errorf("unexpected non-empty fields: orgID=%q inviteID=%q inviteToken=%q", oid, iid, itok)
	}
}

func TestParseSessionArg_orgSlash(t *testing.T) {
	sid, oid, iid, _ := parseSessionArg("org-abc/sess-xyz", "https://portal.example.com")
	if sid != "sess-xyz" {
		t.Errorf("sessionID = %q, want sess-xyz", sid)
	}
	if oid != "org-abc" {
		t.Errorf("orgID = %q, want org-abc", oid)
	}
	if iid != "" {
		t.Errorf("inviteID should be empty, got %q", iid)
	}
}

func TestParseSessionArg_inviteURL(t *testing.T) {
	raw := "https://portal.example.com/join?org=org1&session=sess1&invite=inv1&token=tok1"
	sid, oid, iid, itok := parseSessionArg(raw, "https://portal.example.com")
	if sid != "sess1" {
		t.Errorf("sessionID = %q, want sess1", sid)
	}
	if oid != "org1" {
		t.Errorf("orgID = %q, want org1", oid)
	}
	if iid != "inv1" {
		t.Errorf("inviteID = %q, want inv1", iid)
	}
	if itok != "tok1" {
		t.Errorf("inviteToken = %q, want tok1", itok)
	}
}

func TestJoinAction_happy(t *testing.T) {
	const (
		orgID     = "org-001"
		sessionID = "sess-001"
		accountID = "acct-001"
	)

	// Track git calls.
	var gitCalls [][]string
	origRunGit := runGit
	origRunGitOutput := runGitOutput
	t.Cleanup(func() {
		runGit = origRunGit
		runGitOutput = origRunGitOutput
	})
	runGit = func(args ...string) error {
		gitCalls = append(gitCalls, append([]string(nil), args...))
		return nil
	}
	runGitOutput = func(args ...string) (string, error) {
		return "", nil
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})

	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID, func(w http.ResponseWriter, r *http.Request) {
		resp := openapi.Session{
			Id:          sessionID,
			Name:        "Test Session",
			Goal:        "Build something",
			OrgId:       orgID,
			DefaultMode: openapi.SessionDefaultModeSync,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := setupJoinEnv(t, srv.URL)

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "join", orgID + "/" + sessionID})
	if err != nil {
		t.Fatalf("join returned error: %v", err)
	}

	// Verify git clone was called.
	foundClone := false
	for _, call := range gitCalls {
		if len(call) > 0 && call[0] == "clone" {
			foundClone = true
			break
		}
	}
	if !foundClone {
		t.Errorf("expected git clone call, got: %v", gitCalls)
	}

	// Verify per-session state was written.
	refPath := filepath.Join(dir, "sessions", sessionID, "ref")
	data, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("reading ref state: %v", err)
	}
	expectedRef := "jam/" + sessionID + "/" + accountID + "/main"
	if string(data) != expectedRef {
		t.Errorf("ref state = %q, want %q", data, expectedRef)
	}

	instancePath := filepath.Join(dir, "sessions", sessionID, "instance_id")
	if _, err := os.ReadFile(instancePath); err != nil {
		t.Errorf("instance_id state not written: %v", err)
	}

	orgIDPath := filepath.Join(dir, "sessions", sessionID, "org_id")
	orgIDData, err := os.ReadFile(orgIDPath)
	if err != nil {
		t.Fatalf("reading org_id state: %v", err)
	}
	if string(orgIDData) != orgID {
		t.Errorf("org_id state = %q, want %q", orgIDData, orgID)
	}
}

func TestJoinAction_inviteURL(t *testing.T) {
	const (
		orgID     = "org-002"
		sessionID = "sess-002"
		inviteID  = "inv-002"
		invToken  = "tkn-002"
		accountID = "acct-002"
	)

	origRunGit := runGit
	origRunGitOutput := runGitOutput
	t.Cleanup(func() {
		runGit = origRunGit
		runGitOutput = origRunGitOutput
	})
	runGit = func(args ...string) error { return nil }
	runGitOutput = func(args ...string) (string, error) { return "", nil }

	var inviteAccepted bool

	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/invites/"+inviteID+"/accept",
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "want POST", http.StatusMethodNotAllowed)
				return
			}
			inviteAccepted = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.Session{
			Id:    sessionID,
			Name:  "Invite Session",
			Goal:  "Test invites",
			OrgId: orgID,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupJoinEnv(t, srv.URL)

	inviteURL := srv.URL + "/join?org=" + orgID + "&session=" + sessionID +
		"&invite=" + inviteID + "&token=" + invToken

	app := buildCLIApp()
	if err := app.Run(context.Background(), []string{"jamsesh", "join", inviteURL}); err != nil {
		t.Fatalf("join returned error: %v", err)
	}

	if !inviteAccepted {
		t.Error("invite accept endpoint was not called")
	}
}

func TestJoinAction_missingArg(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", "http://localhost:19999")
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("tok"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "join"})
	if err == nil {
		t.Error("expected error when no arg given, got nil")
	}
}
