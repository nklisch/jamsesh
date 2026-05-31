package sessioncmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"jamsesh/internal/api/openapi"
)

// setupResumeEnv creates a temp JAMSESH_DATA_DIR, writes a per-session token
// and org_id sidecar, and points JAMSESH_PORTAL_URL at the given server URL.
// It returns the temp dir and registers t.Cleanup to unset CLAUDE_SESSION_ID.
func setupResumeEnv(t *testing.T, srvURL, sessionID, orgID string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	_ = os.Chmod(dir, 0o700)
	t.Setenv("JAMSESH_PORTAL_URL", srvURL)

	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	// Write per-session bearer.
	if err := os.WriteFile(filepath.Join(sessDir, "token"), []byte("bearer-"+sessionID), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	// Write org_id sidecar.
	if err := os.WriteFile(filepath.Join(sessDir, "org_id"), []byte(orgID), 0o600); err != nil {
		t.Fatalf("write org_id: %v", err)
	}

	// Ensure CLAUDE_SESSION_ID is unset by default; individual tests can override.
	t.Setenv("CLAUDE_SESSION_ID", "")
	return dir
}

// setupResumeInstanceBinding writes an instance_id file so that
// state.CurrentSessionID() maps ccInstanceID → sessionID.
func setupResumeInstanceBinding(t *testing.T, dir, sessionID, ccInstanceID string) {
	t.Helper()
	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatalf("mkdir for instance binding: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "instance_id"), []byte(ccInstanceID), 0o600); err != nil {
		t.Fatalf("write instance_id: %v", err)
	}
}

// buildResumeApp returns a minimal CLI app containing ResumeCommand.
func buildResumeApp() *cli.Command {
	return &cli.Command{
		Name:           "jamsesh",
		Commands:       []*cli.Command{ResumeCommand()},
		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {},
	}
}

// mintHandler returns an httptest mux handler for POST /api/session-resumes that
// serves a successful SessionResumeResponse. capturedAuthHdr is filled on each call.
func serveMintOK(mux *http.ServeMux, sessionID, resumeURL string, capturedAuthHdr *string) {
	mux.HandleFunc("/api/session-resumes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "want POST", http.StatusMethodNotAllowed)
			return
		}
		if capturedAuthHdr != nil {
			*capturedAuthHdr = r.Header.Get("Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(openapi.SessionResumeResponse{
			SessionId: sessionID,
			ResumeUrl: resumeURL,
			ExpiresIn: 60,
		})
	})
}

// serveMintErr registers a /api/session-resumes that always returns 500.
func serveMintErr(mux *http.ServeMux) {
	mux.HandleFunc("/api/session-resumes", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})
}

// ---- Resolver matrix tests ----

// TestResumeAction_explicitSessionID verifies that an explicit <session-id>
// argument is used directly, bypassing env-based resolution.
func TestResumeAction_explicitSessionID(t *testing.T) {
	const (
		sessionID = "sess-resume-explicit-001"
		orgID     = "org-resume-001"
		resumeURL = "https://portal.example.com/resume#rt=tok-explicit"
	)

	mux := http.NewServeMux()
	serveMintOK(mux, sessionID, resumeURL, nil)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupResumeEnv(t, srv.URL, sessionID, orgID)
	capturedOpen := stubOpenSilent(t)

	app := buildResumeApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "resume", sessionID,
	}); err != nil {
		t.Fatalf("resume returned error: %v", err)
	}

	if len(*capturedOpen) != 1 || (*capturedOpen)[0] != resumeURL {
		t.Errorf("openSilent captured = %v, want [%q]", *capturedOpen, resumeURL)
	}
}

