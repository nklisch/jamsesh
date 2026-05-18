package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
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
		Bind:                 addr,
		DBDriver:             "sqlite",
		DBDSN:                "./jamsesh.db",
		TLS:                  config.TLSConfig{Mode: "behind_proxy"},
		Log:                  config.LogConfig{Format: "json"},
		Storage:              "./storage",
		ShutdownGraceSeconds: 5,
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

// TestGraceWindowCompletes verifies that a slow in-flight request completes
// when the grace window is longer than the request duration.
//
// Setup: handler holds for 200 ms; grace = 10 s.
// Expect: server.Run returns nil; slow request completes.
func TestGraceWindowCompletes(t *testing.T) {
	const handlerDelay = 200 * time.Millisecond
	const graceSeconds = 10

	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	var requestCompleted atomic.Bool

	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			time.Sleep(handlerDelay)
			requestCompleted.Store(true)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})

	cfg := config.Config{
		Bind:                 addr,
		DBDriver:             "sqlite",
		TLS:                  config.TLSConfig{Mode: "behind_proxy"},
		ShutdownGraceSeconds: graceSeconds,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() {
		runErr <- server.Run(ctx, cfg, slowHandler)
	}()

	// Wait for the server to be ready.
	baseURL := "http://" + addr
	waitReady(t, baseURL+"/", 5*time.Second)

	// Start a slow request in the background.
	reqDone := make(chan error, 1)
	go func() {
		resp, err := http.Get(baseURL + "/slow") //nolint:noctx
		if resp != nil {
			resp.Body.Close()
		}
		reqDone <- err
	}()

	// Give the request a moment to reach the handler before triggering shutdown.
	time.Sleep(20 * time.Millisecond)
	cancel()

	// The slow request should complete without error.
	select {
	case err := <-reqDone:
		if err != nil {
			t.Errorf("slow request returned error: %v", err)
		}
	case <-time.After(graceSeconds * time.Second):
		t.Fatal("slow request did not complete within grace window")
	}

	if !requestCompleted.Load() {
		t.Error("handler did not complete: requestCompleted is false")
	}

	// server.Run should return nil once the request is done.
	select {
	case err := <-runErr:
		if err != nil {
			t.Errorf("server.Run returned non-nil error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server.Run did not return within 3 s after slow request completed")
	}
}

// TestGraceWindowCutsOff verifies that a slow in-flight request is cut off
// when the grace window expires before the request finishes.
//
// Setup: handler holds for 3 s; grace = 1 s.
// Expect: server.Run returns within ~2 s (grace expires, not the full 3 s).
func TestGraceWindowCutsOff(t *testing.T) {
	const handlerDelay = 3 * time.Second
	const graceSeconds = 1

	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	var handlerStarted atomic.Bool

	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			handlerStarted.Store(true)
			time.Sleep(handlerDelay)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})

	cfg := config.Config{
		Bind:                 addr,
		DBDriver:             "sqlite",
		TLS:                  config.TLSConfig{Mode: "behind_proxy"},
		ShutdownGraceSeconds: graceSeconds,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() {
		runErr <- server.Run(ctx, cfg, slowHandler)
	}()

	// Wait for the server to be ready.
	baseURL := "http://" + addr
	waitReady(t, baseURL+"/", 5*time.Second)

	// Start a slow request in the background. We don't care if it errors —
	// the server will cut it off.
	go func() {
		resp, err := http.Get(baseURL + "/slow") //nolint:noctx
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}()

	// Wait for the handler to start, then trigger shutdown.
	deadline := time.Now().Add(2 * time.Second)
	for !handlerStarted.Load() {
		if time.Now().After(deadline) {
			t.Fatal("slow handler did not start within 2 s")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

	// server.Run must return within 2 s (grace=1s + small overhead).
	// If it waited for the full handler delay (3 s) the grace window didn't work.
	start := time.Now()
	select {
	case err := <-runErr:
		elapsed := time.Since(start)
		// server.Run returns a non-nil error when the shutdown context expires
		// before all connections drain.
		t.Logf("server.Run returned in %v: %v", elapsed, err)
		if elapsed > 2*time.Second {
			t.Errorf("server.Run took %v; expected < 2 s (grace was %d s)", elapsed, graceSeconds)
		}
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("server.Run did not return within 2.5 s; grace window did not cut off the slow request")
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
		Bind:                 addr,
		DBDriver:             "sqlite",
		TLS:                  config.TLSConfig{Mode: "behind_proxy"},
		ShutdownGraceSeconds: 5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = server.Run(ctx, cfg, http.NotFoundHandler())
	if err == nil {
		t.Fatal("expected listen error, got nil")
	}
}

// waitReady polls the given URL until it returns a 2xx status or the deadline
// elapses. It fails the test if the deadline is reached.
func waitReady(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("server did not become ready within %v", timeout)
		}
		resp, err := http.Get(url) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
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
