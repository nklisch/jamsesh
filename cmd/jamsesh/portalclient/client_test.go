package portalclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

// setupTokenDir sets CLAUDE_PLUGIN_DATA to a temp directory and writes an
// initial access token. It returns a cleanup function.
func setupTokenDir(t *testing.T, token string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_PLUGIN_DATA", dir)
	if err := os.WriteFile(dir+"/token", []byte(token), 0o600); err != nil {
		t.Fatalf("writing token: %v", err)
	}
}

func TestClient_Do_HappyPath(t *testing.T) {
	setupTokenDir(t, "tok-abc")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-abc" {
			http.Error(w, "bad auth", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/test", nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestClient_Do_401ThenSuccess(t *testing.T) {
	setupTokenDir(t, "tok-old")
	dir := os.Getenv("CLAUDE_PLUGIN_DATA")

	var callCount atomic.Int32
	var refreshCalled atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			// First call: return 401.
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Second call: require new token.
		if r.Header.Get("Authorization") != "Bearer tok-new" {
			http.Error(w, "wrong token on retry", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
		Refresh: func(ctx context.Context) error {
			refreshCalled.Add(1)
			// Simulate Refresher writing a new token to disk.
			return os.WriteFile(dir+"/token", []byte("tok-new"), 0o600)
		},
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/resource", nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on retry, got %d", resp.StatusCode)
	}
	if refreshCalled.Load() != 1 {
		t.Fatalf("expected Refresh called once, got %d", refreshCalled.Load())
	}
	if callCount.Load() != 2 {
		t.Fatalf("expected 2 server calls, got %d", callCount.Load())
	}
}

func TestClient_Do_401AfterRefreshFails(t *testing.T) {
	setupTokenDir(t, "tok-stale")

	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	refreshErr := errors.New("refresh_token revoked")
	c := &Client{
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
		Refresh: func(ctx context.Context) error {
			return refreshErr
		},
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/resource", nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when refresh fails, got nil")
	}
	if !errors.Is(err, refreshErr) {
		t.Fatalf("expected wrapped refreshErr, got: %v", err)
	}
	// Only one server call — we bail before the retry.
	if callCount.Load() != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount.Load())
	}
}

func TestClient_Do_401StillAfterRefresh(t *testing.T) {
	setupTokenDir(t, "tok-bad")

	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		// Always 401.
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &Client{
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
		Refresh: func(ctx context.Context) error {
			// Refresh succeeds but portal still rejects.
			return nil
		},
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/resource", nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on persistent 401, got nil")
	}
	// Two server calls: initial + retry.
	if callCount.Load() != 2 {
		t.Fatalf("expected 2 server calls, got %d", callCount.Load())
	}
}

func TestClient_Do_NoRefreshFunc_401(t *testing.T) {
	setupTokenDir(t, "tok-x")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	// Refresh is nil — client must return the 401 response, not error.
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/resource", nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error with nil Refresh: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 pass-through, got %d", resp.StatusCode)
	}
}

func TestGetJSON(t *testing.T) {
	setupTokenDir(t, "tok-json")

	type payload struct {
		Name string `json:"name"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"jamsesh"}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	got, err := GetJSON[payload](context.Background(), c, "/info")
	if err != nil {
		t.Fatalf("GetJSON error: %v", err)
	}
	if got.Name != "jamsesh" {
		t.Fatalf("expected name=jamsesh, got %q", got.Name)
	}
}

func TestPostJSON(t *testing.T) {
	setupTokenDir(t, "tok-post")

	type req struct {
		Msg string `json:"msg"`
	}
	type resp struct {
		Echo string `json:"echo"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "want POST", http.StatusMethodNotAllowed)
			return
		}
		var body req
		if err := parseJSON(r, &body); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"echo":"` + body.Msg + `"}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	got, err := PostJSON[resp](context.Background(), c, "/echo", req{Msg: "hello"})
	if err != nil {
		t.Fatalf("PostJSON error: %v", err)
	}
	if got.Echo != "hello" {
		t.Fatalf("expected echo=hello, got %q", got.Echo)
	}
}