// TestResumeAction_bareCurrentSessionID verifies that bare "jamsesh resume"
// with a CLAUDE_SESSION_ID mapping uses state.CurrentSessionID().
func TestResumeAction_bareCurrentSessionID(t *testing.T) {
	const (
		sessionID    = "sess-resume-current-001"
		orgID        = "org-resume-current-001"
		ccInstanceID = "cc-instance-001"
		resumeURL    = "https://portal.example.com/resume#rt=tok-current"
	)

	mux := http.NewServeMux()
	serveMintOK(mux, sessionID, resumeURL, nil)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := setupResumeEnv(t, srv.URL, sessionID, orgID)
	setupResumeInstanceBinding(t, dir, sessionID, ccInstanceID)
	t.Setenv("CLAUDE_SESSION_ID", ccInstanceID)

	capturedOpen := stubOpenSilent(t)

	app := buildResumeApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "resume",
	}); err != nil {
		t.Fatalf("resume returned error: %v", err)
	}

	if len(*capturedOpen) != 1 || (*capturedOpen)[0] != resumeURL {
		t.Errorf("openSilent captured = %v, want [%q]", *capturedOpen, resumeURL)
	}
}

// TestResumeAction_bareOneSessionNoCC verifies that outside CC context with
// exactly one session, bare "jamsesh resume" uses that session.
func TestResumeAction_bareOneSessionNoCC(t *testing.T) {
	const (
		sessionID = "sess-resume-single-001"
		orgID     = "org-resume-single-001"
		resumeURL = "https://portal.example.com/resume#rt=tok-single"
	)

	mux := http.NewServeMux()
	serveMintOK(mux, sessionID, resumeURL, nil)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// CLAUDE_SESSION_ID is unset (setupResumeEnv sets it to "").
	setupResumeEnv(t, srv.URL, sessionID, orgID)

	capturedOpen := stubOpenSilent(t)

	app := buildResumeApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "resume",
	}); err != nil {
		t.Fatalf("resume returned error: %v", err)
	}

	if len(*capturedOpen) != 1 || (*capturedOpen)[0] != resumeURL {
		t.Errorf("openSilent captured = %v, want [%q]", *capturedOpen, resumeURL)
	}
}

