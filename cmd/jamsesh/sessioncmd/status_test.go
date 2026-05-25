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
	"time"

	"jamsesh/internal/api/openapi"
)

// setupDurableSession creates the per-session state for a single durable session
// under JAMSESH_DATA_DIR. It writes the per-session token, org_id, and
// optionally ref. Returns the session dir.
func setupDurableSession(t *testing.T, dir, sessionID, orgID, yourRef, token string) {
	t.Helper()
	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatalf("creating session dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "token"), []byte(token), 0o600); err != nil {
		t.Fatalf("writing per-session token: %v", err)
	}
	if orgID != "" {
		if err := os.WriteFile(filepath.Join(sessDir, "org_id"), []byte(orgID), 0o600); err != nil {
			t.Fatalf("writing org_id: %v", err)
		}
	}
	if yourRef != "" {
		if err := os.WriteFile(filepath.Join(sessDir, "ref"), []byte(yourRef), 0o600); err != nil {
			t.Fatalf("writing ref: %v", err)
		}
	}
}

// setupPlaygroundSession creates the per-session state for a playground session.
func setupPlaygroundSession(t *testing.T, dir, sessionID, nickname, token string) {
	t.Helper()
	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatalf("creating session dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "token"), []byte(token), 0o600); err != nil {
		t.Fatalf("writing per-session token: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "org_id"), []byte(playgroundOrgID), 0o600); err != nil {
		t.Fatalf("writing org_id (playground): %v", err)
	}
	if nickname != "" {
		if err := os.WriteFile(filepath.Join(sessDir, "nickname"), []byte(nickname), 0o600); err != nil {
			t.Fatalf("writing nickname: %v", err)
		}
	}
}

// captureStdout runs f and returns everything it writes to os.Stdout as a string.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	oldStdout := os.Stdout
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = pw

	f()

	pw.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(pr)
	return buf.String()
}

// captureStderr runs f and returns everything it writes to os.Stderr.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	oldStderr := os.Stderr
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = pw

	f()

	pw.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(pr)
	return buf.String()
}

