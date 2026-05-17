package readyz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/router/readyz"
)

// newHealthyServer returns a test server that responds 200 to all requests.
func newHealthyServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newUnhealthyServer returns a test server that responds 503 to all requests.
func newUnhealthyServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// hostPort extracts the host:port from a test server URL.
func hostPort(srv *httptest.Server) string {
	// URL is "http://127.0.0.1:PORT"
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestProbeCheck_EmptyAddrs(t *testing.T) {
	p := &readyz.Probe{}
	got := p.Check(context.Background(), nil)
	if len(got) != 0 {
		t.Fatalf("expected empty result for nil addrs, got %v", got)
	}
	got = p.Check(context.Background(), []string{})
	if len(got) != 0 {
		t.Fatalf("expected empty result for empty addrs, got %v", got)
	}
}

func TestProbeCheck_AllHealthy(t *testing.T) {
	s1 := newHealthyServer(t)
	s2 := newHealthyServer(t)
	addrs := []string{hostPort(s1), hostPort(s2)}

	p := &readyz.Probe{Path: "/"}
	got := p.Check(context.Background(), addrs)
	if len(got) != 2 {
		t.Fatalf("expected 2 healthy, got %v", got)
	}
}

func TestProbeCheck_MixedHealth(t *testing.T) {
	healthy := newHealthyServer(t)
	unhealthy := newUnhealthyServer(t)
	addrs := []string{hostPort(healthy), hostPort(unhealthy)}

	p := &readyz.Probe{Path: "/"}
	got := p.Check(context.Background(), addrs)
	if len(got) != 1 {
		t.Fatalf("expected 1 healthy, got %v", got)
	}
	if got[0] != hostPort(healthy) {
		t.Fatalf("expected healthy addr %q, got %q", hostPort(healthy), got[0])
	}
}

func TestProbeCheck_AllUnhealthy(t *testing.T) {
	u1 := newUnhealthyServer(t)
	u2 := newUnhealthyServer(t)
	addrs := []string{hostPort(u1), hostPort(u2)}

	p := &readyz.Probe{Path: "/"}
	got := p.Check(context.Background(), addrs)
	if len(got) != 0 {
		t.Fatalf("expected 0 healthy, got %v", got)
	}
}

func TestProbeCheck_UnreachableAddr(t *testing.T) {
	// A port that nothing listens on — probe should not block and should
	// return healthy=false without panicking.
	p := &readyz.Probe{
		Path:   "/readyz",
		Client: &http.Client{Timeout: 100 * time.Millisecond},
	}
	got := p.Check(context.Background(), []string{"127.0.0.1:1"})
	if len(got) != 0 {
		t.Fatalf("expected 0 healthy for unreachable addr, got %v", got)
	}
}

// TestProbeCheck_Parallel verifies that N addresses are probed concurrently
// so total wall-clock time is bounded by one probe timeout, not N.
func TestProbeCheck_Parallel(t *testing.T) {
	const n = 5
	const delay = 80 * time.Millisecond

	// Each server sleeps for `delay` before responding 200.
	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
	})

	addrs := make([]string, n)
	for i := range n {
		srv := httptest.NewServer(slowHandler)
		t.Cleanup(srv.Close)
		addrs[i] = hostPort(srv)
	}

	p := &readyz.Probe{
		Path:   "/",
		Client: &http.Client{Timeout: 2 * time.Second},
	}

	start := time.Now()
	got := p.Check(context.Background(), addrs)
	elapsed := time.Since(start)

	if len(got) != n {
		t.Fatalf("expected %d healthy, got %d", n, len(got))
	}
	// Serial would take n*delay; parallel should be ~delay. Allow 2x delay for CI jitter.
	maxSerial := time.Duration(n) * delay
	if elapsed >= maxSerial {
		t.Errorf("probes appear serial: elapsed %v ≥ serial bound %v", elapsed, maxSerial)
	}
}

func TestProbeCheck_DefaultPath(t *testing.T) {
	// Probe with no Path set should hit /readyz.
	var hitPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitPath = r.URL.Path
		if r.URL.Path == "/readyz" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	p := &readyz.Probe{} // Path intentionally empty
	got := p.Check(context.Background(), []string{hostPort(srv)})
	if len(got) != 1 {
		t.Fatalf("expected 1 healthy (default path /readyz), got %v; hitPath=%q", got, hitPath)
	}
}

// TestProbeCheck_FailureCounterIncrements verifies that probe failures
// (non-200 responses and unreachable addresses) increment the
// RouterProbeFailuresTotal metric for the failing address.
func TestProbeCheck_FailureCounterIncrements(t *testing.T) {
	unhealthy := newUnhealthyServer(t)
	healthyAddr := hostPort(newHealthyServer(t))
	unhealthyAddr := hostPort(unhealthy)

	reg := metrics.New()
	p := &readyz.Probe{
		Path:    "/",
		Metrics: reg,
	}

	got := p.Check(context.Background(), []string{healthyAddr, unhealthyAddr})
	if len(got) != 1 {
		t.Fatalf("expected 1 healthy, got %v", got)
	}

	// The unhealthy address should have one failure recorded.
	// The healthy address must have zero failures.
	// Scrape metrics to verify.
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	reg.Handler().ServeHTTP(rw, req)
	body := rw.Body.String()

	wantFailure := `jamsesh_router_probe_failures_total{addr="` + unhealthyAddr + `"} 1`
	if !strings.Contains(body, wantFailure) {
		t.Errorf("expected probe failure metric %q in output\n--- output ---\n%s", wantFailure, body)
	}

	// Healthy address must NOT appear as a failure.
	wantNoFailure := `jamsesh_router_probe_failures_total{addr="` + healthyAddr + `"}`
	if strings.Contains(body, wantNoFailure) {
		t.Errorf("healthy address appeared in probe failure metrics: %q", wantNoFailure)
	}
}

// TestProbeCheck_UnreachableIncrementsCounter verifies that connection
// failures (address unreachable) also increment the probe failure counter.
func TestProbeCheck_UnreachableIncrementsCounter(t *testing.T) {
	unreachableAddr := "127.0.0.1:1" // nothing listens here

	reg := metrics.New()
	p := &readyz.Probe{
		Path:    "/readyz",
		Client:  &http.Client{Timeout: 100 * time.Millisecond},
		Metrics: reg,
	}

	got := p.Check(context.Background(), []string{unreachableAddr})
	if len(got) != 0 {
		t.Fatalf("expected 0 healthy for unreachable addr, got %v", got)
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	reg.Handler().ServeHTTP(rw, req)
	body := rw.Body.String()

	wantFailure := `jamsesh_router_probe_failures_total{addr="` + unreachableAddr + `"} 1`
	if !strings.Contains(body, wantFailure) {
		t.Errorf("expected probe failure metric %q in output\n--- output ---\n%s", wantFailure, body)
	}
}
