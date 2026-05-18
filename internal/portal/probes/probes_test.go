package probes_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jamsesh/internal/portal/probes"
)

// decodeResponse unmarshals the handler's JSON response into a generic map
// so tests can inspect status and per-check fields without tying to private types.
func decodeResponse(t *testing.T, body string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, body)
	}
	return m
}

func getChecks(t *testing.T, m map[string]interface{}) []interface{} {
	t.Helper()
	raw, ok := m["checks"]
	if !ok {
		t.Fatal("response missing 'checks' field")
	}
	checks, ok := raw.([]interface{})
	if !ok {
		t.Fatalf("'checks' is not an array: %T", raw)
	}
	return checks
}

func checkField(t *testing.T, c interface{}, field string) interface{} {
	t.Helper()
	m, ok := c.(map[string]interface{})
	if !ok {
		t.Fatalf("check entry is not an object: %T", c)
	}
	return m[field]
}

// serve runs the handler against a synthetic GET request and returns the
// recorded response.
func serve(h http.Handler) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	h.ServeHTTP(w, r)
	return w
}

// ---------------------------------------------------------------------------
// All checks pass
// ---------------------------------------------------------------------------

func TestAllOK(t *testing.T) {
	h := probes.Handler([]probes.Check{
		{Name: "alpha", Fn: func(_ context.Context) error { return nil }},
		{Name: "beta", Fn: func(_ context.Context) error { return nil }},
	})

	w := serve(h)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("want json content-type, got %q", ct)
	}

	m := decodeResponse(t, w.Body.String())
	if m["status"] != "ready" {
		t.Errorf("want status=ready, got %q", m["status"])
	}

	checks := getChecks(t, m)
	if len(checks) != 2 {
		t.Fatalf("want 2 check entries, got %d", len(checks))
	}
	for _, c := range checks {
		if checkField(t, c, "ok") != true {
			t.Errorf("check %q: want ok=true, got %v", checkField(t, c, "name"), checkField(t, c, "ok"))
		}
		if checkField(t, c, "error") != nil {
			t.Errorf("check %q: want no error field, got %v", checkField(t, c, "name"), checkField(t, c, "error"))
		}
	}
}

// ---------------------------------------------------------------------------
// One check fails
// ---------------------------------------------------------------------------

func TestOneFail(t *testing.T) {
	h := probes.Handler([]probes.Check{
		{Name: "ok-check", Fn: func(_ context.Context) error { return nil }},
		{Name: "bad-check", Fn: func(_ context.Context) error { return errors.New("disk full") }},
	})

	w := serve(h)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}

	m := decodeResponse(t, w.Body.String())
	if m["status"] != "not_ready" {
		t.Errorf("want status=not_ready, got %q", m["status"])
	}

	checks := getChecks(t, m)
	if len(checks) != 2 {
		t.Fatalf("want 2 check entries, got %d", len(checks))
	}

	// Locate checks by name.
	byName := make(map[string]map[string]interface{}, 2)
	for _, c := range checks {
		cm := c.(map[string]interface{})
		byName[cm["name"].(string)] = cm
	}

	if byName["ok-check"]["ok"] != true {
		t.Errorf("ok-check: want ok=true")
	}
	if byName["ok-check"]["error"] != nil {
		t.Errorf("ok-check: want no error field")
	}
	if byName["bad-check"]["ok"] != false {
		t.Errorf("bad-check: want ok=false")
	}
	if byName["bad-check"]["error"] != "disk full" {
		t.Errorf("bad-check: want error='disk full', got %v", byName["bad-check"]["error"])
	}
}

// ---------------------------------------------------------------------------
// All checks fail
// ---------------------------------------------------------------------------

func TestAllFail(t *testing.T) {
	h := probes.Handler([]probes.Check{
		{Name: "a", Fn: func(_ context.Context) error { return errors.New("err-a") }},
		{Name: "b", Fn: func(_ context.Context) error { return errors.New("err-b") }},
	})

	w := serve(h)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}

	m := decodeResponse(t, w.Body.String())
	if m["status"] != "not_ready" {
		t.Errorf("want status=not_ready, got %q", m["status"])
	}

	checks := getChecks(t, m)
	for _, c := range checks {
		if checkField(t, c, "ok") != false {
			t.Errorf("check %q: want ok=false", checkField(t, c, "name"))
		}
		if checkField(t, c, "error") == nil {
			t.Errorf("check %q: want error field present", checkField(t, c, "name"))
		}
	}
}

// ---------------------------------------------------------------------------
// Timeout
// ---------------------------------------------------------------------------

func TestTimeout(t *testing.T) {
	// Block forever — the 2-second probe timeout should fire and report "timeout".
	// We replace the check timeout with a much shorter duration via a blocking
	// function that never returns before the context deadline.
	h := probes.Handler([]probes.Check{
		{Name: "slow", Fn: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}},
	})

	// Wrap the request context with an even shorter deadline so the test
	// doesn't actually wait 2 seconds. The handler propagates this context
	// to each check, so ctx.Done() fires immediately.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Millisecond)
	defer cancel()
	r = r.WithContext(ctx)

	h.ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}

	m := decodeResponse(t, w.Body.String())
	if m["status"] != "not_ready" {
		t.Errorf("want status=not_ready, got %q", m["status"])
	}

	checks := getChecks(t, m)
	if len(checks) != 1 {
		t.Fatalf("want 1 check, got %d", len(checks))
	}
	if checkField(t, checks[0], "error") != "timeout" {
		t.Errorf("want error=timeout, got %v", checkField(t, checks[0], "error"))
	}
}

// ---------------------------------------------------------------------------
// Parallel timing — N checks each sleeping T total ≤ T + margin, not N*T.
// ---------------------------------------------------------------------------

func TestParallelTiming(t *testing.T) {
	const sleep = 100 * time.Millisecond
	const n = 3

	checks := make([]probes.Check, n)
	for i := range checks {
		checks[i] = probes.Check{
			Name: "slow",
			Fn: func(_ context.Context) error {
				time.Sleep(sleep)
				return nil
			},
		}
	}

	h := probes.Handler(checks)

	start := time.Now()
	w := serve(h)
	elapsed := time.Since(start)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	// If checks ran sequentially the total would be ≥ n*sleep (300ms).
	// Parallel execution should finish in roughly sleep + small overhead (≤ 1.5× sleep).
	limit := sleep + sleep/2 // 150ms margin
	if elapsed > limit {
		t.Errorf("checks appear sequential: elapsed %v, want ≤ %v (parallel limit)", elapsed, limit)
	}
}

// ---------------------------------------------------------------------------
// Empty check list — should return 200 ready with an empty checks array.
// ---------------------------------------------------------------------------

func TestEmpty(t *testing.T) {
	h := probes.Handler(nil)

	w := serve(h)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	m := decodeResponse(t, w.Body.String())
	if m["status"] != "ready" {
		t.Errorf("want status=ready, got %q", m["status"])
	}
}