// TestResumeAction_multiSessionUnmappedErrors verifies that multiple sessions +
// unmapped CC instance produces an error citing jamsesh status and opens nothing.
func TestResumeAction_multiSessionUnmappedErrors(t *testing.T) {
	const (
		sessionID1 = "sess-resume-multi-001"
		sessionID2 = "sess-resume-multi-002"
		orgID      = "org-resume-multi"
	)

	mux := http.NewServeMux()
	// Mint must never be reached.
	var mintCalled bool
	mux.HandleFunc("/api/session-resumes", func(w http.ResponseWriter, r *http.Request) {
		mintCalled = true
		http.Error(w, "should not be called", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	_ = os.Chmod(dir, 0o700)
	t.Setenv("JAMSESH_PORTAL_URL", srv.URL)
	t.Setenv("CLAUDE_SESSION_ID", "") // outside CC context

	// Write two sessions.
	for _, sid := range []string{sessionID1, sessionID2} {
		sessDir := filepath.Join(dir, "sessions", sid)
		if err := os.MkdirAll(sessDir, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(sessDir, "token"), []byte("bearer-"+sid), 0o600); err != nil {
			t.Fatalf("write token: %v", err)
		}
		if err := os.WriteFile(filepath.Join(sessDir, "org_id"), []byte(orgID), 0o600); err != nil {
			t.Fatalf("write org_id: %v", err)
		}
	}

	capturedOpen := stubOpenSilent(t)

	var stdout, stderr string
	app := buildResumeApp()
	stdout, stderr = captureStdoutAndStderr(t, func() {
		err := app.Run(context.Background(), []string{"jamsesh", "resume"})
		if err == nil {
			t.Fatal("expected error for multiple sessions + unmapped, got nil")
		}
		if !strings.Contains(err.Error(), "jamsesh status") {
			t.Errorf("error should cite jamsesh status, got: %v", err)
		}
	})

	// Nothing must be opened.
	if len(*capturedOpen) != 0 {
		t.Errorf("openSilent should not be called on multi-session error, got: %v", *capturedOpen)
	}

	if mintCalled {
		t.Error("mint endpoint should not be called on resolver error")
	}

	// SECURITY: no token leaked.
	assertNoHashRT(t, "stdout", stdout)
	assertNoHashRT(t, "stderr", stderr)
}

// TestResumeAction_ccInstanceUnmappedErrors verifies that when CLAUDE_SESSION_ID
// is set but not mapped to any session, the error cites jamsesh status.
func TestResumeAction_ccInstanceUnmappedErrors(t *testing.T) {
	const ccInstanceID = "cc-unmapped-instance"

	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	_ = os.Chmod(dir, 0o700)
	t.Setenv("JAMSESH_PORTAL_URL", "http://localhost:19999") // no real server needed
	t.Setenv("CLAUDE_SESSION_ID", ccInstanceID)

	capturedOpen := stubOpenSilent(t)

	app := buildResumeApp()
	err := app.Run(context.Background(), []string{"jamsesh", "resume"})
	if err == nil {
		t.Fatal("expected error for unmapped CC instance, got nil")
	}
	if !strings.Contains(err.Error(), "jamsesh status") {
		t.Errorf("error should cite jamsesh status, got: %v", err)
	}

	if len(*capturedOpen) != 0 {
		t.Errorf("openSilent should not be called when CC instance is unmapped, got: %v", *capturedOpen)
	}
}

// ---- Mint-failure tests ----

// TestResumeAction_mintFailureNonzero verifies that a mint failure causes a
// nonzero exit, nothing is opened, and no token appears in any output.
func TestResumeAction_mintFailureNonzero(t *testing.T) {
	const (
		sessionID = "sess-resume-mintfail-001"
		orgID     = "org-resume-mintfail-001"
		resumeURL = "https://portal.example.com/resume#rt=secret-never-printed"
	)

	mux := http.NewServeMux()
	serveMintErr(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupResumeEnv(t, srv.URL, sessionID, orgID)
	capturedOpen := stubOpenSilent(t)

	var stdout, stderr string
	var gotErr error
	app := buildResumeApp()
	stdout, stderr = captureStdoutAndStderr(t, func() {
		gotErr = app.Run(context.Background(), []string{
			"jamsesh", "resume", sessionID,
		})
	})

	if gotErr == nil {
		t.Fatal("expected error on mint failure, got nil")
	}

	// Nothing must be opened.
	if len(*capturedOpen) != 0 {
		t.Errorf("openSilent must NOT be called on mint failure, got: %v", *capturedOpen)
	}

	// SECURITY: no token in any output.
	assertNoTokenLeak(t, "stdout", stdout, resumeURL)
	assertNoTokenLeak(t, "stderr", stderr, resumeURL)
	assertNoHashRT(t, "stdout", stdout)
	assertNoHashRT(t, "stderr", stderr)
}

// TestResumeAction_mintFailureNoFallback verifies that unlike --open (which
// falls back to a token-free URL), "jamsesh resume" strictly errors on mint
// failure and never falls back to openURL.
func TestResumeAction_mintFailureNoFallback(t *testing.T) {
	const (
		sessionID = "sess-resume-nofallback-001"
		orgID     = "org-resume-nofallback-001"
	)

	mux := http.NewServeMux()
	serveMintErr(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	setupResumeEnv(t, srv.URL, sessionID, orgID)
	capturedOpen := stubOpenSilent(t)
	capturedFallback := stubOpenURL(t)

	app := buildResumeApp()
	err := app.Run(context.Background(), []string{
		"jamsesh", "resume", sessionID,
	})

	if err == nil {
		t.Fatal("expected error on mint failure, got nil")
	}
	if len(*capturedOpen) != 0 {
		t.Errorf("openSilent must NOT be called on mint failure, got: %v", *capturedOpen)
	}
	if len(*capturedFallback) != 0 {
		t.Errorf("openURL fallback must NOT be called for resume command (unlike --open), got: %v", *capturedFallback)
	}
}

func TestResumeAction_invalidMintResponseDoesNotOpenOrLeak(t *testing.T) {
	const (
		sessionID = "sess-resume-invalid-mint-001"
		orgID     = "org-resume-invalid-mint-001"
		tokenURL  = "https://portal.example.com/resume#rt=invalid-mint-secret"
	)

	cases := []struct {
		name       string
		respSessID string
		resumeURL  string
	}{
		{
			name:       "empty resume_url",
			respSessID: sessionID,
			resumeURL:  "",
		},
		{
			name:       "session mismatch",
			respSessID: "sess-resume-invalid-mint-other",
			resumeURL:  tokenURL,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			serveMintOK(mux, tc.respSessID, tc.resumeURL, nil)
			srv := httptest.NewServer(mux)
			defer srv.Close()

			setupResumeEnv(t, srv.URL, sessionID, orgID)
			capturedOpen := stubOpenSilent(t)

			var stdout, stderr string
			var gotErr error
			app := buildResumeApp()
			stdout, stderr = captureStdoutAndStderr(t, func() {
				gotErr = app.Run(context.Background(), []string{
					"jamsesh", "resume", sessionID,
				})
			})

			if gotErr == nil {
				t.Fatal("expected error for invalid mint response, got nil")
			}
			if len(*capturedOpen) != 0 {
				t.Errorf("openSilent must NOT be called for invalid mint response, got: %v", *capturedOpen)
			}
			assertNoTokenLeak(t, "stdout", stdout, tokenURL)
			assertNoTokenLeak(t, "stderr", stderr, tokenURL)
			assertNoHashRT(t, "stdout", stdout)
			assertNoHashRT(t, "stderr", stderr)
		})
	}
}

// TestResumeAction_playgroundMissingBearer verifies that when a playground
// session (org_id == "org_playground") has no per-session bearer stored, the
// command fails locally with a clear error and does NOT attempt a mint at all —
// i.e. does NOT fall back to the legacy account token.
func TestResumeAction_playgroundMissingBearer(t *testing.T) {
	const (
		sessionID = "sess-resume-pg-nobearer-001"
		// playgroundOrgID = "org_playground" — use the package const.
	)

	var mintCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/session-resumes", func(w http.ResponseWriter, r *http.Request) {
		mintCalled = true
		http.Error(w, "should not be called", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Set up env with org_id=org_playground but NO token file.
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)
	_ = os.Chmod(dir, 0o700)
	t.Setenv("JAMSESH_PORTAL_URL", srv.URL)
	t.Setenv("CLAUDE_SESSION_ID", "")

	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write org_id = org_playground, but deliberately omit the token file.
	if err := os.WriteFile(filepath.Join(sessDir, "org_id"), []byte("org_playground"), 0o600); err != nil {
		t.Fatalf("write org_id: %v", err)
	}

	capturedOpen := stubOpenSilent(t)

	var stdout, stderr string
	app := buildResumeApp()
	stdout, stderr = captureStdoutAndStderr(t, func() {
		err := app.Run(context.Background(), []string{"jamsesh", "resume", sessionID})
		if err == nil {
			t.Fatal("expected error for playground session with missing bearer, got nil")
		}
		// Must cite the session id and hint at jamsesh status.
		if !strings.Contains(err.Error(), sessionID) {
			t.Errorf("error should cite session id, got: %v", err)
		}
		if !strings.Contains(err.Error(), "jamsesh status") {
			t.Errorf("error should hint at jamsesh status, got: %v", err)
		}
		// Must NOT contain "no playground credential" coming from an account-token mint attempt.
		// The key ACs: (1) error is local, (2) mint was never called.
	})

	// Mint endpoint must NEVER be reached — no fallback to account token.
	if mintCalled {
		t.Error("mint endpoint must NOT be called when playground bearer is missing; got a mint attempt (account-token fallback?)")
	}

	// Nothing must be opened.
	if len(*capturedOpen) != 0 {
		t.Errorf("openSilent should not be called, got: %v", *capturedOpen)
	}

	// SECURITY: no token leak.
	assertNoHashRT(t, "stdout", stdout)
	assertNoHashRT(t, "stderr", stderr)
}

// TestResumeAction_playgroundHappyPath verifies that when a playground session
// has a per-session anonymous bearer stored, resume succeeds: it uses EXACTLY
// the per-session bearer (not the legacy account token) on the mint request
// and opens the exact resume_url.
func TestResumeAction_playgroundHappyPath(t *testing.T) {
	const (
		sessionID  = "sess-resume-pg-happy-001"
		anonBearer = "anon-bearer-playground-resume-secret"
		resumeURL  = "https://portal.example.com/resume#rt=pgresumesecrettoken"
	)

	var capturedAuthHdr string
	mux := http.NewServeMux()
	serveMintOK(mux, sessionID, resumeURL, &capturedAuthHdr)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Set up env: org_id=org_playground + per-session token.
	dir := setupResumeEnv(t, srv.URL, sessionID, "org_playground")
	// Overwrite the token written by setupResumeEnv with the specific anon bearer.
	if err := os.WriteFile(
		filepath.Join(dir, "sessions", sessionID, "token"),
		[]byte(anonBearer), 0o600,
	); err != nil {
		t.Fatalf("write anon bearer: %v", err)
	}

	// Also write a legacy account token to verify it is NOT used.
	if err := os.WriteFile(
		filepath.Join(dir, "token"),
		[]byte("legacy-account-token-must-not-be-used"), 0o600,
	); err != nil {
		t.Fatalf("write legacy token: %v", err)
	}

	capturedOpen := stubOpenSilent(t)

	app := buildResumeApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "resume", sessionID,
	}); err != nil {
		t.Fatalf("playground resume returned error: %v", err)
	}

	// The exact resume_url must have been opened.
	if len(*capturedOpen) != 1 || (*capturedOpen)[0] != resumeURL {
		t.Errorf("openSilent captured = %v, want [%q]", *capturedOpen, resumeURL)
	}

	// SECURITY: mint must use EXACTLY the per-session anon bearer, never the legacy token.
	wantAuth := "Bearer " + anonBearer
	if capturedAuthHdr != wantAuth {
		t.Errorf("mint Authorization = %q, want %q (playground anon bearer, not legacy account token)", capturedAuthHdr, wantAuth)
	}
}

// TestResumeAction_successUsesPerSessionBearer verifies that the mint request
// uses the per-session bearer token (not any account-wide token).
func TestResumeAction_successUsesPerSessionBearer(t *testing.T) {
	const (
		sessionID     = "sess-resume-bearer-001"
		orgID         = "org-resume-bearer-001"
		sessionBearer = "bearer-per-session-secret"
		resumeURL     = "https://portal.example.com/resume#rt=tok-bearer"
	)

	var capturedAuthHdr string
	mux := http.NewServeMux()
	serveMintOK(mux, sessionID, resumeURL, &capturedAuthHdr)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := setupResumeEnv(t, srv.URL, sessionID, orgID)
	// Overwrite the per-session token with a specific value to verify it's used.
	if err := os.WriteFile(
		filepath.Join(dir, "sessions", sessionID, "token"),
		[]byte(sessionBearer), 0o600,
	); err != nil {
		t.Fatalf("write session token: %v", err)
	}

	capturedOpen := stubOpenSilent(t)

	app := buildResumeApp()
	if err := app.Run(context.Background(), []string{
		"jamsesh", "resume", sessionID,
	}); err != nil {
		t.Fatalf("resume returned error: %v", err)
	}

	if len(*capturedOpen) != 1 || (*capturedOpen)[0] != resumeURL {
		t.Errorf("openSilent captured = %v, want [%q]", *capturedOpen, resumeURL)
	}
	expectedAuth := "Bearer " + sessionBearer
	if capturedAuthHdr != expectedAuth {
		t.Errorf("mint Authorization = %q, want %q (per-session bearer)", capturedAuthHdr, expectedAuth)
	}
}
