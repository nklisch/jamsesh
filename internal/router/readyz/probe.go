// Package readyz provides a parallel readiness-probe helper. It checks a
// list of pod addresses concurrently and returns the healthy subset.
package readyz

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// defaultTimeout is used when no http.Client is supplied.
const defaultTimeout = 2 * time.Second

// Probe checks the readiness endpoint on a list of addresses in parallel.
type Probe struct {
	// Client is the HTTP client used for probe requests. When nil a default
	// client with a 2-second timeout is used.
	Client *http.Client

	// Path is the readiness path probed on each address, typically "/readyz".
	Path string
}

// client returns p.Client if set, otherwise a shared default client.
func (p *Probe) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: defaultTimeout}
}

// path returns p.Path if set, otherwise "/readyz".
func (p *Probe) path() string {
	if p.Path != "" {
		return p.Path
	}
	return "/readyz"
}

// Check probes each address in addrs concurrently and returns the subset
// whose readiness endpoint responded with HTTP 200. Failures are silently
// swallowed; the contract is "healthy subset or empty slice".
//
// The overall latency is bounded by the HTTP client's Timeout (default 2 s),
// not by len(addrs), because all requests run in parallel.
func (p *Probe) Check(ctx context.Context, addrs []string) []string {
	if len(addrs) == 0 {
		return nil
	}

	client := p.client()
	path := p.path()

	type result struct {
		addr    string
		healthy bool
	}

	results := make([]result, len(addrs))
	var wg sync.WaitGroup
	wg.Add(len(addrs))

	for i, addr := range addrs {
		i, addr := i, addr // capture
		go func() {
			defer wg.Done()
			url := "http://" + addr + path
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				results[i] = result{addr: addr, healthy: false}
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				results[i] = result{addr: addr, healthy: false}
				return
			}
			resp.Body.Close()
			results[i] = result{addr: addr, healthy: resp.StatusCode == http.StatusOK}
		}()
	}

	wg.Wait()

	healthy := make([]string, 0, len(addrs))
	for _, r := range results {
		if r.healthy {
			healthy = append(healthy, r.addr)
		}
	}
	return healthy
}
