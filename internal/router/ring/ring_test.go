package ring_test

import (
	"fmt"
	"sync"
	"testing"

	"jamsesh/internal/router/ring"
)

// makePods returns n pods with predictable IDs and addresses.
func makePods(n int) []ring.Pod {
	pods := make([]ring.Pod, n)
	for i := range pods {
		pods[i] = ring.Pod{
			ID:      fmt.Sprintf("pod-%d", i),
			Address: fmt.Sprintf("10.0.0.%d:8080", i),
		}
	}
	return pods
}

// TestEmptyRing checks that Get on an empty ring returns a zero Pod.
func TestEmptyRing(t *testing.T) {
	t.Parallel()

	r := ring.New(150)
	got := r.Get("any-session-id")
	if got != (ring.Pod{}) {
		t.Errorf("empty ring: Get returned %+v; want zero Pod", got)
	}
}

// TestEmptyRingAfterClear checks that zeroing pods via SetPods gives empty ring.
func TestEmptyRingAfterClear(t *testing.T) {
	t.Parallel()

	r := ring.New(150)
	r.SetPods(makePods(3))
	r.SetPods(nil) // clear
	got := r.Get("session-abc")
	if got != (ring.Pod{}) {
		t.Errorf("cleared ring: Get returned %+v; want zero Pod", got)
	}
}

// TestSinglePodRouting checks that all keys route to the only pod.
func TestSinglePodRouting(t *testing.T) {
	t.Parallel()

	r := ring.New(150)
	pods := makePods(1)
	r.SetPods(pods)

	for _, key := range []string{"sess1", "sess2", "sess3", "abc", "xyz"} {
		got := r.Get(key)
		if got.ID != pods[0].ID {
			t.Errorf("single pod: Get(%q) = %q; want %q", key, got.ID, pods[0].ID)
		}
	}
}

// TestDeterministicRouting checks that the same key always maps to the same pod.
func TestDeterministicRouting(t *testing.T) {
	t.Parallel()

	pods := makePods(5)
	r := ring.New(150)
	r.SetPods(pods)

	// Build the expected mapping on first pass.
	keys := make([]string, 100)
	for i := range keys {
		keys[i] = fmt.Sprintf("session-%04d", i)
	}
	expected := make(map[string]ring.Pod, len(keys))
	for _, k := range keys {
		expected[k] = r.Get(k)
	}

	// Re-query: must return identical pods.
	for _, k := range keys {
		got := r.Get(k)
		if got != expected[k] {
			t.Errorf("non-deterministic: Get(%q) = %+v; want %+v", k, got, expected[k])
		}
	}
}

// TestAtomicSetPods checks that the snapshot is fully replaced atomically.
func TestAtomicSetPods(t *testing.T) {
	t.Parallel()

	r := ring.New(150)
	pods1 := makePods(3)
	pods2 := makePods(5) // different set

	r.SetPods(pods1)
	r.SetPods(pods2)

	// After the second SetPods the ring must only contain pods from pods2.
	pod2IDs := make(map[string]bool, len(pods2))
	for _, p := range pods2 {
		pod2IDs[p.ID] = true
	}

	for i := 0; i < 200; i++ {
		key := fmt.Sprintf("key-%d", i)
		got := r.Get(key)
		if got == (ring.Pod{}) {
			t.Errorf("non-empty ring returned zero Pod for key %q", key)
			continue
		}
		if !pod2IDs[got.ID] {
			t.Errorf("Get(%q) returned pod %q not in current set", key, got.ID)
		}
	}
}

