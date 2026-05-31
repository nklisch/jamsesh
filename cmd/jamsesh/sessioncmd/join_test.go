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

	"jamsesh/internal/api/openapi"
)

// setupJoinEnv creates a temp JAMSESH_DATA_DIR dir, writes a fake access
// token, and points JAMSESH_PORTAL_URL at the given test server URL.
func setupJoinEnv(t *testing.T, srvURL string) string {
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
	var gitWithEnvCalls [][]string
	var gitWithEnvs [][]string
	origRunGit := runGit
	origRunGitOutput := runGitOutput
	origRunGitWithEnv := runGitWithEnv
	t.Cleanup(func() {
		runGit = origRunGit
		runGitOutput = origRunGitOutput
		runGitWithEnv = origRunGitWithEnv
	})
	runGit = func(args ...string) error {
		gitCalls = append(gitCalls, append([]string(nil), args...))
		return nil
	}
	runGitOutput = func(args ...string) (string, error) {
		return "", nil
	}
	runGitWithEnv = func(env []string, args ...string) error {
		gitWithEnvCalls = append(gitWithEnvCalls, append([]string(nil), args...))
		gitWithEnvs = append(gitWithEnvs, append([]string(nil), env...))
		return nil
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

	// Verify git clone was called via runGitWithEnv (extraHeader env path).
	// The clone command is invoked with credential config in GIT_CONFIG_* env.
	var cloneCall []string
	var cloneEnv []string
	for idx, call := range gitWithEnvCalls {
		for _, a := range call {
			if a == "clone" {
				cloneCall = call
				cloneEnv = gitWithEnvs[idx]
				break
			}
		}
		if cloneCall != nil {
			break
		}
	}
	if cloneCall == nil {
		t.Fatalf("expected git clone call via runGitWithEnv, got runGit=%v runGitWithEnv=%v",
			gitCalls, gitWithEnvCalls)
	}

	// Token must NOT appear in any positional arg (process listing safety).
	// The clone URL must be credential-less, and the bearer must flow via the
	// GIT_CONFIG_* environment channel as http.extraHeader.
	// Assert the clone URL itself carries no `userinfo` segment.
	for i, a := range cloneCall {
		if a == "clone" {
			// Next positional after possible flags should be the URL.
			// Walk forward to first non-flag arg.
			for j := i + 1; j < len(cloneCall); j++ {
				if strings.HasPrefix(cloneCall[j], "-") {
					continue
				}
				if cloneCall[j] == "--bare" {
					continue
				}
				if strings.Contains(cloneCall[j], "@") &&
					(strings.HasPrefix(cloneCall[j], "http://") || strings.HasPrefix(cloneCall[j], "https://")) {
					t.Errorf("clone URL contains userinfo: %q (token leak via URL)", cloneCall[j])
				}
				break
			}
		}
	}
	// The bearer token "tok-test" and its Basic header must not appear
	// anywhere in argv.
	expectedHeader := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:tok-test"))
	for _, a := range cloneCall {
		if strings.Contains(a, "tok-test") || strings.Contains(a, expectedHeader) || strings.Contains(a, "http.extraHeader") {
			t.Errorf("bearer token leaked into argv: %q", a)
		}
	}
	if got := envLookup(cloneEnv, "GIT_CONFIG_KEY_0"); got != "http.extraHeader" {
		t.Errorf("GIT_CONFIG_KEY_0 = %q, want http.extraHeader; env=%v", got, cloneEnv)
	}
	if got := envLookup(cloneEnv, "GIT_CONFIG_VALUE_0"); got != expectedHeader {
		t.Errorf("GIT_CONFIG_VALUE_0 = %q, want %q", got, expectedHeader)
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
	origRunGitWithEnv := runGitWithEnv
	t.Cleanup(func() {
		runGit = origRunGit
		runGitOutput = origRunGitOutput
		runGitWithEnv = origRunGitWithEnv
	})
	runGit = func(args ...string) error { return nil }
	runGitOutput = func(args ...string) (string, error) { return "", nil }
	runGitWithEnv = func(env []string, args ...string) error { return nil }

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

// TestBuildCloneURL_NoCredentialsEmbedded is the focused regression test for
// gate-security-cli-join-clone-url-bearer-in-process-args. buildCloneURL must
// return a credential-less URL; the bearer flows through GIT_CONFIG_* env at
// the clone call site, not via the URL's userinfo segment.
func TestBuildCloneURL_NoCredentialsEmbedded(t *testing.T) {
	cases := []struct {
		name, portalURL string
	}{
		{"https no trailing slash", "https://portal.example.com"},
		{"https trailing slash", "https://portal.example.com/"},
		{"https with subpath", "https://portal.example.com/api/v1"},
		{"http localhost", "http://127.0.0.1:8080"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildCloneURL(tc.portalURL, "org-1", "sess-1")
			if strings.Contains(got, "@") {
				// `@` only appears in URLs as the separator between userinfo
				// and host — a credential-less URL must never contain it.
				t.Errorf("buildCloneURL returned URL with userinfo segment: %q", got)
			}
			if strings.Contains(got, "x-access-token") {
				t.Errorf("buildCloneURL leaked credential marker: %q", got)
			}
			if !strings.HasSuffix(got, "/git/org-1/sess-1.git") {
				t.Errorf("buildCloneURL path malformed: %q", got)
			}
		})
	}
}

func TestJoinAction_missingArg(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)

	_ = os.Chmod(dir, 0o700)
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

// ---- --open flag tests ----

// stubJoinGit is a minimal git stub for join tests (clone + checkout succeed).
func stubJoinGit(t *testing.T) {
	t.Helper()
	origRunGit := runGit
	origRunGitOutput := runGitOutput
	origRunGitWithEnv := runGitWithEnv
	t.Cleanup(func() {
		runGit = origRunGit
		runGitOutput = origRunGitOutput
		runGitWithEnv = origRunGitWithEnv
	})
	runGit = func(args ...string) error { return nil }
	runGitOutput = func(args ...string) (string, error) { return "", nil }
	runGitWithEnv = func(env []string, args ...string) error { return nil }
}

// buildJoinMux returns an httptest mux that handles /api/me and the session
// metadata endpoint for the given org+session.
func buildJoinMux(accountID, orgID, sessionID string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeMeJSON(w, accountID, orgID)
	})
	mux.HandleFunc("/api/orgs/"+orgID+"/sessions/"+sessionID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openapi.Session{
			Id:          sessionID,
			Name:        "Test Session",
			Goal:        "Test goal",
			OrgId:       orgID,
			DefaultMode: openapi.SessionDefaultModeSync,
		})
	})
	return mux
}

