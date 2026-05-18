package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestNewPanics verifies that New rejects invalid arguments.
func TestNewPanics(t *testing.T) {
	t.Run("zero maxEntries panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for maxEntries=0")
			}
		}()
		New(0, time.Second)
	})

	t.Run("zero TTL panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for ttl=0")
			}
		}()
		New(10, 0)
	})
}

// TestSetGet verifies that a Set followed by a Get within TTL returns a hit.
func TestSetGet(t *testing.T) {
	h := New(10, time.Minute)
	h.Set("sess-1", "pod-a")

	got, ok := h.Get("sess-1")
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}
	if got != "pod-a" {
		t.Fatalf("expected pod-a, got %q", got)
	}
}

// TestGetMissOnAbsent verifies that Get on an absent key returns a miss.
func TestGetMissOnAbsent(t *testing.T) {
	h := New(10, time.Minute)

	_, ok := h.Get("no-such-session")
	if ok {
		t.Fatal("expected miss for absent key")
	}
}

// TestTTLExpiry verifies that Get returns a miss after the TTL has elapsed.
func TestTTLExpiry(t *testing.T) {
	h := New(10, 10*time.Millisecond)
	h.Set("sess-exp", "pod-b")

	// Confirm it's present immediately.
	if _, ok := h.Get("sess-exp"); !ok {
		t.Fatal("expected hit immediately after Set")
	}

	time.Sleep(20 * time.Millisecond)

	_, ok := h.Get("sess-exp")
	if ok {
		t.Fatal("expected miss after TTL expiry, got hit")
	}
}

// TestLRUEviction verifies that filling the cache beyond maxEntries evicts
// the least-recently-used entry.
func TestLRUEviction(t *testing.T) {
	const max = 5
	h := New(max, time.Minute)

	// Insert max entries — keys "0" through "4".
	for i := 0; i < max; i++ {
		h.Set(fmt.Sprintf("%d", i), fmt.Sprintf("pod-%d", i))
	}

	// Touch key "0" so "1" becomes the LRU.
	if _, ok := h.Get("0"); !ok {
		t.Fatal("expected hit for key 0 before eviction test")
	}

	// Insert one more — should evict LRU ("1").
	h.Set("extra", "pod-extra")

	// "1" should be evicted.
	if _, ok := h.Get("1"); ok {
		t.Fatal("expected key '1' to be evicted (LRU)")
	}

	// "0" and "extra" should still be present.
	if _, ok := h.Get("0"); !ok {
		t.Fatal("expected key '0' to survive (was recently used)")
	}
	if _, ok := h.Get("extra"); !ok {
		t.Fatal("expected key 'extra' to be present")
	}
}

// TestLRUEvictionOrder verifies that without any Gets, insertions evict the
// oldest inserted entry when capacity is exceeded.
func TestLRUEvictionOrder(t *testing.T) {
	h := New(3, time.Minute)
	h.Set("a", "pod-a")
	h.Set("b", "pod-b")
	h.Set("c", "pod-c")
	// "a" is now LRU. Adding "d" should evict "a".
	h.Set("d", "pod-d")

	if _, ok := h.Get("a"); ok {
		t.Fatal("expected 'a' to be evicted")
	}
	for _, key := range []string{"b", "c", "d"} {
		if _, ok := h.Get(key); !ok {
			t.Fatalf("expected key %q to survive eviction", key)
		}
	}
}

// TestInvalidate verifies that Invalidate causes a subsequent Get to miss.
func TestInvalidate(t *testing.T) {
	h := New(10, time.Minute)
	h.Set("sess-inv", "pod-c")

	h.Invalidate("sess-inv")

	_, ok := h.Get("sess-inv")
	if ok {
		t.Fatal("expected miss after Invalidate")
	}
}

// TestInvalidateAbsent verifies that Invalidate on an absent key is a no-op.
func TestInvalidateAbsent(t *testing.T) {
	h := New(10, time.Minute)
	// Should not panic.
	h.Invalidate("not-there")
}

// TestSetRefreshesTTL verifies that calling Set again on an existing key
// refreshes its TTL.
func TestSetRefreshesTTL(t *testing.T) {
	h := New(10, 30*time.Millisecond)
	h.Set("sess-r", "pod-d")

	// Sleep until near expiry, then refresh.
	time.Sleep(20 * time.Millisecond)
	h.Set("sess-r", "pod-d") // refresh

	// Sleep past the original TTL; the refreshed entry should still be alive.
	time.Sleep(20 * time.Millisecond)

	if _, ok := h.Get("sess-r"); !ok {
		t.Fatal("expected hit after TTL refresh, got miss")
	}
}

// TestSetUpdatesValue verifies that calling Set on an existing key updates
// the stored pod ID.
func TestSetUpdatesValue(t *testing.T) {
	h := New(10, time.Minute)
	h.Set("sess-u", "pod-old")
	h.Set("sess-u", "pod-new")

	got, ok := h.Get("sess-u")
	if !ok {
		t.Fatal("expected hit")
	}
	if got != "pod-new" {
		t.Fatalf("expected pod-new, got %q", got)
	}
}

// TestConcurrentAccess exercises Get, Set, and Invalidate under concurrent
// goroutines to validate race-freedom. Run with -race.
func TestConcurrentAccess(t *testing.T) {
	h := New(100, 50*time.Millisecond)

	const (
		writers     = 10
		readers     = 10
		invalidators = 5
		iterations  = 200
	)

	var wg sync.WaitGroup
	sessions := make([]string, 50)
	for i := range sessions {
		sessions[i] = fmt.Sprintf("session-%d", i)
	}

	// Writers: continuously Set entries.
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := sessions[i%len(sessions)]
				h.Set(key, fmt.Sprintf("pod-%d", w))
			}
		}(w)
	}

	// Readers: continuously Get entries.
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := sessions[i%len(sessions)]
				h.Get(key) //nolint:errcheck — result not the point here
			}
		}(r)
	}

	// Invalidators: continuously Invalidate entries.
	for v := 0; v < invalidators; v++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := sessions[i%len(sessions)]
				h.Invalidate(key)
			}
		}(v)
	}

	wg.Wait()
}

// TestExpiredEntryRemovedFromMap verifies that after a TTL miss, the internal
// map does not retain the expired entry (prevents unbounded growth).
func TestExpiredEntryRemovedFromMap(t *testing.T) {
	h := New(10, 10*time.Millisecond)
	h.Set("sess-cleanup", "pod-x")

	time.Sleep(20 * time.Millisecond)

	// Trigger expiry cleanup via Get.
	h.Get("sess-cleanup")

	h.mu.Lock()
	_, inMap := h.items["sess-cleanup"]
	listLen := h.ll.Len()
	h.mu.Unlock()

	if inMap {
		t.Fatal("expired entry should have been removed from the map")
	}
	if listLen != 0 {
		t.Fatalf("expected empty list after expiry cleanup, got len=%d", listLen)
	}
}
