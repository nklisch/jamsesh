package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"jamsesh/internal/router/ring"
)

// K8sConfig holds the configuration for the Kubernetes endpoint-watch discoverer.
type K8sConfig struct {
	// APIServerURL is the base URL of the Kubernetes API server, e.g.
	// "https://kubernetes.default.svc" for in-cluster or the mock URL in tests.
	APIServerURL string

	// Namespace is the pod namespace, e.g. "jamsesh".
	Namespace string

	// ServiceName is the Endpoints object name (matches the Service name), e.g. "portal".
	ServiceName string

	// PortName is matched against subset port entries. Empty matches any port.
	// If non-empty only addresses paired with a matching port name are emitted.
	PortName string

	// PodPort is the port number appended to discovered pod IPs when building
	// ring.Pod.Address values. Required (non-zero).
	PodPort int

	// BearerToken is sent as "Authorization: Bearer <token>" on all requests.
	// Leave empty for unauthenticated access (e.g. against a test httptest server).
	BearerToken string

	// ResyncInterval is how often to re-list the full endpoint set even without
	// a watch event (as a fallback for missed events). Default: 30s.
	ResyncInterval time.Duration

	// HTTPClient, if non-nil, overrides the default http.Client used for all
	// requests. Useful in tests to inject an httptest.Server client.
	HTTPClient *http.Client
}

// k8sDiscoverer implements Discoverer for a Kubernetes Endpoints watch.
type k8sDiscoverer struct {
	cfg K8sConfig
}

// K8s returns a Discoverer that watches the Kubernetes Endpoints API and
// publishes the current set of healthy pod addresses to the ring. It uses a
// long-poll watch (ResourceVersion-gated) and falls back to a periodic re-list
// at cfg.ResyncInterval to recover from missed events.
//
// The discoverer does NOT probe pod /readyz — it trusts the Kubernetes
// readiness gate. Pod addresses are published as-is from the Endpoints object.
func K8s(cfg K8sConfig) Discoverer {
	if cfg.ResyncInterval <= 0 {
		cfg.ResyncInterval = 30 * time.Second
	}
	return &k8sDiscoverer{cfg: cfg}
}

// Run blocks until ctx is cancelled. It continuously re-lists the Endpoints
// object, publishes the current set, then opens a watch stream. When the watch
// delivers a MODIFIED event the new set is published. The watch is re-opened on
// error or natural close. A re-list is also triggered at ResyncInterval as a
// safety net for missed events.
func (d *k8sDiscoverer) Run(ctx context.Context, publish func([]ring.Pod)) error {
	var prevKey string // for change suppression

	doList := func() (string, []ring.Pod, error) {
		rv, addrs, err := d.list(ctx)
		if err != nil {
			return "", nil, err
		}
		pods := d.addrsToPods(addrs)
		return rv, pods, nil
	}

	publishIfChanged := func(pods []ring.Pod) {
		key := podKey(pods)
		if key == prevKey {
			return
		}
		prevKey = key
		publish(pods)
	}

	for {
		// Re-list to get current state + resourceVersion.
		rv, pods, err := doList()
		if err != nil {
			// Transient error: wait a bit and retry.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}
		publishIfChanged(pods)

		// Open a watch stream from the current resourceVersion.
		// watchCtx is cancelled after ResyncInterval to force a re-list.
		watchCtx, cancelWatch := context.WithTimeout(ctx, d.cfg.ResyncInterval)
		err = d.watch(watchCtx, rv, func(updated []ring.Pod) {
			publishIfChanged(updated)
		})
		cancelWatch()

		// ctx.Done() → real shutdown, propagate.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Any other error (or timeout) → re-list on next iteration.
		if err != nil && err != context.DeadlineExceeded {
			// Log-friendly but we have no logger dep here; continue silently.
			_ = err
		}
	}
}

// list issues GET /api/v1/namespaces/<ns>/endpoints/<svc> and returns the
// resourceVersion and the pod addresses from the current Endpoints object.
func (d *k8sDiscoverer) list(ctx context.Context) (resourceVersion string, addrs []string, err error) {
	url := fmt.Sprintf("%s/api/v1/namespaces/%s/endpoints/%s",
		d.cfg.APIServerURL, d.cfg.Namespace, d.cfg.ServiceName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", nil, fmt.Errorf("k8s discoverer: build list request: %w", err)
	}
	if d.cfg.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.cfg.BearerToken)
	}

	resp, err := d.client().Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("k8s discoverer: list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("k8s discoverer: list: unexpected status %d", resp.StatusCode)
	}

	var ep k8sEndpoints
	if err := json.NewDecoder(resp.Body).Decode(&ep); err != nil {
		return "", nil, fmt.Errorf("k8s discoverer: list: decode: %w", err)
	}

	return ep.Metadata.ResourceVersion, d.extractAddrs(ep), nil
}

