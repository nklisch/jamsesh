package discovery_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"jamsesh/internal/router/discovery"
	"jamsesh/internal/router/readyz"
	"jamsesh/internal/router/ring"
)

// toggleServer is a test HTTP server whose response code can be toggled at
// runtime — useful for simulating a pod that becomes healthy mid-test.
type toggleServer struct {
	mu      sync.Mutex
	healthy bool
	srv     *httptest.Server
}

func newToggleServer(t *testing.T, initiallyHealthy bool) *toggleServer {
	t.Helper()
	ts := &toggleServer{healthy: initiallyHealthy}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		h := ts.healthy
		ts.mu.Unlock()
		if h {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func (ts *toggleServer) setHealthy(v bool) {
	ts.mu.Lock()
	ts.healthy = v
	ts.mu.Unlock()
}

func (ts *toggleServer) addr() string {
	return strings.TrimPrefix(ts.srv.URL, "http://")
}

// collectPublished runs the discoverer in the background and collects all
// published pod slices until the context is done (or the test helper stops it).
type collector struct {
	mu   sync.Mutex
	sets [][]ring.Pod
	ch   chan []ring.Pod
}

func newCollector() *collector {
	c := &collector{ch: make(chan []ring.Pod, 32)}
	go func() {
		for pods := range c.ch {
			c.mu.Lock()
			c.sets = append(c.sets, pods)
			c.mu.Unlock()
		}
	}()
	return c
}

func (c *collector) publish(pods []ring.Pod) {
	cp := make([]ring.Pod, len(pods))
	copy(cp, pods)
	c.ch <- cp
}

func (c *collector) close() { close(c.ch) }

func (c *collector) last() []ring.Pod {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sets) == 0 {
		return nil
	}
	return c.sets[len(c.sets)-1]
}

func (c *collector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sets)
}

// podAddrs extracts sorted addresses from a pod slice for comparison.
func podAddrs(pods []ring.Pod) []string {
	addrs := make([]string, len(pods))
	for i, p := range pods {
		addrs[i] = p.Address
	}
	sort.Strings(addrs)
	return addrs
}

