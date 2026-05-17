package portalclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func setupRefreshDir(t *testing.T, refreshToken string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
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

	dir := os.Getenv("CLAUDE_PLUGIN_DATA")
	gotAccess, _ := os.ReadFile(dir + "/token")
	gotRefresh, _ := os.ReadFile(dir + "/refresh_token")

	if string(gotAccess) != "at-new" {
		t.Errorf("access token: got %q, want %q", string(gotAccess), "at-new")
	}
	if string(gotRefresh) != "rt-new" {
		t.Errorf("refresh token: got %q, want %q", string(gotRefresh), "rt-new")
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

func TestRefresher_Refresh_MissingRefreshToken(t *testing.T) {
	// CLAUDE_PLUGIN_DATA set but no refresh_token file.
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)

	r := &Refresher{BaseURL: "http://unused"}
	err := r.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error when refresh_token file is absent")
	}
}
