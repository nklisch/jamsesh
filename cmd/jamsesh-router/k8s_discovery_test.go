// TestRouter_K8sDiscovery_NewPodPickedUp exercises the k8s-mode discovery path.
//
// Design rationale:
//
// The Kubernetes Endpoints model maps N pod IPs to a single containerPort. A
// real Deployment has all pods listen on the same port. httptest.Server binds
// random ports, so we cannot use multiple httptest.Server instances each as a
// distinct "pod IP" through a single PodPort — the ports differ.
//
// Solution: use the loopback address family. httptest.Server always binds
// 127.0.0.1. We run two backends on 127.0.0.1 (same address, different ports).
// The k8s stub announces ONLY THEIR IPs. The discoverer appends PodPort to
// each IP. So the ring address is "127.0.0.1:<PodPort>" — which maps to
// exactly one backend at a time. This is fine: we only need to verify that the
// ring CONTAINS a discovered address, not that the router can actually proxy to
// two distinct hosts simultaneously via a single PodPort.
//
// The test therefore verifies the discovery invariant:
//
//	When the k8s stub announces a new IP, the discoverer publishes a pod set
//	that includes "newIP:PodPort" within the SLO deadline.
//
// It exercises internal/router/discovery/k8s.go directly (not via runCtx).
// The routing-through-the-ring path is covered by the static-mode e2e tests.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"jamsesh/internal/router/discovery"
	"jamsesh/internal/router/ring"
)

// ── k8s API stub ──────────────────────────────────────────────────────────────

// k8sStub is an httptest.Server that speaks a minimal subset of the Kubernetes
// Endpoints API:
//
//	GET  /api/v1/namespaces/<ns>/endpoints/<svc>               → current state
//	GET  /api/v1/namespaces/<ns>/endpoints/<svc>?watch=true    → long-poll stream
//
// Concurrency-safe; call SetIPs to update the discovered pod IP list.
type k8sStub struct {
	mu      sync.Mutex
	ips     []string     // pod IPs currently in the endpoint list (no port)
	rv      int          // monotonic resourceVersion
	watches []*watchConn // active long-poll connections
	srv     *httptest.Server
}

type watchConn struct {
	ch     chan []string // receives updated IP lists on SetIPs
	done   bool
}

// newK8sStub creates and starts the stub, registering t.Cleanup.
func newK8sStub(t *testing.T, initialIPs []string) *k8sStub {
	t.Helper()
	s := &k8sStub{
		ips: append([]string{}, initialIPs...),
		rv:  1,
	}
	s.srv = httptest.NewServer(s)
	t.Cleanup(s.srv.Close)
	return s
}

// URL returns the base URL of the stub.
func (s *k8sStub) URL() string { return s.srv.URL }

// SetIPs atomically replaces the pod IP list and delivers a MODIFIED watch event
// to all active long-poll connections.
func (s *k8sStub) SetIPs(ips []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ips = append([]string{}, ips...)
	s.rv++
	for _, w := range s.watches {
		if !w.done {
			select {
			case w.ch <- append([]string{}, s.ips...):
			default:
			}
		}
	}
}

func (s *k8sStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query().Get("watch")
	if q == "true" || q == "1" {
		s.serveWatch(w, r)
		return
	}
	s.serveList(w)
}

func (s *k8sStub) serveList(w http.ResponseWriter) {
	s.mu.Lock()
	ips := append([]string{}, s.ips...)
	rv := strconv.Itoa(s.rv)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(buildK8sEndpoints(ips, rv))
}

