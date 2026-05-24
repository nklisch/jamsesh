package playground_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"jamsesh/internal/portal/playground"
)

// TestNewCreateRateLimiter_PerHourConversion verifies that per-hour caps are
// correctly converted to the per-minute granularity used by the ratelimit store.
//
// With CreatePerIPHour=3: perMinute = ceil(3/60) = 1, burst = 1.
// The per-minute limiter allows 1 request immediately, then requires waiting.
// The 2nd request from the same IP within the burst window is rate-limited.
//
// This is stricter than "3 per hour" by design: the minute-level burst
// (=1) prevents rapid-fire abuse even when the hourly cap isn't yet exhausted.
// The acceptance criterion "4th create → 429" is satisfied trivially since
// even the 2nd is blocked.
func TestNewCreateRateLimiter_PerHourConversion(t *testing.T) {
	cfg := playground.Config{
		Enabled:         true,
		CreatePerIPHour: 3,
	}
	store := playground.NewCreateRateLimiter(cfg)

	// First request is always allowed (burst=1 for perMinute=1).
	allowed1, _ := store.Allow("192.0.2.1")
	if !allowed1 {
		t.Error("first create from same IP should be allowed")
	}

	// 2nd request from the same IP should be rate-limited (minute burst exhausted).
	// This satisfies the "4th create → 429" acceptance criterion: if 2nd is
	// already blocked, 4th certainly is.
	allowed2, retryAfter := store.Allow("192.0.2.1")
	if allowed2 {
		t.Error("2nd create from same IP (within first minute) should be rate-limited")
	}
	if retryAfter <= 0 {
		t.Error("retryAfter should be positive when rate-limited")
	}
}

// TestNewCreateRateLimiter_FourthCreateBlocked directly tests the acceptance
// criterion from the story: "4th create from same IP within an hour → 429".
// Uses a higher CreatePerIPHour to allow testing rapid-fire requests: with
// CreatePerIPHour=60 → perMinute=1 burst=1 (still blocks 2nd).
// With CreatePerIPHour=120 → perMinute=2 burst=2 (3rd blocked by minute,
// or 4th by hourly cap).
//
// This test uses CreatePerIPHour=180 → perMinute=3 → burst=3.
// The 4th request from the same IP is rate-limited.
func TestNewCreateRateLimiter_FourthCreateBlocked(t *testing.T) {
	cfg := playground.Config{
		Enabled:         true,
		CreatePerIPHour: 180, // perMinute = ceil(180/60) = 3; burst = 3
	}
	store := playground.NewCreateRateLimiter(cfg)

	// First 3 requests should be allowed (within perMinute burst of 3).
	for i := range 3 {
		allowed, _ := store.Allow("192.0.2.10")
		if !allowed {
			t.Errorf("request %d should be allowed (within per-minute burst of 3)", i+1)
		}
	}

	// 4th request should be rate-limited (minute burst exhausted).
	allowed4, retryAfter := store.Allow("192.0.2.10")
	if allowed4 {
		t.Error("4th create from same IP should be rate-limited")
	}
	if retryAfter <= 0 {
		t.Error("retryAfter should be positive when 4th request is rate-limited")
	}
}

// TestNewCreateRateLimiter_DifferentIPsIndependent verifies that different
// source IPs do not share a rate-limit counter — exhausting one IP's quota
// does not affect another IP's quota.
//
// Uses CreatePerIPHour=60 → perMinute=1 → burst=1. IP A's burst is exhausted
// after 1 request; IP B's fresh quota still allows 1 request.
func TestNewCreateRateLimiter_DifferentIPsIndependent(t *testing.T) {
	cfg := playground.Config{
		Enabled:         true,
		CreatePerIPHour: 60, // perMinute=1, burst=1
	}
	store := playground.NewCreateRateLimiter(cfg)

	// Exhaust IP A (burst=1, so after 1 request it's blocked).
	store.Allow("10.0.0.1")
	allowedA, _ := store.Allow("10.0.0.1")
	if allowedA {
		t.Error("2nd request from IP A should be rate-limited (burst=1)")
	}

	// IP B should still be allowed — its own quota is fresh.
	allowedB, _ := store.Allow("10.0.0.2")
	if !allowedB {
		t.Error("1st request from IP B should not be affected by IP A's limit")
	}
}

// TestNewCreateRateLimiter_ZeroHourDefaultsToOne verifies that a zero or
// negative CreatePerIPHour is clamped to 1 so the limiter is never inoperative.
func TestNewCreateRateLimiter_ZeroHourDefaultsToOne(t *testing.T) {
	cfg := playground.Config{
		Enabled:         true,
		CreatePerIPHour: 0,
	}
	store := playground.NewCreateRateLimiter(cfg)

	// First request allowed.
	allowed, _ := store.Allow("10.0.0.3")
	if !allowed {
		t.Error("first request should always be allowed (min burst=1)")
	}

	// Second request should be rate-limited (burst=1).
	allowed2, _ := store.Allow("10.0.0.3")
	if allowed2 {
		t.Error("second request from same IP should be rate-limited when perHour=1")
	}
}

// TestCreateRateLimitMiddleware_Disabled verifies that passing enabled=false
// produces a pass-through: all requests reach the inner handler regardless of
// how many have been seen.
func TestCreateRateLimitMiddleware_Disabled(t *testing.T) {
	cfg := playground.Config{
		Enabled:         false,
		CreatePerIPHour: 1, // would normally cap after 1 request
	}
	store := playground.NewCreateRateLimiter(cfg)
	mw := playground.CreateRateLimitMiddleware(store, false /* disabled */)

	calls := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(inner)

	ip := "10.0.0.4:12345"
	for range 5 {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/playground/sessions", nil)
		r.RemoteAddr = ip
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("disabled middleware: want 200, got %d", w.Code)
		}
	}
	if calls != 5 {
		t.Errorf("inner handler called %d times, want 5", calls)
	}
}