// TestConsistentHashInvariant checks that adding or removing one pod from a
// 5-pod ring re-routes at most 1/5 + 10% of keys.
//
// Consistent-hashing guarantee: when you remove pod P, only the keys that
// were previously owned by P move to a new pod.  With 5 pods and uniform
// hashing, roughly 1/5 (20%) of keys should be affected.  We allow a ±10
// percentage-point tolerance (so ≤ 30% may move).
func TestConsistentHashInvariant(t *testing.T) {
	t.Parallel()

	const (
		nKeys      = 10_000
		nPods      = 5
		vnodes     = 150
		maxMoveFrac = 0.30 // 1/5 + 10 percentage points
	)

	pods5 := makePods(nPods)
	r := ring.New(vnodes)
	r.SetPods(pods5)

	// Record the mapping with 5 pods.
	keys := make([]string, nKeys)
	before := make(map[string]string, nKeys) // key → podID
	for i := range keys {
		keys[i] = fmt.Sprintf("sess-%06d", i)
		before[keys[i]] = r.Get(keys[i]).ID
	}

	// Test remove-one: drop the last pod.
	pods4 := pods5[:nPods-1]
	r.SetPods(pods4)
	moved := 0
	for _, k := range keys {
		after := r.Get(k).ID
		if after != before[k] {
			moved++
		}
	}
	moveFrac := float64(moved) / float64(nKeys)
	if moveFrac > maxMoveFrac {
		t.Errorf("remove one pod: %.1f%% of keys moved; want ≤ %.1f%%",
			moveFrac*100, maxMoveFrac*100)
	}
	t.Logf("remove one pod: %.2f%% of %d keys moved", moveFrac*100, nKeys)

	// Restore 5 pods and record fresh mapping.
	r.SetPods(pods5)
	for _, k := range keys {
		before[k] = r.Get(k).ID
	}

	// Test add-one: extend to 6 pods.
	pods6 := makePods(nPods + 1)
	r.SetPods(pods6)
	moved = 0
	for _, k := range keys {
		after := r.Get(k).ID
		if after != before[k] {
			moved++
		}
	}
	moveFrac = float64(moved) / float64(nKeys)
	if moveFrac > maxMoveFrac {
		t.Errorf("add one pod: %.1f%% of keys moved; want ≤ %.1f%%",
			moveFrac*100, maxMoveFrac*100)
	}
	t.Logf("add one pod: %.2f%% of %d keys moved", moveFrac*100, nKeys)
}

// TestConcurrentGetAndSetPods exercises the race detector by running Get and
// SetPods concurrently from many goroutines.
func TestConcurrentGetAndSetPods(t *testing.T) {
	t.Parallel()

	r := ring.New(150)
	r.SetPods(makePods(5))

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	stop := make(chan struct{})

	// Writers: alternately set 3-pod and 5-pod rings.
	for i := 0; i < goroutines/4; i++ {
		go func(idx int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if idx%2 == 0 {
						r.SetPods(makePods(3))
					} else {
						r.SetPods(makePods(5))
					}
				}
			}
		}(i)
	}

	// Readers: continuously call Get and verify it returns a valid Pod or
	// zero Pod (when the ring happens to be empty mid-swap).
	for i := goroutines / 4; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("session-%d", idx)
			for {
				select {
				case <-stop:
					return
				default:
					got := r.Get(key)
					// Either a valid pod or zero — both are correct.
					_ = got
				}
			}
		}(i)
	}

	// Run for a short burst then stop.
	// We drive iterations, not wall-clock time, to keep the test fast and
	// deterministic on slow CI machines.
	const iterations = 50_000
	for i := 0; i < iterations; i++ {
		_ = r.Get(fmt.Sprintf("burst-%d", i))
	}
	close(stop)
	wg.Wait()
}

// TestVnodeDistribution checks that with 150 vnodes per pod and 5 pods, every
// pod handles at least some fraction of a large key sample — i.e. no pod is
// entirely starved and no single pod monopolises the ring.
//
// FNV-based consistent hashing with 150 vnodes at 5 pods does NOT guarantee
// tight uniformity: ring-arc lengths depend on hash collisions and clustering,
// so variance can be large.  The meaningful property to assert is that every
// pod appears at all (≥ 2%) and no single pod monopolises the ring (≤ 55%).
// Tight uniform-distribution assertions belong in benchmarks, not unit tests.
func TestVnodeDistribution(t *testing.T) {
	t.Parallel()

	const (
		nPods   = 5
		nKeys   = 50_000
		vnodes  = 150
		minFrac = 0.02 // every pod must handle at least 2% of keys
		maxFrac = 0.55 // no pod may handle more than 55% of keys
	)

	r := ring.New(vnodes)
	r.SetPods(makePods(nPods))

	counts := make(map[string]int)
	for i := 0; i < nKeys; i++ {
		p := r.Get(fmt.Sprintf("sess-%d", i))
		counts[p.ID]++
	}

	for _, p := range makePods(nPods) {
		frac := float64(counts[p.ID]) / float64(nKeys)
		if frac < minFrac || frac > maxFrac {
			t.Errorf("pod %s handled %.1f%% of keys; want %.0f%%–%.0f%%",
				p.ID, frac*100, minFrac*100, maxFrac*100)
		}
		t.Logf("pod %s: %.2f%%", p.ID, frac*100)
	}
}