// TestStatusAction_durableSession verifies that a single durable session is
// correctly fetched and displayed.
func TestStatusAction_durableSession(t *testing.T) {
	const (
		orgID     = "org-st-001"
		sessionID = "sess-st-001"
		token     = "tok-durable-001"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.Session{
			Id:          sessionID,
			Name:        "Status Test",
			Goal:        "Verify status command",
			OrgId:       orgID,
			DefaultMode: openapi.SessionDefaultModeSync,
			Members:     []openapi.MemberSummary{{AccountId: "acct-001", Role: "member"}},
		})
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID+"/refs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.RefListResponse{
			Refs: []openapi.Ref{
				{Ref: "jam/" + sessionID + "/acct-001/main", Sha: "abc1234567890", Mode: openapi.RefModeSync},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", srv.URL)

	setupDurableSession(t, dir, sessionID, orgID, "jam/"+sessionID+"/acct-001/main", token)

	app := buildCLIApp()
	var output string
	var runErr error
	output = captureStdout(t, func() {
		runErr = app.Run(context.Background(), []string{"jamsesh", "status"})
	})

	if runErr != nil {
		t.Fatalf("status returned error: %v", runErr)
	}

	if !strings.Contains(output, "Durable sessions") {
		t.Errorf("output missing durable section header; got: %q", output)
	}
	if !strings.Contains(output, sessionID) {
		t.Errorf("output missing session ID; got: %q", output)
	}
	if !strings.Contains(output, orgID) {
		t.Errorf("output missing org ID; got: %q", output)
	}
}

// TestStatusAction_playgroundSession verifies that a playground session is
// correctly fetched from the playground endpoint and displayed.
func TestStatusAction_playgroundSession(t *testing.T) {
	const (
		sessionID = "sess-pg-001"
		nickname  = "amber-otter"
		token     = "tok-playground-001"
	)

	hardCap := time.Now().Add(23*time.Hour + 12*time.Minute)
	idle := time.Now().Add(30 * time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/playground/sessions/"+sessionID, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.PlaygroundSessionSummary{
			Id:            sessionID,
			Name:          "playground-" + sessionID,
			OrgId:         playgroundOrgID,
			MembersCount:  2,
			HardCapAt:     hardCap,
			IdleTimeoutAt: idle,
			Status:        openapi.PlaygroundSessionSummaryStatusActive,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", srv.URL)

	setupPlaygroundSession(t, dir, sessionID, nickname, token)

	app := buildCLIApp()
	var output string
	var runErr error
	output = captureStdout(t, func() {
		runErr = app.Run(context.Background(), []string{"jamsesh", "status"})
	})

	if runErr != nil {
		t.Fatalf("status returned error: %v", runErr)
	}

	if !strings.Contains(output, "Playground sessions") {
		t.Errorf("output missing playground section header; got: %q", output)
	}
	if !strings.Contains(output, sessionID) {
		t.Errorf("output missing session ID; got: %q", output)
	}
	if !strings.Contains(output, nickname) {
		t.Errorf("output missing nickname %q; got: %q", nickname, output)
	}
	// Should show some "Ends in" duration (hours or minutes).
	if !strings.Contains(output, "Ends in:") {
		t.Errorf("output missing 'Ends in:' duration; got: %q", output)
	}
}

// TestStatusAction_playgroundSession_noNicknameSidecar verifies backward-compat
// behaviour: a playground session that was created before the nickname sidecar
// feature (i.e. the "nickname" file is absent) must still exit 0, render the
// playground row in the output, and produce no stray error about the missing
// file.  This is the trip-wire that catches any future regression where
// readNickname starts fataling on a missing sidecar instead of returning "".
func TestStatusAction_playgroundSession_noNicknameSidecar(t *testing.T) {
	const (
		sessionID = "sess-pg-nonick"
		token     = "tok-playground-nonick"
	)

	hardCap := time.Now().Add(23*time.Hour + 12*time.Minute)
	idle := time.Now().Add(30 * time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/playground/sessions/"+sessionID, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.PlaygroundSessionSummary{
			Id:            sessionID,
			Name:          "playground-" + sessionID,
			OrgId:         playgroundOrgID,
			MembersCount:  1,
			HardCapAt:     hardCap,
			IdleTimeoutAt: idle,
			Status:        openapi.PlaygroundSessionSummaryStatusActive,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", srv.URL)

	// Pass empty nickname — setupPlaygroundSession skips the sidecar write,
	// simulating a pre-fix session loaded from disk.
	setupPlaygroundSession(t, dir, sessionID, "", token)

	app := buildCLIApp()
	var output string
	var stderrOutput string
	var runErr error
	output = captureStdout(t, func() {
		stderrOutput = captureStderr(t, func() {
			runErr = app.Run(context.Background(), []string{"jamsesh", "status"})
		})
	})

	if runErr != nil {
		t.Fatalf("status returned error with no nickname sidecar: %v", runErr)
	}

	if !strings.Contains(output, "Playground sessions") {
		t.Errorf("output missing playground section header; got: %q", output)
	}
	if !strings.Contains(output, sessionID) {
		t.Errorf("output missing session ID; got: %q", output)
	}
	// Should show some "Ends in" duration even without a nickname.
	if !strings.Contains(output, "Ends in:") {
		t.Errorf("output missing 'Ends in:' duration; got: %q", output)
	}
	// The nickname column must render without crashing — an empty string is
	// the correct backward-compat value, so there must be no "nickname" error
	// logged to stderr.
	if strings.Contains(stderrOutput, "nickname") {
		t.Errorf("unexpected stderr mention of 'nickname'; got: %q", stderrOutput)
	}
	// Confirm no stray warning about a missing sidecar was emitted at all.
	if stderrOutput != "" {
		t.Errorf("expected clean stderr with no warnings; got: %q", stderrOutput)
	}
}

// TestStatusAction_mixedSessions verifies that durable and playground sessions
// are grouped separately in the output.
func TestStatusAction_mixedSessions(t *testing.T) {
	const (
		durableSessionID  = "sess-d-001"
		durableOrgID      = "org-d-001"
		durableToken      = "tok-d-001"
		pgSessionID       = "sess-pg-002"
		pgNickname        = "quiet-fox"
		pgToken           = "tok-pg-002"
	)

	hardCap := time.Now().Add(10 * time.Hour)
	idle := time.Now().Add(20 * time.Minute)

	mux := http.NewServeMux()

	// Durable session endpoint.
	mux.HandleFunc("/api/orgs/"+durableOrgID+"/sessions/"+durableSessionID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.Session{
			Id:          durableSessionID,
			Name:        "Durable Jam",
			OrgId:       durableOrgID,
			DefaultMode: openapi.SessionDefaultModeSync,
		})
	})
	mux.HandleFunc("/api/orgs/"+durableOrgID+"/sessions/"+durableSessionID+"/refs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.RefListResponse{Refs: []openapi.Ref{}})
	})

	// Playground session endpoint.
	mux.HandleFunc("/api/playground/sessions/"+pgSessionID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.PlaygroundSessionSummary{
			Id:            pgSessionID,
			Name:          "playground-" + pgSessionID,
			OrgId:         playgroundOrgID,
			MembersCount:  2,
			HardCapAt:     hardCap,
			IdleTimeoutAt: idle,
			Status:        openapi.PlaygroundSessionSummaryStatusActive,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", srv.URL)

	setupDurableSession(t, dir, durableSessionID, durableOrgID, "jam/"+durableSessionID+"/acct/main", durableToken)
	setupPlaygroundSession(t, dir, pgSessionID, pgNickname, pgToken)

	app := buildCLIApp()
	var output string
	var runErr error
	output = captureStdout(t, func() {
		runErr = app.Run(context.Background(), []string{"jamsesh", "status"})
	})

	if runErr != nil {
		t.Fatalf("status returned error: %v", runErr)
	}

	if !strings.Contains(output, "Durable sessions") {
		t.Errorf("output missing durable section header; got: %q", output)
	}
	if !strings.Contains(output, "Playground sessions") {
		t.Errorf("output missing playground section header; got: %q", output)
	}
	if !strings.Contains(output, durableSessionID) {
		t.Errorf("output missing durable session ID; got: %q", output)
	}
	if !strings.Contains(output, pgSessionID) {
		t.Errorf("output missing playground session ID; got: %q", output)
	}
	if !strings.Contains(output, pgNickname) {
		t.Errorf("output missing playground nickname; got: %q", output)
	}

	// Durable section should appear before playground section.
	durableIdx := strings.Index(output, "Durable")
	pgIdx := strings.Index(output, "Playground")
	if durableIdx >= pgIdx {
		t.Errorf("expected Durable section before Playground section; got:\n%s", output)
	}
}

// TestStatusAction_missingToken verifies that a session with no per-session token
// is skipped with a stderr warning; the command exits 0.
func TestStatusAction_missingToken(t *testing.T) {
	const sessionID = "sess-notoken"

	// Session dir exists but has no token file.
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", "http://localhost:0") // unreachable; should not be hit

	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Write org_id so we get past the "no sessions" path.
	if err := os.WriteFile(filepath.Join(sessDir, "org_id"), []byte("org-test"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Do NOT write a token file.

	app := buildCLIApp()
	var stderrOutput string
	var runErr error
	captureStdout(t, func() {
		stderrOutput = captureStderr(t, func() {
			runErr = app.Run(context.Background(), []string{"jamsesh", "status"})
		})
	})

	if runErr != nil {
		t.Fatalf("status should exit 0 on missing token; got: %v", runErr)
	}
	if !strings.Contains(stderrOutput, "no token") {
		t.Errorf("expected stderr warning about missing token; got: %q", stderrOutput)
	}
}

// TestStatusAction_noSessions verifies the friendly "no sessions" message.
func TestStatusAction_noSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", "http://localhost:0")

	// Create an empty sessions directory.
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}

	app := buildCLIApp()
	var output string
	var runErr error
	output = captureStdout(t, func() {
		runErr = app.Run(context.Background(), []string{"jamsesh", "status"})
	})

	if runErr != nil {
		t.Fatalf("status returned error on no sessions: %v", runErr)
	}
	if !strings.Contains(output, "No sessions") {
		t.Errorf("expected 'No sessions' message; got: %q", output)
	}
	if !strings.Contains(output, "jamsesh:jam") {
		t.Errorf("expected '/jamsesh:jam' pointer; got: %q", output)
	}
}

// TestStatusAction_jsonOutput verifies the --json top-level shape: {"durable":[...],"playground":[...]}.
func TestStatusAction_jsonOutput(t *testing.T) {
	const (
		orgID     = "org-st-json"
		sessionID = "sess-st-json"
		token     = "tok-json-test"
	)

	mux := http.NewServeMux()
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

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", srv.URL)

	setupDurableSession(t, dir, sessionID, orgID, "jam/"+sessionID+"/acct/main", token)

	app := buildCLIApp()
	var output string
	var runErr error
	output = captureStdout(t, func() {
		runErr = app.Run(context.Background(), []string{"jamsesh", "status", "--json"})
	})

	if runErr != nil {
		t.Fatalf("status --json returned error: %v", runErr)
	}

	var out statusJSONOutput
	if err := json.Unmarshal([]byte(output), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, output)
	}
	if len(out.Durable) != 1 {
		t.Fatalf("expected 1 durable session, got %d", len(out.Durable))
	}
	if out.Durable[0].SessionID != sessionID {
		t.Errorf("json durable[0].session_id = %q, want %q", out.Durable[0].SessionID, sessionID)
	}
	if out.Durable[0].Name != "JSON Test" {
		t.Errorf("json durable[0].name = %q, want %q", out.Durable[0].Name, "JSON Test")
	}
	if out.Playground == nil {
		t.Errorf("json playground should be an array (not null), even when empty")
	}
}

// TestStatusAction_jsonOutputMixedSessions verifies that --json includes both
// durable and playground arrays correctly.
func TestStatusAction_jsonOutputMixedSessions(t *testing.T) {
	const (
		durableSessionID = "sess-d-json"
		durableOrgID     = "org-d-json"
		durableToken     = "tok-d-json"
		pgSessionID      = "sess-pg-json"
		pgToken          = "tok-pg-json"
	)

	hardCap := time.Now().Add(5 * time.Hour)
	idle := time.Now().Add(20 * time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/orgs/"+durableOrgID+"/sessions/"+durableSessionID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.Session{
			Id: durableSessionID, Name: "Durable JSON", OrgId: durableOrgID,
		})
	})
	mux.HandleFunc("/api/orgs/"+durableOrgID+"/sessions/"+durableSessionID+"/refs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.RefListResponse{Refs: []openapi.Ref{}})
	})
	mux.HandleFunc("/api/playground/sessions/"+pgSessionID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.PlaygroundSessionSummary{
			Id: pgSessionID, OrgId: playgroundOrgID, MembersCount: 1,
			HardCapAt: hardCap, IdleTimeoutAt: idle,
			Status: openapi.PlaygroundSessionSummaryStatusActive,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", srv.URL)

	setupDurableSession(t, dir, durableSessionID, durableOrgID, "", durableToken)
	setupPlaygroundSession(t, dir, pgSessionID, "bright-otter", pgToken)

	app := buildCLIApp()
	var output string
	var runErr error
	output = captureStdout(t, func() {
		runErr = app.Run(context.Background(), []string{"jamsesh", "status", "--json"})
	})

	if runErr != nil {
		t.Fatalf("status --json returned error: %v", runErr)
	}

	var out statusJSONOutput
	if err := json.Unmarshal([]byte(output), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, output)
	}
	if len(out.Durable) != 1 {
		t.Errorf("expected 1 durable session, got %d", len(out.Durable))
	}
	if len(out.Playground) != 1 {
		t.Errorf("expected 1 playground session, got %d", len(out.Playground))
	}
	if len(out.Durable) > 0 && out.Durable[0].SessionID != durableSessionID {
		t.Errorf("durable session_id = %q, want %q", out.Durable[0].SessionID, durableSessionID)
	}
	if len(out.Playground) > 0 && out.Playground[0].SessionID != pgSessionID {
		t.Errorf("playground session_id = %q, want %q", out.Playground[0].SessionID, pgSessionID)
	}
}

