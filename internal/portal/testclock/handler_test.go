//go:build e2etest

package testclock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*httptest.Server, *AdvanceableClock) {
	t.Helper()
	clock := New()
	srv := httptest.NewServer(RouteMount(clock))
	t.Cleanup(srv.Close)
	return srv, clock
}

func TestHandler_HappyPath(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Post(srv.URL+"/clock-advance", "application/json",
		strings.NewReader(`{"advance_seconds": 900}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var body advanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.OffsetSeconds != 900 {
		t.Fatalf("offset_seconds = %d, want 900", body.OffsetSeconds)
	}
	if _, err := time.Parse(time.RFC3339Nano, body.Now); err != nil {
		t.Fatalf("now = %q is not RFC3339Nano: %v", body.Now, err)
	}
}

func TestHandler_CumulativeOffset(t *testing.T) {
	srv, _ := newTestServer(t)
	for i := 1; i <= 3; i++ {
		resp, err := http.Post(srv.URL+"/clock-advance", "application/json",
			strings.NewReader(`{"advance_seconds": 60}`))
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		var body advanceResponse
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		want := int64(60 * i)
		if body.OffsetSeconds != want {
			t.Fatalf("call %d offset = %d, want %d", i, body.OffsetSeconds, want)
		}
	}
}

func TestHandler_ZeroAdvanceIsAllowed(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Post(srv.URL+"/clock-advance", "application/json",
		strings.NewReader(`{"advance_seconds": 0}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body advanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.OffsetSeconds != 0 {
		t.Fatalf("offset_seconds = %d, want 0", body.OffsetSeconds)
	}
}

func TestHandler_NegativeRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Post(srv.URL+"/clock-advance", "application/json",
		strings.NewReader(`{"advance_seconds": -1}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandler_InvalidJSONRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Post(srv.URL+"/clock-advance", "application/json",
		strings.NewReader(`not json`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandler_NonIntegerRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Post(srv.URL+"/clock-advance", "application/json",
		strings.NewReader(`{"advance_seconds": "abc"}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandler_UnknownFieldRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Post(srv.URL+"/clock-advance", "application/json",
		strings.NewReader(`{"advance_seconds": 10, "rewind": true}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (DisallowUnknownFields)", resp.StatusCode)
	}
}

func TestHandler_MissingFieldDefaultsToZero(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Post(srv.URL+"/clock-advance", "application/json",
		strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (missing field defaults to 0)", resp.StatusCode)
	}
	var body advanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.OffsetSeconds != 0 {
		t.Fatalf("offset_seconds = %d, want 0", body.OffsetSeconds)
	}
}

// Note: RouteMount returns a path-agnostic handler — path and method
// matching are the caller's responsibility (the portal router registers
// it on POST /test/clock-advance). The unit tests below exercise the
// handler directly with arbitrary paths, since the handler ignores the
// path. Method-rejection is asserted at the integration boundary in
// cmd/portal/test_clock_advance_e2e_test.go.
