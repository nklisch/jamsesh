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

	// Verify git clone was called via runGitWithEnv (extraHeader path).
	// The clone command is invoked as `git -c http.extraHeader=... clone --bare <url> <dest>`.
	var cloneCall []string
	for _, call := range gitWithEnvCalls {
		for _, a := range call {
			if a == "clone" {
				cloneCall = call
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
	// http.extraHeader -c flag, where it's still embedded in argv but at least
	// scoped behind a header rather than a remote-URL field that ends up in
	// `git remote -v` / reflog output.
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
	// The bearer token "tok-test" must not appear unencoded anywhere in
	// argv outside the http.extraHeader Basic-auth blob.
	for _, a := range cloneCall {
		if strings.HasPrefix(a, "http.extraHeader=") {
			continue
		}
		if strings.Contains(a, "tok-test") {
			t.Errorf("bearer token leaked into argv: %q", a)
		}
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
// return a credential-less URL; the bearer flows through `-c http.extraHeader`
// at the clone call site, not via the URL's userinfo segment.
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
