package ratelimit_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"jamsesh/internal/portal/ratelimit"
)

// fakeClock is a minimal injectable clock for unit tests. Advance moves it
// forward without sleeping.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{now: t} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// TestStore_Allow_UnderLimit verifies that requests well within the burst are
// all allowed.
func TestStore_Allow_UnderLimit(t *testing.T) {
	s := ratelimit.NewStore(ratelimit.Config{PerMinute: 10})
	for i := range 5 {
		allowed, _ := s.Allow("192.0.2.1")
		if !allowed {
			t.Errorf("request %d should be allowed (under burst of 10)", i+1)
		}
	}
}

// TestStore_Allow_ExceedsBurst verifies that requests exceeding the burst are
// rejected.
func TestStore_Allow_ExceedsBurst(t *testing.T) {
	s := ratelimit.NewStore(ratelimit.Config{PerMinute: 3})
	// Consume the burst.
	for range 3 {
		s.Allow("192.0.2.2")
	}
	// Next request should be rate-limited.
	allowed, retryAfter := s.Allow("192.0.2.2")
	if allowed {
		t.Error("request after burst should be rate-limited")
	}
	if retryAfter <= 0 {
		t.Error("retryAfter should be positive when rate-limited")
	}
}

// TestStore_Allow_DifferentKeys verifies that different keys have independent limiters.
func TestStore_Allow_DifferentKeys(t *testing.T) {
	s := ratelimit.NewStore(ratelimit.Config{PerMinute: 1})
	// Exhaust key A.
	s.Allow("192.0.2.3")

	// Key B should still be allowed.
	allowed, _ := s.Allow("192.0.2.4")
	if !allowed {
		t.Error("key B should not be affected by key A's limit")
	}
}

// TestStore_Size verifies that the size counter increases as new keys are seen.
func TestStore_Size(t *testing.T) {
	s := ratelimit.NewStore(ratelimit.Config{PerMinute: 10})
	if s.Size() != 0 {
		t.Errorf("want initial size 0, got %d", s.Size())
	}
	s.Allow("a")
	s.Allow("b")
	s.Allow("b") // duplicate
	if s.Size() != 2 {
		t.Errorf("want size 2, got %d", s.Size())
	}
}

// TestStore_HourlyLimit verifies that the hourly limiter also gates requests
// when set to a tighter burst than the per-minute limiter.
func TestStore_HourlyLimit(t *testing.T) {
	// 60/min but only 2/hour burst — so after 2 requests the hourly limit fires.
	s := ratelimit.NewStore(ratelimit.Config{PerMinute: 60, PerHour: 2})
	s.Allow("192.0.2.5")
	s.Allow("192.0.2.5")
	allowed, _ := s.Allow("192.0.2.5")
	if allowed {
		t.Error("third request should be blocked by hourly limiter")
	}
}

// TestMiddleware_Disabled verifies that a disabled middleware passes all requests through.
func TestMiddleware_Disabled(t *testing.T) {
	s := ratelimit.NewStore(ratelimit.Config{PerMinute: 1})
	mw := s.Middleware(false)

	// Even after exhausting the burst (which we don't, because disabled),
	// every request should pass.
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for range 5 {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = "192.0.2.6:1234"
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("disabled middleware: want 200, got %d", w.Code)
		}
	}
}

// TestMiddleware_Enabled_Allows verifies that requests within the burst pass through.
func TestMiddleware_Enabled_Allows(t *testing.T) {
	s := ratelimit.NewStore(ratelimit.Config{PerMinute: 10})
	mw := s.Middleware(true)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = "192.0.2.7:5678"
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

// TestStore_GC_RemovesStaleEntries verifies that stale entries are removed by GC
// after the GC interval elapses, using a fake clock — no wall-clock sleep needed.
func TestStore_GC_RemovesStaleEntries(t *testing.T) {
	// Start at a fixed epoch so sub-second drift never affects the test.
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := newFakeClock(epoch)

	s := ratelimit.NewStoreWithClock(ratelimit.Config{PerMinute: 10}, clk)

	// Register two entries.
	s.Allow("192.0.2.10")
	s.Allow("192.0.2.11")
	if s.Size() != 2 {
		t.Fatalf("want 2 entries before GC, got %d", s.Size())
	}

	// Advance past the TTL (1 h) so entries become stale, but stay inside
	// gcInterval (5 min) so GC has not fired yet.
	// Actually we need to advance past gcInterval to trigger GC on the next
	// Allow call. Let's advance past both TTL and gcInterval.
	clk.Advance(2 * time.Hour) // past TTL (1 h) and gcInterval (5 min)

	// Trigger GC by calling Allow on a new key (forces getOrCreate).
	s.Allow("192.0.2.99")

	// The two original stale entries should have been swept; only the new one remains.
	if s.Size() != 1 {
		t.Errorf("want 1 entry after GC (only new key), got %d", s.Size())
	}
}

// TestStore_BucketRefill_FakeClock verifies that token-bucket refill is driven
// by the injected clock, not wall time. After exhausting the burst, advancing
// the clock by one minute restores capacity.
func TestStore_BucketRefill_FakeClock(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := newFakeClock(epoch)

	// 3/min burst — exhaust it.
	s := ratelimit.NewStoreWithClock(ratelimit.Config{PerMinute: 3}, clk)
	for range 3 {
		s.Allow("10.0.0.1")
	}
	allowed, _ := s.Allow("10.0.0.1")
	if allowed {
		t.Fatal("want rate-limited after burst exhausted")
	}

	// Advance the fake clock by 1 minute; the token bucket should have refilled.
	clk.Advance(1 * time.Minute)

	allowed, _ = s.Allow("10.0.0.1")
	if !allowed {
		t.Error("want request allowed after clock advances 1 minute (bucket refill)")
	}
}

// TestMiddleware_Enabled_Blocks verifies that requests over the burst get 429.
func TestMiddleware_Enabled_Blocks(t *testing.T) {
	s := ratelimit.NewStore(ratelimit.Config{PerMinute: 2})
	mw := s.Middleware(true)

	callCount := 0
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))

	ip := "192.0.2.8:9999"
	// First two pass (burst = 2).
	for range 2 {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = ip
		handler.ServeHTTP(w, r)
	}

	// Third is rate-limited.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = ip
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("want 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("want Retry-After header on 429 response")
	}
	if callCount != 2 {
		t.Errorf("want inner handler called 2 times, got %d", callCount)
	}
}