// TestJoinAction_openFlagBareID verifies that --open on a bare-id join calls
// openURL with the durable session-view URL.
func TestJoinAction_openFlagBareID(t *testing.T) {
	const (
		orgID     = "org-jopen-001"
		sessionID = "sess-jopen-001"
		accountID = "acct-jopen-001"
	)

	srv := httptest.NewServer(buildJoinMux(accountID, orgID, sessionID))
	defer srv.Close()

	setupJoinEnv(t, srv.URL)
	stubJoinGit(t)
	captured := stubOpenURL(t)

	app := buildCLIApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "join", sessionID, "--open",
	}); err != nil {
		t.Fatalf("join returned error: %v", err)
	}

	expectedURL := srv.URL + "/orgs/" + orgID + "/sessions/" + sessionID
	if len(*captured) != 1 || (*captured)[0] != expectedURL {
		t.Errorf("openURL captured = %v, want [%q]", *captured, expectedURL)
	}
}

// TestJoinAction_openFlagOrgSlash verifies that --open with an org/session arg
// form opens the same durable session-view URL (proves open is post-resolution).
func TestJoinAction_openFlagOrgSlash(t *testing.T) {
	const (
		orgID     = "org-jopen-002"
		sessionID = "sess-jopen-002"
		accountID = "acct-jopen-002"
	)

	srv := httptest.NewServer(buildJoinMux(accountID, orgID, sessionID))
	defer srv.Close()

	setupJoinEnv(t, srv.URL)
	stubJoinGit(t)
	captured := stubOpenURL(t)

	app := buildCLIApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "join", orgID + "/" + sessionID, "--open",
	}); err != nil {
		t.Fatalf("join returned error: %v", err)
	}

	// Whether the arg was bare-id or org/session, the opened URL must be identical.
	expectedURL := srv.URL + "/orgs/" + orgID + "/sessions/" + sessionID
	if len(*captured) != 1 || (*captured)[0] != expectedURL {
		t.Errorf("openURL captured = %v, want [%q]", *captured, expectedURL)
	}
}

// TestJoinAction_noOpenFlag verifies that omitting --open does NOT call openURL.
func TestJoinAction_noOpenFlag(t *testing.T) {
	const (
		orgID     = "org-jopen-003"
		sessionID = "sess-jopen-003"
		accountID = "acct-jopen-003"
	)

	srv := httptest.NewServer(buildJoinMux(accountID, orgID, sessionID))
	defer srv.Close()

	setupJoinEnv(t, srv.URL)
	stubJoinGit(t)
	captured := stubOpenURL(t)

	app := buildCLIApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "join", sessionID,
	}); err != nil {
		t.Fatalf("join returned error: %v", err)
	}

	if len(*captured) != 0 {
		t.Errorf("openURL should not be called without --open, got: %v", *captured)
	}
}

// ---- Resume mint + open tests (join) ----