// watch opens a long-poll watch stream via
//
//	GET /api/v1/namespaces/<ns>/endpoints/<svc>?watch=true&resourceVersion=<rv>
//
// and calls onUpdate for each MODIFIED or ADDED event. It returns when ctx is
// cancelled, the server closes the stream, or a JSON decode error occurs.
func (d *k8sDiscoverer) watch(ctx context.Context, resourceVersion string, onUpdate func([]ring.Pod)) error {
	url := fmt.Sprintf("%s/api/v1/namespaces/%s/endpoints/%s?watch=true&resourceVersion=%s",
		d.cfg.APIServerURL, d.cfg.Namespace, d.cfg.ServiceName, resourceVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("k8s discoverer: build watch request: %w", err)
	}
	if d.cfg.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.cfg.BearerToken)
	}

	resp, err := d.client().Do(req)
	if err != nil {
		return fmt.Errorf("k8s discoverer: watch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("k8s discoverer: watch: unexpected status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event k8sWatchEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return fmt.Errorf("k8s discoverer: watch: decode event: %w", err)
		}
		switch event.Type {
		case "ADDED", "MODIFIED":
			addrs := d.extractAddrs(event.Object)
			pods := d.addrsToPods(addrs)
			onUpdate(pods)
		case "DELETED":
			onUpdate(nil)
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("k8s discoverer: watch: scan: %w", err)
	}
	return io.EOF
}

// extractAddrs pulls pod IPs from the Endpoints subset matching cfg.PortName
// (or any port if PortName is empty) and returns "ip:podPort" strings.
func (d *k8sDiscoverer) extractAddrs(ep k8sEndpoints) []string {
	var addrs []string
	for _, subset := range ep.Subsets {
		if !d.subsetMatchesPort(subset) {
			continue
		}
		for _, addr := range subset.Addresses {
			if addr.IP != "" {
				addrs = append(addrs, fmt.Sprintf("%s:%d", addr.IP, d.cfg.PodPort))
			}
		}
	}
	return addrs
}

// subsetMatchesPort returns true if cfg.PortName is empty (match all) or if
// one of the subset's ports has a Name matching cfg.PortName.
func (d *k8sDiscoverer) subsetMatchesPort(subset k8sSubset) bool {
	if d.cfg.PortName == "" {
		return true
	}
	for _, p := range subset.Ports {
		if p.Name == d.cfg.PortName {
			return true
		}
	}
	return false
}

// addrsToPods converts "host:port" address strings to ring.Pod values.
// The Pod ID is the address itself (stable, unique per endpoint).
func (d *k8sDiscoverer) addrsToPods(addrs []string) []ring.Pod {
	pods := make([]ring.Pod, len(addrs))
	for i, a := range addrs {
		pods[i] = ring.Pod{ID: a, Address: a}
	}
	return pods
}

// client returns the configured HTTP client, or http.DefaultClient.
func (d *k8sDiscoverer) client() *http.Client {
	if d.cfg.HTTPClient != nil {
		return d.cfg.HTTPClient
	}
	return http.DefaultClient
}

// podKey returns a canonical change-detection key for a pod slice.
func podKey(pods []ring.Pod) string {
	addrs := make([]string, len(pods))
	for i, p := range pods {
		addrs[i] = p.ID
	}
	return joinSorted(addrs)
}

// ── Minimal k8s API types ──────────────────────────────────────────────────

type k8sEndpoints struct {
	Metadata k8sObjectMeta `json:"metadata"`
	Subsets  []k8sSubset   `json:"subsets"`
}

type k8sObjectMeta struct {
	ResourceVersion string `json:"resourceVersion"`
}

type k8sSubset struct {
	Addresses []k8sAddress `json:"addresses"`
	Ports     []k8sPort    `json:"ports"`
}

type k8sAddress struct {
	IP string `json:"ip"`
}

type k8sPort struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

type k8sWatchEvent struct {
	Type   string       `json:"type"`
	Object k8sEndpoints `json:"object"`
}
