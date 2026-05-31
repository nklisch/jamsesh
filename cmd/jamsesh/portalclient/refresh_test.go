package portalclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jamsesh/cmd/jamsesh/state"
)

func setupRefreshDir(t *testing.T, refreshToken string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)

	_ = os.Chmod(dir, 0o700)
	if err := os.WriteFile(dir+"/refresh_token", []byte(refreshToken), 0o600); err != nil {
		t.Fatalf("writing refresh_token: %v", err)
	}
}

func TestRefresher_Refresh_WritesTokens(t *testing.T) {
	setupRefreshDir(t, "rt-initial")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/auth/refresh" {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["refresh_token"] == "" {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenPair{
			AccessToken:      "at-new",
			RefreshToken:     "rt-new",
			AccessExpiresAt:  time.Now().Add(time.Hour),
			RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		})
	}))
	defer srv.Close()

	r := &Refresher{BaseURL: srv.URL, HTTP: srv.Client()}
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh error: %v", err)
	}

	dir := os.Getenv("JAMSESH_DATA_DIR")
	gotAccess, _ := os.ReadFile(dir + "/token")
	gotRefresh, _ := os.ReadFile(dir + "/refresh_token")

	if string(gotAccess) != "at-new" {
		t.Errorf("access token: got %q, want %q", string(gotAccess), "at-new")
	}
	if string(gotRefresh) != "rt-new" {
		t.Errorf("refresh token: got %q, want %q", string(gotRefresh), "rt-new")
	}
}

// TestRefresher_Refresh_WritesSessionScopedToken verifies that a Refresher
// scoped to an explicit session (SessionID, set by WireRefresh from
// client.SessionID) writes the refreshed access token to that session's
// per-session path — not the legacy ${data-dir}/token (a MIGRATED_TO_PER_SESSION
// stub after migration). Keeps the refresh write-back consistent with the
// session the client's requests use.
func TestRefresher_Refresh_WritesSessionScopedToken(t *testing.T) {
	setupRefreshDir(t, "rt-initial")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenPair{
			AccessToken:      "at-sess",
			RefreshToken:     "rt-sess",
			AccessExpiresAt:  time.Now().Add(time.Hour),
			RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		})
	}))
	defer srv.Close()

	r := &Refresher{BaseURL: srv.URL, HTTP: srv.Client(), SessionID: "sess-x"}
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh error: %v", err)
	}

	got, err := state.ReadSessionToken("sess-x")
	if err != nil {
		t.Fatalf("ReadSessionToken: %v", err)
	}
	if string(got) != "at-sess" {
		t.Errorf("session-scoped access token: got %q, want %q (sessions/sess-x/token)", string(got), "at-sess")
	}
	// The legacy account-wide token must NOT be written for a session-scoped refresh.
	dir := os.Getenv("JAMSESH_DATA_DIR")
	if _, err := os.Stat(dir + "/token"); err == nil {
		t.Errorf("legacy ${data-dir}/token should not be written for a session-scoped refresh")
	}
}

func TestRefresher_Refresh_SingleFlight(t *testing.T) {
	setupRefreshDir(t, "rt-sf")

	const goroutines = 10
	var hitCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		// Simulate a small delay so concurrent goroutines have time to pile up.
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenPair{
			AccessToken:      "at-sf",
			RefreshToken:     "rt-sf2",
			AccessExpiresAt:  time.Now().Add(time.Hour),
			RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		})
	}))
	defer srv.Close()

	r := &Refresher{BaseURL: srv.URL, HTTP: srv.Client()}

	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = r.Refresh(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	if got := hitCount.Load(); got != 1 {
		t.Errorf("singleflight: expected 1 HTTP POST, got %d", got)
	}
}

func TestRefresher_Refresh_ServerError(t *testing.T) {
	setupRefreshDir(t, "rt-bad")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "token revoked", http.StatusUnauthorized)
	}))
	defer srv.Close()

	r := &Refresher{BaseURL: srv.URL, HTTP: srv.Client()}
	err := r.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error on non-200 refresh response, got nil")
	}
}

// TestRefresher_Refresh_LargeErrorBodyTruncated pins the body-bound
// invariant: a misbehaving upstream that returns megabyte-scale error
// bodies on refresh must not have those bodies surface in local error
// strings / stderr / logs. The error message is bounded to ~maxErrBodyBytes.
// (gate-security-refresh-error-body-leak)
func TestRefresher_Refresh_LargeErrorBodyTruncated(t *testing.T) {
	setupRefreshDir(t, "rt-large-err")

	// 8 KiB body — well above the 512B bound.
	huge := strings.Repeat("X", 8*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(huge))
	}))
	defer srv.Close()

	r := &Refresher{BaseURL: srv.URL, HTTP: srv.Client()}
	err := r.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error on non-200 refresh response, got nil")
	}
	msg := err.Error()
	// The error must include some body content (the truncated prefix), but
	// MUST be bounded. The full 8 KiB body must not appear.
	if len(msg) > 2*1024 {
		t.Errorf("refresh error message length %d > expected bound (~512B body cap + framing)", len(msg))
	}
	if strings.Count(msg, "X") > 600 {
		// 512 body bytes + a handful of framing chars; > 600 means the bound failed.
		t.Errorf("refresh error contains >600 body bytes (X count = %d); want <= ~600", strings.Count(msg, "X"))
	}
}

func TestRefresher_Refresh_MissingRefreshToken(t *testing.T) {
	// JAMSESH_DATA_DIR set but no refresh_token file.
	dir := t.TempDir()
	t.Setenv("JAMSESH_DATA_DIR", dir)

	_ = os.Chmod(dir, 0o700)

	r := &Refresher{BaseURL: "http://unused"}
	err := r.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error when refresh_token file is absent")
	}
}
