package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"jamsesh/internal/portal/config"
	"jamsesh/internal/portal/router"
	"jamsesh/internal/portal/server"
)

// TestGracefulShutdown is the in-process smoke test for the server lifecycle:
//  1. Find an ephemeral port and configure server on it.
//  2. Start server.Run in a goroutine with a cancellable context.
//  3. Poll GET /healthz until 200 (up to 5 s).
//  4. Cancel the context to trigger graceful shutdown.
//  5. Assert Run returns nil within 1 s.
//  6. Assert the /healthz response body was {"status":"ok"}.
func TestGracefulShutdown(t *testing.T) {
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	cfg := config.Config{
		Bind:     addr,
		DBDriver: "sqlite",
		DBDSN:    "./jamsesh.db",
		TLS:      config.TLSConfig{Mode: "behind_proxy"},
		Log:      config.LogConfig{Format: "json"},
		Storage:  "./storage",
	}

	handler := router.New(router.Deps{
		TrustProxyHeaders: true, // behind_proxy mode
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() {
		runErr <- server.Run(ctx, cfg, handler)
	}()

	// Poll /healthz until the server is up (max 5 s, 50 ms backoff).
	baseURL := "http://" + addr
	var healthBody string
	deadline := time.Now().Add(5 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("server did not become ready within 5 s")
		}
		resp, err := http.Get(baseURL + "/healthz") //nolint:noctx
		if err == nil {
			var payload struct {
				Status string `json:"status"`
			}
			if derr := json.NewDecoder(resp.Body).Decode(&payload); derr == nil {
				healthBody = payload.Status
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Verify healthz body.
	if healthBody != "ok" {
		t.Errorf("/healthz body status: got %q, want %q", healthBody, "ok")
	}

	// Trigger graceful shutdown.
	cancel()

	// Run must return nil within 1 s.
	select {
	case err := <-runErr:
		if err != nil {
			t.Errorf("server.Run returned non-nil error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("server.Run did not return within 1 s after context cancel")
	}
}

func TestListenError(t *testing.T) {
	// Bind to a port, keep the listener open, then try to bind again —
	// server.Run should return the listen error.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := ln.Addr().String()
	defer ln.Close()

	cfg := config.Config{
		Bind:     addr,
		DBDriver: "sqlite",
		TLS:      config.TLSConfig{Mode: "behind_proxy"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = server.Run(ctx, cfg, http.NotFoundHandler())
	if err == nil {
		t.Fatal("expected listen error, got nil")
	}
}

// freePort finds an available TCP port on loopback.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: net.Listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("freePort: close: %v", err)
	}
	return port
}