func (s *k8sStub) serveWatch(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	conn := &watchConn{ch: make(chan []string, 16)}
	s.mu.Lock()
	s.watches = append(s.watches, conn)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		conn.done = true
		updated := s.watches[:0]
		for _, c := range s.watches {
			if c != conn {
				updated = append(updated, c)
			}
		}
		s.watches = updated
		s.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)
	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case ips, ok := <-conn.ch:
			if !ok {
				return
			}
			s.mu.Lock()
			rv := strconv.Itoa(s.rv)
			s.mu.Unlock()

			event := map[string]any{
				"type":   "MODIFIED",
				"object": buildK8sEndpoints(ips, rv),
			}
			if err := enc.Encode(event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// buildK8sEndpoints constructs a minimal Kubernetes Endpoints JSON payload.
// ips is a slice of bare IP addresses (no port).
func buildK8sEndpoints(ips []string, rv string) map[string]any {
	addresses := make([]map[string]any, 0, len(ips))
	for _, ip := range ips {
		addresses = append(addresses, map[string]any{"ip": ip})
	}
	return map[string]any{
		"kind":       "Endpoints",
		"apiVersion": "v1",
		"metadata":   map[string]any{"resourceVersion": rv},
		"subsets": []map[string]any{
			{
				"addresses": addresses,
				"ports":     []map[string]any{{"port": 8443, "name": "http"}},
			},
		},
	}
}

// ── Test ──────────────────────────────────────────────────────────────────────

// TestRouter_K8sDiscovery_NewPodPickedUp verifies the k8s discovery invariant:
// when a new pod IP appears in the Endpoints API, the discoverer publishes a
// pod set containing "newIP:PodPort" within a 15 s SLO.
//
// The test exercises the production discovery.K8s code directly. The ring
// integration (routing through the discovered pods) is exercised by the
// static-mode e2e tests; this test is scoped to the discovery seam.
func TestRouter_K8sDiscovery_NewPodPickedUp(t *testing.T) {
	const (
		namespace   = "jamsesh"
		serviceName = "portal"
		podPort     = 8443 // fixed per Kubernetes container-port convention
	)

	// We use two distinct loopback IPs to simulate two distinct pods. In real k8s
	// each pod has a unique IP; in tests we use 127.0.0.1 and 127.0.0.2.
	// Note: 127.0.0.2 is a valid loopback address on Linux (any 127.x.x.x works).
	// On macOS 127.0.0.2 may require "sudo ifconfig lo0 alias 127.0.0.2"; we
	// skip gracefully if the second IP is not usable.
	//
	// Simpler: since we are testing the DISCOVERY layer (what the discoverer
	// publishes to the ring), not actual proxying, we only need to verify that
	// the ring.Pod slice contains the expected address. The discoverer trusts the
	// k8s API — it does not probe each discovered IP. So using any IP string
	// (even non-routable ones) in the k8s stub is valid for this invariant.
	const (
		podIP1 = "10.0.0.1" // fictional pod IPs — only used in ring membership
		podIP2 = "10.0.0.2"
		podIP3 = "10.0.0.3"
	)

	// ── Fake k8s API: start with 2 pods ─────────────────────────────────────

	k8s := newK8sStub(t, []string{podIP1, podIP2})

	// ── Configure the k8s discoverer ─────────────────────────────────────────

	cfg := discovery.K8sConfig{
		APIServerURL:   k8s.URL(),
		Namespace:      namespace,
		ServiceName:    serviceName,
		PodPort:        podPort,
		ResyncInterval: 3 * time.Second, // fast resync so test doesn't wait 30 s
		HTTPClient:     k8s.srv.Client(), // reuse the httptest TLS-free client
	}
	disc := discovery.K8s(cfg)

	// ── Collect published pod sets ────────────────────────────────────────────

	var (
		mu       sync.Mutex
		latest   []ring.Pod
		publishC = make(chan struct{}, 64)
	)
	publish := func(pods []ring.Pod) {
		mu.Lock()
		latest = pods
		mu.Unlock()
		select {
		case publishC <- struct{}{}:
		default:
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = disc.Run(ctx, publish)
	}()

	// ── Phase 1: verify initial set {pod1, pod2} is published ────────────────

	wantInitial := []string{
		fmt.Sprintf("%s:%d", podIP1, podPort),
		fmt.Sprintf("%s:%d", podIP2, podPort),
	}
	if !waitForPodSet(publishC, &mu, &latest, wantInitial, 10*time.Second) {
		mu.Lock()
		cur := podAddresses(latest)
		mu.Unlock()
		t.Fatalf("k8s discovery: initial set not published within 10 s; got %v, want %v",
			cur, wantInitial)
	}
	t.Logf("k8s discovery: initial set {pod1, pod2} published ✓")

	// ── Phase 2: add pod3 and verify discovery within SLO ────────────────────

	k8s.SetIPs([]string{podIP1, podIP2, podIP3})

	pod3Addr := fmt.Sprintf("%s:%d", podIP3, podPort)
	const slo = 15 * time.Second
	if !waitForPodAddr(publishC, &mu, &latest, pod3Addr, slo) {
		mu.Lock()
		cur := podAddresses(latest)
		mu.Unlock()
		t.Fatalf("k8s discovery: pod3 %q not discovered within %s SLO; current set: %v",
			pod3Addr, slo, cur)
	}
	t.Logf("k8s discovery: new pod %q discovered within SLO ✓", pod3Addr)

	// Verify pod1 and pod2 are still present after the update.
	mu.Lock()
	cur := podAddresses(latest)
	mu.Unlock()
	for _, want := range []string{
		fmt.Sprintf("%s:%d", podIP1, podPort),
		fmt.Sprintf("%s:%d", podIP2, podPort),
	} {
		found := false
		for _, a := range cur {
			if a == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("k8s discovery: pod %q missing from set after adding pod3; current: %v", want, cur)
		}
	}
	t.Logf("k8s discovery: full set after pod3 addition: %v ✓", cur)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// podAddresses extracts Address fields from a pod slice.
func podAddresses(pods []ring.Pod) []string {
	out := make([]string, len(pods))
	for i, p := range pods {
		out[i] = p.Address
	}
	return out
}

// waitForPodSet polls publishC until the published pod set contains ALL addresses
// in want (no extras required). Returns true within timeout.
func waitForPodSet(ch <-chan struct{}, mu *sync.Mutex, latest *[]ring.Pod, want []string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ch:
		case <-time.After(100 * time.Millisecond):
		}
		if podSetContainsAll(mu, latest, want) {
			return true
		}
	}
	return false
}

// waitForPodAddr polls publishC until the published pod set contains addr.
func waitForPodAddr(ch <-chan struct{}, mu *sync.Mutex, latest *[]ring.Pod, addr string, timeout time.Duration) bool {
	return waitForPodSet(ch, mu, latest, []string{addr}, timeout)
}

// podSetContainsAll returns true if *latest contains all addresses in want.
func podSetContainsAll(mu *sync.Mutex, latest *[]ring.Pod, want []string) bool {
	mu.Lock()
	pods := append([]ring.Pod{}, *latest...)
	mu.Unlock()
	for _, w := range want {
		found := false
		for _, p := range pods {
			if p.Address == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