// TestStaticDiscoverer_AllHealthy verifies that when all addrs are healthy, all
// are published.
func TestStaticDiscoverer_AllHealthy(t *testing.T) {
	s1 := newToggleServer(t, true)
	s2 := newToggleServer(t, true)

	probe := &readyz.Probe{Path: "/"}
	d := discovery.Static([]string{s1.addr(), s2.addr()}, probe, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	coll := newCollector()
	defer coll.close()

	done := make(chan error, 1)
	go func() { done <- d.Run(ctx, coll.publish) }()

	// Wait for at least one publish (first tick pass).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if coll.count() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	addrs := podAddrs(coll.last())
	if len(addrs) != 2 {
		t.Fatalf("expected 2 healthy pods, got %v", addrs)
	}

	cancel()
	<-done
}

// TestStaticDiscoverer_MixedHealth verifies that only healthy addrs appear in
// the published set.
func TestStaticDiscoverer_MixedHealth(t *testing.T) {
	healthy := newToggleServer(t, true)
	unhealthy := newToggleServer(t, false)

	probe := &readyz.Probe{Path: "/"}
	d := discovery.Static([]string{healthy.addr(), unhealthy.addr()}, probe, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	coll := newCollector()
	defer coll.close()

	go d.Run(ctx, coll.publish) //nolint:errcheck

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if coll.count() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	addrs := podAddrs(coll.last())
	if len(addrs) != 1 {
		t.Fatalf("expected 1 healthy pod, got %v", addrs)
	}
	if addrs[0] != healthy.addr() {
		t.Fatalf("expected healthy addr %q, got %q", healthy.addr(), addrs[0])
	}

	cancel()
}

// TestStaticDiscoverer_BecomesHealthy verifies that an addr that starts
// unhealthy and then becomes healthy is published on the next poll pass.
func TestStaticDiscoverer_BecomesHealthy(t *testing.T) {
	toggler := newToggleServer(t, false) // starts unhealthy

	probe := &readyz.Probe{
		Path:   "/",
		Client: &http.Client{Timeout: 500 * time.Millisecond},
	}
	const interval = 50 * time.Millisecond
	d := discovery.Static([]string{toggler.addr()}, probe, interval)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coll := newCollector()
	defer coll.close()

	go d.Run(ctx, coll.publish) //nolint:errcheck

	// Wait for first publish (first tick, unhealthy → empty set).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if coll.count() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if coll.count() == 0 {
		t.Fatal("no publish within deadline after first tick")
	}
	if len(coll.last()) != 0 {
		t.Fatalf("expected empty initial publish (unhealthy), got %v", coll.last())
	}

	// Flip the server to healthy.
	toggler.setHealthy(true)

	// Wait for the next publish that includes the now-healthy addr.
	prevCount := coll.count()
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if coll.count() > prevCount && len(coll.last()) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	addrs := podAddrs(coll.last())
	if len(addrs) != 1 {
		t.Fatalf("expected 1 healthy pod after flip, got %v", addrs)
	}
	if addrs[0] != toggler.addr() {
		t.Fatalf("expected addr %q, got %q", toggler.addr(), addrs[0])
	}

	cancel()
}

// TestStaticDiscoverer_NoSpuriousPublish verifies that publish is not called
// again when the healthy set hasn't changed.
func TestStaticDiscoverer_NoSpuriousPublish(t *testing.T) {
	s := newToggleServer(t, true)

	probe := &readyz.Probe{Path: "/"}
	const interval = 30 * time.Millisecond
	d := discovery.Static([]string{s.addr()}, probe, interval)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coll := newCollector()
	defer coll.close()

	go d.Run(ctx, coll.publish) //nolint:errcheck

	<-ctx.Done()

	// There should be exactly 1 publish (first tick), not one per tick, since
	// the set never changed.
	if coll.count() != 1 {
		t.Fatalf("expected exactly 1 publish (no-change suppression), got %d", coll.count())
	}
}

// TestStaticDiscoverer_PodIDEqualsAddr verifies the Pod ID equals the addr
// for static mode (deterministic and useful as a ring key).
func TestStaticDiscoverer_PodIDEqualsAddr(t *testing.T) {
	s := newToggleServer(t, true)

	probe := &readyz.Probe{Path: "/"}
	d := discovery.Static([]string{s.addr()}, probe, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	coll := newCollector()
	defer coll.close()

	go d.Run(ctx, coll.publish) //nolint:errcheck

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if coll.count() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	pods := coll.last()
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got %v", pods)
	}
	if pods[0].ID != s.addr() {
		t.Fatalf("expected Pod.ID == addr %q, got %q", s.addr(), pods[0].ID)
	}

	cancel()
}

// TestStaticDiscoverer_NoImmediateProbeOnStartup verifies that the discoverer
// does NOT publish on startup before the first tick interval elapses.
//
// This is the regression test for the "readyz evicts seeded ring" bug: the
// router seeds the consistent-hash ring with all configured pod addresses at
// startup, then launches the discovery goroutine. If the discoverer probed
// immediately and all portals were still completing their readiness checks
// (Postgres ping + os.Stat), publish([]) would clear the seeded ring and all
// requests would 503 until the next probe interval.
//
// The fix: wait for the first tick before probing. This test confirms no
// publish fires before that interval elapses, regardless of backend health.
func TestStaticDiscoverer_NoImmediateProbeOnStartup(t *testing.T) {
	// Start the backend as unhealthy — simulates portals that haven't completed
	// their /readyz checks yet (Postgres ping, os.Stat) at the moment the
	// discovery goroutine starts.
	backend := newToggleServer(t, false)

	const interval = 200 * time.Millisecond
	probe := &readyz.Probe{Path: "/"}
	d := discovery.Static([]string{backend.addr()}, probe, interval)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coll := newCollector()
	defer coll.close()

	go d.Run(ctx, coll.publish) //nolint:errcheck

	// Sleep for half the interval — no publish should have fired yet because
	// the discoverer waits for the first tick before probing.
	time.Sleep(interval / 2)

	if coll.count() != 0 {
		t.Fatalf("discoverer published %d time(s) before first tick interval elapsed; "+
			"expected 0 — pre-tick probe must not evict the seeded ring", coll.count())
	}

	// After the full interval the first probe fires. The backend is still
	// unhealthy, so publish([]) is called exactly once (empty set).
	time.Sleep(interval) // wait past the first tick

	if coll.count() != 1 {
		t.Fatalf("expected exactly 1 publish after first tick, got %d", coll.count())
	}
	if len(coll.last()) != 0 {
		t.Fatalf("expected empty publish (unhealthy backend), got %v", coll.last())
	}

	cancel()
}

// TestStaticDiscoverer_CancelStopsRun verifies that Run returns promptly on
// context cancellation.
func TestStaticDiscoverer_CancelStopsRun(t *testing.T) {
	s := newToggleServer(t, true)

	probe := &readyz.Probe{Path: "/"}
	d := discovery.Static([]string{s.addr()}, probe, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- d.Run(ctx, func([]ring.Pod) {}) }()

	cancel()

	select {
	case <-done:
		// ok — Run returned
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
