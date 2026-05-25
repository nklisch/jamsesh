package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestStateDir creates a temp directory and sets JAMSESH_DATA_DIR.
// Returns a cleanup function.
func setupTestStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)

	_ = os.Chmod(dir, 0o700)
	return dir
}

// TestBrowserFlowHappyPath simulates a complete browser OAuth flow without
// opening a real browser. A mock portal handles the token exchange; the test
// acts as the "browser" by extracting the redirect_uri from the opened URL
// and sending the callback directly to the local listener.
func TestBrowserFlowHappyPath(t *testing.T) {
	stateDir := setupTestStateDir(t)

	// Mock portal: handles /api/auth/code (token exchange).
	fakeAccessToken := "test-access-token-abc123"
	fakeRefreshToken := "test-refresh-token-xyz789"

	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/code":
			if r.Method != http.MethodPost {
				http.Error(w, "want POST", http.StatusMethodNotAllowed)
				return
			}
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			// Validate required PKCE and code fields.
			if r.FormValue("grant_type") != "authorization_code" {
				http.Error(w, "bad grant_type", http.StatusBadRequest)
				return
			}
			if r.FormValue("code") == "" {
				http.Error(w, "missing code", http.StatusBadRequest)
				return
			}
			if r.FormValue("code_verifier") == "" {
				http.Error(w, "missing code_verifier", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tokenResponse{
				AccessToken:  fakeAccessToken,
				RefreshToken: fakeRefreshToken,
				TokenType:    "Bearer",
				ExpiresIn:    3600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer portal.Close()

	// capturedURL receives the URL that the "browser" would open.
	capturedURL := make(chan string, 1)

	fakeOpenURL := func(rawURL string) error {
		capturedURL <- rawURL
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run the browser flow in a goroutine; we'll drive the callback below.
	errCh := make(chan error, 1)
	go func() {
		errCh <- browserFlow(ctx, portal.URL, fakeOpenURL)
	}()

	// Wait for the openURL call, then act as the browser to hit the callback.
	openedURL := <-capturedURL
	parsed, err := url.Parse(openedURL)
	if err != nil {
		t.Fatalf("parsing opened URL %q: %v", openedURL, err)
	}

	// Verify the authorization URL has the required parameters.
	q := parsed.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", q.Get("code_challenge_method"))
	}
	if q.Get("code_challenge") == "" {
		t.Error("code_challenge is empty")
	}
	stateParam := q.Get("state")
	if stateParam == "" {
		t.Error("state is empty")
	}
	redirectURI := q.Get("redirect_uri")
	if !strings.HasPrefix(redirectURI, "http://127.0.0.1:") {
		t.Errorf("redirect_uri = %q, want http://127.0.0.1:<port>/cb prefix", redirectURI)
	}

	// Simulate the browser hitting the local callback with a fake code and
	// the correct state.
	cbURL := redirectURI + "?code=fake-auth-code&state=" + url.QueryEscape(stateParam)
	resp, err := http.Get(cbURL) //nolint:noctx
	if err != nil {
		t.Fatalf("hitting callback URL: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("callback response status = %d, want 200", resp.StatusCode)
	}

	// The browser flow should now complete successfully.
	if err := <-errCh; err != nil {
		t.Fatalf("browserFlow returned error: %v", err)
	}

	// Verify tokens were written to local state.
	tokenBytes, err := os.ReadFile(filepath.Join(stateDir, "token"))
	if err != nil {
		t.Fatalf("reading token file: %v", err)
	}
	if strings.TrimSpace(string(tokenBytes)) != fakeAccessToken {
		t.Errorf("token = %q, want %q", strings.TrimSpace(string(tokenBytes)), fakeAccessToken)
	}

	refreshBytes, err := os.ReadFile(filepath.Join(stateDir, "refresh_token"))
	if err != nil {
		t.Fatalf("reading refresh_token file: %v", err)
	}
	if strings.TrimSpace(string(refreshBytes)) != fakeRefreshToken {
		t.Errorf("refresh_token = %q, want %q", strings.TrimSpace(string(refreshBytes)), fakeRefreshToken)
	}

	// Verify token file mode is 0600.
	info, err := os.Stat(filepath.Join(stateDir, "token"))
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("token file mode = %o, want 0600", info.Mode().Perm())
	}
}

// TestBrowserFlowStateMismatch verifies that the callback handler rejects
// a state parameter that doesn't match the one included in the auth URL.
func TestBrowserFlowStateMismatch(t *testing.T) {
	setupTestStateDir(t)

	// Minimal mock portal — the token exchange should never be reached.
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer portal.Close()

	capturedURL := make(chan string, 1)
	fakeOpenURL := func(rawURL string) error {
		capturedURL <- rawURL
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- browserFlow(ctx, portal.URL, fakeOpenURL)
	}()

	openedURL := <-capturedURL
	parsed, _ := url.Parse(openedURL)
	redirectURI := parsed.Query().Get("redirect_uri")

	// Send a callback with a tampered state.
	cbURL := redirectURI + "?code=legit-code&state=tampered-state-value"
	resp, err := http.Get(cbURL) //nolint:noctx
	if err != nil {
		t.Fatalf("hitting callback: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("callback status = %d, want 400 on state mismatch", resp.StatusCode)
	}

	if err := <-errCh; err == nil {
		t.Error("browserFlow expected an error on state mismatch, got nil")
	} else if !strings.Contains(err.Error(), "state") {
		t.Errorf("error = %q, want it to mention 'state'", err.Error())
	}
}

// TestBrowserFlowAuthURLParameters validates the shape of the authorization
// URL without running a full flow.
func TestBrowserFlowAuthURLParameters(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	stateVal := "test-state-nonce"
	redirectURI := "http://127.0.0.1:12345/cb"
	portalURL := "https://portal.example.com"

	authURL, err := buildAuthURL(portalURL, pkce, stateVal, redirectURI)
	if err != nil {
		t.Fatalf("buildAuthURL: %v", err)
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parsing auth URL: %v", err)
	}
	q := parsed.Query()

	cases := []struct{ key, want string }{
		{"response_type", "code"},
		{"code_challenge", pkce.Challenge},
		{"code_challenge_method", "S256"},
		{"state", stateVal},
		{"redirect_uri", redirectURI},
	}
	for _, c := range cases {
		if got := q.Get(c.key); got != c.want {
			t.Errorf("param %q = %q, want %q", c.key, got, c.want)
		}
	}

	if !strings.Contains(parsed.Path, "/api/auth/oauth/github/start") {
		t.Errorf("auth URL path = %q, want it to contain /api/auth/oauth/github/start", parsed.Path)
	}
}

// TestCommandHelpShowsDeviceCodeFlag verifies that the --device-code flag
// appears in the command definition without running a real flow.
func TestCommandHelpShowsDeviceCodeFlag(t *testing.T) {
	cmd := Command()
	found := false
	for _, f := range cmd.Flags {
		if f.Names()[0] == "device-code" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Command() does not expose a --device-code flag")
	}
}

// TestExchangeCodeRequest verifies that exchangeCode sends the correct HTTP
// parameters to the portal's token endpoint.
func TestExchangeCodeRequest(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", stateDir)

	_ = os.Chmod(stateDir, 0o700)

	var gotForm url.Values
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/code" {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		gotForm = r.Form
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken:  "at",
			RefreshToken: "rt",
		})
	}))
	defer portal.Close()

	err := exchangeCode(context.Background(), portal.URL, "my-code", "my-verifier",
		fmt.Sprintf("%s/cb", portal.URL))
	if err != nil {
		t.Fatalf("exchangeCode: %v", err)
	}

	checks := map[string]string{
		"grant_type":    "authorization_code",
		"code":          "my-code",
		"code_verifier": "my-verifier",
	}
	for k, want := range checks {
		if got := gotForm.Get(k); got != want {
			t.Errorf("form field %q = %q, want %q", k, got, want)
		}
	}
}