// TestStatusAction_noSessionsJSON verifies --json with no sessions returns empty arrays.
func TestStatusAction_noSessionsJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	t.Setenv("JAMSESH_PORTAL_URL", "http://localhost:0")

	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}

	app := buildCLIApp()
	var output string
	var runErr error
	output = captureStdout(t, func() {
		runErr = app.Run(context.Background(), []string{"jamsesh", "status", "--json"})
	})

	if runErr != nil {
		t.Fatalf("status --json returned error on no sessions: %v", runErr)
	}

	var out statusJSONOutput
	if err := json.Unmarshal([]byte(output), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, output)
	}
	if out.Durable == nil {
		t.Errorf("json durable should be empty array, not null")
	}
	if out.Playground == nil {
		t.Errorf("json playground should be empty array, not null")
	}
	if len(out.Durable) != 0 || len(out.Playground) != 0 {
		t.Errorf("expected empty arrays; got durable=%d playground=%d", len(out.Durable), len(out.Playground))
	}
}

// TestReadSessionState_readsRefAndOrgID verifies that readSessionState reads
// both the org_id and ref sidecar files.
func TestReadSessionState_readsRefAndOrgID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)

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

// setupStatusEnv writes both the legacy token file and a per-session token
// into a temp JAMSESH_DATA_DIR dir so status tests cover both lookup paths.
func setupStatusEnv(t *testing.T, srvURL, sessionID, orgID, yourRef string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
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
	// Write per-session token so new enumeration path works.
	if err := os.WriteFile(filepath.Join(sessDir, "token"), []byte("tok-test"), 0o600); err != nil {
		t.Fatalf("writing per-session token: %v", err)
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

// TestStatusAction_textOutput verifies the grouped text output for a durable session.
// Uses setupStatusEnv for compatibility with the existing mock-server pattern.
func TestStatusAction_textOutput(t *testing.T) {
	const (
		orgID     = "org-st-001"
		sessionID = "sess-st-001"
		accountID = "acct-st-001"
	)

	mux := http.NewServeMux()

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

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupStatusEnv(t, srv.URL, sessionID, orgID, "jam/"+sessionID+"/"+accountID+"/main")

	app := buildCLIApp()
	var output string
	var err error
	output = captureStdout(t, func() {
		err = app.Run(context.Background(), []string{"jamsesh", "status"})
	})

	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}

	if !strings.Contains(output, "Durable sessions") {
		t.Errorf("output missing durable section header; got: %q", output)
	}
	if !strings.Contains(output, sessionID) {
		t.Errorf("output missing session ID; got: %q", output)
	}
	if !strings.Contains(output, orgID) {
		t.Errorf("output missing org ID; got: %q", output)
	}
}

// TestStatusAction_jsonOutputLegacy verifies that the new --json shape contains
// the session_id field within the durable array entry (backward compat).
func TestStatusAction_jsonOutputLegacy(t *testing.T) {
	const (
		orgID     = "org-st-002"
		sessionID = "sess-st-002"
		accountID = "acct-st-002"
	)

	mux := http.NewServeMux()
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

	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupStatusEnv(t, srv.URL, sessionID, orgID, "jam/"+sessionID+"/"+accountID+"/main")

	app := buildCLIApp()
	var output string
	var err error
	output = captureStdout(t, func() {
		err = app.Run(context.Background(), []string{"jamsesh", "status", "--json"})
	})

	if err != nil {
		t.Fatalf("status --json returned error: %v", err)
	}

	var out statusJSONOutput
	if err := json.Unmarshal([]byte(output), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, output)
	}
	if len(out.Durable) != 1 {
		t.Fatalf("expected 1 durable session, got %d; output: %q", len(out.Durable), output)
	}
	if out.Durable[0].SessionID != sessionID {
		t.Errorf("json durable[0].session_id = %q, want %q", out.Durable[0].SessionID, sessionID)
	}
	if out.Durable[0].Name != "JSON Test" {
		t.Errorf("json durable[0].name = %q, want %q", out.Durable[0].Name, "JSON Test")
	}
}