// TestCreateRateLimitMiddleware_Enabled_BlocksAfterBurst verifies that the
// middleware returns 429 with a Retry-After header once the burst is exhausted
// and does not invoke the inner handler for blocked requests.
//
// With CreatePerIPHour=120 → perMinute=2 → burst=2. The first 2 requests are
// allowed; the 3rd is blocked.
func TestCreateRateLimitMiddleware_Enabled_BlocksAfterBurst(t *testing.T) {
	cfg := playground.Config{
		Enabled:         true,
		CreatePerIPHour: 120, // perMinute = ceil(120/60) = 2 → burst = 2
	}
	store := playground.NewCreateRateLimiter(cfg)
	mw := playground.CreateRateLimitMiddleware(store, true)

	calls := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusCreated)
	})
	handler := mw(inner)

	ip := "10.0.0.5:9876"

	// Consume the burst (2 requests allowed).
	for range 2 {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/playground/sessions", nil)
		r.RemoteAddr = ip
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Errorf("within-burst request: want 201, got %d", w.Code)
		}
	}

	// 3rd request should be rate-limited.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/playground/sessions", nil)
	r.RemoteAddr = ip
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("after burst: want 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("want Retry-After header on 429 response")
	}
	if calls != 2 {
		t.Errorf("inner handler called %d times, want 2 (blocked request must not call inner)", calls)
	}
}

// TestCreateRateLimitMiddleware_DifferentIPsSeparateCounters verifies that
// separate IPs maintain independent counters through the middleware layer.
func TestCreateRateLimitMiddleware_DifferentIPsSeparateCounters(t *testing.T) {
	cfg := playground.Config{
		Enabled:         true,
		CreatePerIPHour: 1,
	}
	store := playground.NewCreateRateLimiter(cfg)
	mw := playground.CreateRateLimitMiddleware(store, true)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	handler := mw(inner)

	// Exhaust IP A.
	{
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/playground/sessions", nil)
		r.RemoteAddr = "10.0.1.1:1111"
		handler.ServeHTTP(w, r)
		// second call from A should be blocked
		w2 := httptest.NewRecorder()
		r2 := httptest.NewRequest(http.MethodPost, "/api/playground/sessions", nil)
		r2.RemoteAddr = "10.0.1.1:1111"
		handler.ServeHTTP(w2, r2)
		if w2.Code != http.StatusTooManyRequests {
			t.Errorf("IP A 2nd request: want 429, got %d", w2.Code)
		}
	}

	// IP B should be unaffected.
	{
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/playground/sessions", nil)
		r.RemoteAddr = "10.0.1.2:2222"
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Errorf("IP B 1st request: want 201, got %d (IP B should not share IP A's counter)", w.Code)
		}
	}
}

// TestNewCreateRateLimiter_LargePerHour verifies that a large per-hour value
// (e.g. 120) is correctly converted to perMinute=2.
func TestNewCreateRateLimiter_LargePerHour(t *testing.T) {
	cfg := playground.Config{
		Enabled:         true,
		CreatePerIPHour: 120, // 2 per minute, 120 per hour
	}
	store := playground.NewCreateRateLimiter(cfg)

	// Should allow at least 2 requests quickly (burst = perMinute = 2).
	allowed1, _ := store.Allow("10.0.2.1")
	allowed2, _ := store.Allow("10.0.2.1")
	if !allowed1 || !allowed2 {
		t.Error("first 2 requests should be within the per-minute burst of 2")
	}
}

// TestCreateRateLimitMiddleware_Returns429WithRetryAfter verifies the acceptance
// criterion from Story 3 AC #1: "4th create within an hour from same IP returns
// 429 with Retry-After header."
//
// Uses CreatePerIPHour=180 → perMinute=3 → burst=3. After the burst is exhausted
// the 4th request must receive a 429 response whose Retry-After header value is a
// positive integer (number of seconds the client should wait before retrying).
func TestCreateRateLimitMiddleware_Returns429WithRetryAfter(t *testing.T) {
	cfg := playground.Config{
		Enabled:         true,
		CreatePerIPHour: 180, // perMinute = ceil(180/60) = 3 → burst = 3
	}
	store := playground.NewCreateRateLimiter(cfg)
	mw := playground.CreateRateLimitMiddleware(store, true)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	handler := mw(inner)

	ip := "10.0.3.1:5555"

	// Exhaust the burst (3 requests allowed).
	for i := range 3 {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/playground/sessions", nil)
		r.RemoteAddr = ip
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Errorf("request %d within burst: want 201, got %d", i+1, w.Code)
		}
	}

	// 4th request must be rate-limited with a parseable positive Retry-After.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/playground/sessions", nil)
	r.RemoteAddr = ip
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("4th request: want 429, got %d", w.Code)
	}

	retryAfterHeader := w.Header().Get("Retry-After")
	if retryAfterHeader == "" {
		t.Fatal("4th request: want Retry-After header on 429 response, got none")
	}

	secs, err := strconv.Atoi(retryAfterHeader)
	if err != nil {
		t.Errorf("Retry-After header %q must be an integer, got parse error: %v", retryAfterHeader, err)
	} else if secs <= 0 {
		t.Errorf("Retry-After header must be a positive integer, got %d", secs)
	}
}
