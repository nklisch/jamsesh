// Package toxiproxy — toxics.go provides typed helpers for injecting and
// removing Toxiproxy toxics and proxies via the admin HTTP API.
//
// These helpers are separate from toxiproxy.go to keep the core fixture
// focused on container lifecycle and to allow tests that need only a
// reachability check to import a minimal surface.
package toxiproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// CreateProxy creates a named Toxiproxy proxy that listens on listenAddr
// (e.g. "0.0.0.0:22222") and forwards traffic to upstreamAddr
// (e.g. "postgres-container-ip:5432").
//
// The proxy is created in the enabled state. Returns the listen address
// as reported by Toxiproxy (may differ from the requested address if the
// port was 0).
func (tp *Toxiproxy) CreateProxy(ctx context.Context, t *testing.T, name, listenAddr, upstreamAddr string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name":     name,
		"listen":   listenAddr,
		"upstream": upstreamAddr,
		"enabled":  true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		tp.AdminURL+"/proxies",
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("toxiproxy.CreateProxy: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("toxiproxy.CreateProxy: POST /proxies: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("toxiproxy.CreateProxy: status %d (want 201): %s", resp.StatusCode, respBody)
	}

	var created struct {
		Listen string `json:"listen"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		t.Fatalf("toxiproxy.CreateProxy: decode response: %v\nbody: %s", err, respBody)
	}
	return created.Listen
}

// AddLatency injects a latency toxic on proxyName. latencyMs is the base
// latency in milliseconds added to every packet in both directions (upstream
// and downstream). The toxic is given the provided name so it can be removed
// later with RemoveToxic.
func (tp *Toxiproxy) AddLatency(ctx context.Context, t *testing.T, proxyName, toxicName string, latencyMs int) {
	t.Helper()
	tp.addToxic(ctx, t, proxyName, toxicName, "latency", "upstream",
		map[string]any{"latency": latencyMs, "jitter": 0})
}

// AddBandwidthLimit injects a bandwidth-limit toxic on proxyName. rateKBs is
// the rate in KB/s. Stream must be "upstream" or "downstream".
func (tp *Toxiproxy) AddBandwidthLimit(ctx context.Context, t *testing.T, proxyName, toxicName, stream string, rateKBs int) {
	t.Helper()
	tp.addToxic(ctx, t, proxyName, toxicName, "bandwidth", stream,
		map[string]any{"rate": rateKBs})
}

// AddResetPeer injects a reset_peer toxic that abruptly closes all new
// connections to proxyName. timeoutMs is the time in milliseconds before the
// connection is reset (0 = immediate).
func (tp *Toxiproxy) AddResetPeer(ctx context.Context, t *testing.T, proxyName, toxicName string, timeoutMs int) {
	t.Helper()
	tp.addToxic(ctx, t, proxyName, toxicName, "reset_peer", "upstream",
		map[string]any{"timeout": timeoutMs})
}

// RemoveToxic deletes the named toxic from the named proxy.
func (tp *Toxiproxy) RemoveToxic(ctx context.Context, t *testing.T, proxyName, toxicName string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/proxies/%s/toxics/%s", tp.AdminURL, proxyName, toxicName),
		nil)
	if err != nil {
		t.Fatalf("toxiproxy.RemoveToxic: build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("toxiproxy.RemoveToxic: DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("toxiproxy.RemoveToxic: status %d (want 204)", resp.StatusCode)
	}
}

// addToxic is the internal helper that posts a toxic to the admin API.
func (tp *Toxiproxy) addToxic(ctx context.Context, t *testing.T, proxyName, toxicName, kind, stream string, attributes map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name":       toxicName,
		"type":       kind,
		"stream":     stream,
		"toxicity":   1.0,
		"attributes": attributes,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/proxies/%s/toxics", tp.AdminURL, proxyName),
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("toxiproxy.addToxic(%s): build request: %v", kind, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("toxiproxy.addToxic(%s): POST: %v", kind, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("toxiproxy.addToxic(%s): status %d (want 200): %s", kind, resp.StatusCode, respBody)
	}
}