// buildJoinMuxWithResume extends buildJoinMux with a POST /api/session-resumes
// endpoint that returns the given resume URL.
func buildJoinMuxWithResume(accountID, orgID, sessionID, resumeURL string) (*http.ServeMux, *string) {
	mux := buildJoinMux(accountID, orgID, sessionID)
	var capturedAuth string
	mux.HandleFunc("/api/session-resumes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "want POST", http.StatusMethodNotAllowed)
			return
		}
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(openapi.SessionResumeResponse{
			SessionId: sessionID,
			ResumeUrl: resumeURL,
			ExpiresIn: 60,
		})
	})
	return mux, &capturedAuth
}

// TestJoinAction_openFlagMintAndOpen verifies that --open on join mints a
// resume token and opens the exact resume_url via openSilent.
func TestJoinAction_openFlagMintAndOpen(t *testing.T) {
	const (
		orgID     = "org-jmint-001"
		sessionID = "sess-jmint-001"
		accountID = "acct-jmint-001"
		resumeURL = "https://portal.example.com/resume#rt=jointokensecret"
	)

	mux, capturedAuthPtr := buildJoinMuxWithResume(accountID, orgID, sessionID, resumeURL)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := setupJoinEnv(t, srv.URL)
	stubJoinGit(t)
	capturedOpen := stubOpenSilent(t)
	capturedFallback := stubOpenURL(t)

	// Write a per-session token so ReadCurrentBearer finds the per-session bearer.
	if err := os.MkdirAll(filepath.Join(dir, "sessions", sessionID), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions", sessionID, "token"), []byte("sess-bearer-join"), 0o600); err != nil {
		t.Fatalf("write session token: %v", err)
	}

	var stdout, stderr string
	app := buildCLIApp()
	stdout, stderr = captureStdoutAndStderr(t, func() {
		if err := app.Run(context.Background(), []string{
			"jamsesh", "join", orgID + "/" + sessionID, "--open",
		}); err != nil {
			t.Fatalf("join returned error: %v", err)
		}
	})

	// The exact resume_url must have been opened via openSilent.
	if len(*capturedOpen) != 1 || (*capturedOpen)[0] != resumeURL {
		t.Errorf("openSilent captured = %v, want [%q]", *capturedOpen, resumeURL)
	}

	// Fallback must NOT have been called.
	if len(*capturedFallback) != 0 {
		t.Errorf("openURL (fallback) should not be called on mint success, got: %v", *capturedFallback)
	}

	// Mint must have used EXACTLY the per-session bearer written above
	// ("sess-bearer-join"), not any legacy account token. This guards against
	// the account-token fallback regression.
	wantMintAuth := "Bearer sess-bearer-join"
	if *capturedAuthPtr != wantMintAuth {
		t.Errorf("mint Authorization = %q, want %q (exact per-session bearer)", *capturedAuthPtr, wantMintAuth)
	}

	// SECURITY: neither stdout nor stderr may contain the resume_url or #rt= fragment.
	assertNoTokenLeak(t, "stdout", stdout, resumeURL)
	assertNoTokenLeak(t, "stderr", stderr, resumeURL)
}

// TestJoinAction_openFlagMintFailure verifies that when mint fails, a warning
// is emitted and the fallback token-free URL is opened.
func TestJoinAction_openFlagMintFailure(t *testing.T) {
	const (
		orgID     = "org-jmintfail-001"
		sessionID = "sess-jmintfail-001"
		accountID = "acct-jmintfail-001"
	)

	mux := buildJoinMux(accountID, orgID, sessionID)
	mux.HandleFunc("/api/session-resumes", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupJoinEnv(t, srv.URL)
	stubJoinGit(t)
	capturedOpen := stubOpenSilent(t)
	capturedFallback := stubOpenURL(t)

	var stdout, stderr string
	app := buildCLIApp()
	stdout, stderr = captureStdoutAndStderr(t, func() {
		if err := app.Run(context.Background(), []string{
			"jamsesh", "join", orgID + "/" + sessionID, "--open",
		}); err != nil {
			t.Fatalf("join returned error: %v", err)
		}
	})

	// openSilent must NOT have been called.
	if len(*capturedOpen) != 0 {
		t.Errorf("openSilent should not be called on mint failure, got: %v", *capturedOpen)
	}

	// Fallback token-free URL must have been opened.
	expectedFallback := srv.URL + "/orgs/" + orgID + "/sessions/" + sessionID
	if len(*capturedFallback) != 1 || (*capturedFallback)[0] != expectedFallback {
		t.Errorf("fallback openURL captured = %v, want [%q]", *capturedFallback, expectedFallback)
	}

	// Stderr warning must be present.
	if !strings.Contains(stderr, "warning:") {
		t.Errorf("stderr should contain warning on mint failure; got: %q", stderr)
	}

	// SECURITY: no #rt= in any output.
	assertNoHashRT(t, "stdout", stdout)
	assertNoHashRT(t, "stderr", stderr)
}
