package sessioncmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jamsesh/internal/api/openapi"
)


func TestStatusAction_textOutput(t *testing.T) {
	const (
		orgID     = "org-st-001"
		sessionID = "sess-st-001"
		accountID = "acct-st-001"
	)

	mux := http.NewServeMux()

	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.Session{
			Id:          sessionID,
			Name:        "Status Test",
			Goal:        "Verify status command",
			OrgId:       orgID,
			DefaultMode: openapi.SessionDefaultModeSync,
		})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/refs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.RefListResponse{
			Refs: []openapi.Ref{
				{Ref: "jam/" + sessionID + "/" + accountID + "/main", Sha: "abc1234567890", Mode: openapi.RefModeSync},
			},
		})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/comments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.CommentListResponse{
			Items: []openapi.Comment{},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupStatusEnv(t, srv.URL, sessionID, orgID, "jam/"+sessionID+"/"+accountID+"/main")

	// Capture stdout.
	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "status"})

	pw.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(pr)
	output := buf.String()

	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}

	if !strings.Contains(output, "Status Test") {
		t.Errorf("output missing session name; got: %q", output)
	}
	if !strings.Contains(output, sessionID) {
		t.Errorf("output missing session ID; got: %q", output)
	}
	if !strings.Contains(output, "Verify status command") {
		t.Errorf("output missing goal; got: %q", output)
	}
	if !strings.Contains(output, "sync") {
		t.Errorf("output missing mode; got: %q", output)
	}
	if !strings.Contains(output, "abc1234") {
		t.Errorf("output missing short SHA; got: %q", output)
	}
}

func TestStatusAction_jsonOutput(t *testing.T) {
	const (
		orgID     = "org-st-002"
		sessionID = "sess-st-002"
		accountID = "acct-st-002"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.Session{
			Id:    sessionID,
			Name:  "JSON Test",
			Goal:  "Test JSON output",
			OrgId: orgID,
		})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/refs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.RefListResponse{Refs: []openapi.Ref{}})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/comments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.CommentListResponse{Items: []openapi.Comment{}})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupStatusEnv(t, srv.URL, sessionID, orgID, "jam/"+sessionID+"/"+accountID+"/main")

	// Capture stdout.
	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "status", "--json"})

	pw.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(pr)
	output := buf.String()

	if err != nil {
		t.Fatalf("status --json returned error: %v", err)
	}

	var out statusOutput
	if err := json.Unmarshal([]byte(output), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, output)
	}
	if out.SessionID != sessionID {
		t.Errorf("json session_id = %q, want %q", out.SessionID, sessionID)
	}
	if out.Name != "JSON Test" {
		t.Errorf("json name = %q, want %q", out.Name, "JSON Test")
	}
}

func TestStatusAction_commentsAddressedToMe(t *testing.T) {
	const (
		orgID     = "org-st-003"
		sessionID = "sess-st-003"
		accountID = "acct-st-003"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.Session{
			Id:    sessionID,
			Name:  "Comment Test",
			OrgId: orgID,
		})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/refs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.RefListResponse{Refs: []openapi.Ref{}})
	})
	var receivedAddressedTo string
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/comments", func(w http.ResponseWriter, r *http.Request) {
		receivedAddressedTo = r.URL.Query().Get("addressed_to")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.CommentListResponse{
			Items: []openapi.Comment{
				{
					Id:          "comment-001",
					Body:        "Please review this change",
					Kind:        openapi.CommentKindActionRequest,
					AddressedTo: "@" + accountID,
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupStatusEnv(t, srv.URL, sessionID, orgID, "")

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	app := buildCLIApp()
	err := app.Run(context.Background(), []string{"jamsesh", "status"})

	pw.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(pr)
	output := buf.String()

	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}

	// Verify the comments endpoint was called with addressed_to filter.
	if !strings.Contains(receivedAddressedTo, accountID) {
		t.Errorf("addressed_to filter missing account ID; got %q", receivedAddressedTo)
	}

	// Verify comment appears in output.
	if !strings.Contains(output, "comment-001") {
		t.Errorf("output missing comment ID; got: %q", output)
	}
}

func TestReadSessionState_readsRefAndOrgID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)

	const sessID = "sess-rs-001"
	sessDir := filepath.Join(dir, "sessions", sessID)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "ref"), []byte("jam/sess-rs-001/acc/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "org_id"), []byte("org-rs-001\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	orgID, yourRef := readSessionState(sessID)
	if orgID != "org-rs-001" {
		t.Errorf("orgID = %q, want org-rs-001", orgID)
	}
	if yourRef != "jam/sess-rs-001/acc/main" {
		t.Errorf("yourRef = %q, want jam/sess-rs-001/acc/main", yourRef)
	}
}

// setupStatusEnv creates a CLAUDE_PLUGIN_DATA dir with a pre-populated session
// state and sets JAMSESH_PORTAL_URL. The session is discoverable via the
// resolveSession() fallback (first dir in sessions/).
func setupStatusEnv(t *testing.T, srvURL, sessionID, orgID, yourRef string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
	t.Setenv("JAMSESH_PORTAL_URL", srvURL)
	// Unset the CC_SESSION_ID so resolveSession() uses the first-dir fallback.
	t.Setenv("CC_SESSION_ID", "")

	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("tok-test"), 0o600); err != nil {
		t.Fatalf("writing token: %v", err)
	}

	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatalf("creating session dir: %v", err)
	}
	if yourRef != "" {
		if err := os.WriteFile(filepath.Join(sessDir, "ref"), []byte(yourRef), 0o600); err != nil {
			t.Fatalf("writing ref: %v", err)
		}
	}
	if orgID != "" {
		if err := os.WriteFile(filepath.Join(sessDir, "org_id"), []byte(orgID), 0o600); err != nil {
			t.Fatalf("writing org_id: %v", err)
		}
	}
	return dir
}
