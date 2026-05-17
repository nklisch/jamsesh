// Package ring implements a consistent-hash ring with virtual-node replication
// for even load distribution across a dynamic set of backend pods.
//
// # Concurrency model
//
// A Ring holds an atomic.Pointer[ringSnapshot]. Reads (Get) are lock-free:
// they load the current snapshot pointer and operate on its immutable sorted
// slice. Writes (SetPods) build a brand-new snapshot and atomically swap the
// pointer, so concurrent readers always see a fully-consistent view.
//
// # Hashing
//
// Both vnode allocation and key lookup use hash/fnv.New64a. The vnode hash is
// deterministic: hash("{podID}:{vnodeIndex}"). This means a ring built from
// the same pod set always produces the same assignment, which makes tests
// assertable and lets the hint cache tolerate router restarts.
package ring

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync/atomic"
)

// Pod is a routable backend.
type Pod struct {
	ID      string // stable identifier (e.g. k8s pod name)
	Address string // host:port the proxy targets
}

// vnode is a single virtual node on the ring.
type vnode struct {
	hash  uint64
	podID string
}

// ringSnapshot is an immutable consistent-hash snapshot. Once constructed
// it is never mutated; concurrent readers hold a pointer to it.
type ringSnapshot struct {
	vnodes []vnode        // sorted ascending by hash
	pods   map[string]Pod // podID → Pod
}

// Ring is a consistent-hash ring. It is safe for concurrent use.
// Construct with New; do not copy after first use.
type Ring struct {
	// vnodeCount is the number of virtual nodes per real pod. Immutable after
	// construction; read by SetPods with no synchronisation needed.
	vnodeCount int

	// ptr holds the current ring snapshot. Lock-free reads via atomic.Pointer.
	ptr atomic.Pointer[ringSnapshot]
}

// New returns a Ring configured to use vnodes virtual nodes per real pod.
// Callers should pick a value in the range 100–200; the parent design
// recommends 150. Smaller values give less-even distribution; larger values
// use more memory but don't improve consistency materially at session scale.
func New(vnodes int) *Ring {
	r := &Ring{vnodeCount: vnodes}
	// Store an empty snapshot so ptr is never nil; simplifies Get.
	r.ptr.Store(&ringSnapshot{})
	return r
}

// SetPods replaces the ring contents with the provided pod set atomically.
// An empty or nil slice results in an empty ring (subsequent Get calls return
// zero Pod). SetPods is safe to call concurrently from multiple goroutines,
// though in practice it is driven by a single discovery goroutine.
func (r *Ring) SetPods(pods []Pod) {
	if len(pods) == 0 {
		r.ptr.Store(&ringSnapshot{})
		return
	}

	podMap := make(map[string]Pod, len(pods))
	for _, p := range pods {
		podMap[p.ID] = p
	}

	vs := make([]vnode, 0, len(pods)*r.vnodeCount)
	for _, p := range pods {
		for i := 0; i < r.vnodeCount; i++ {
			h := hashVnode(p.ID, i)
			vs = append(vs, vnode{hash: h, podID: p.ID})
		}
	}

	sort.Slice(vs, func(i, j int) bool { return vs[i].hash < vs[j].hash })

	r.ptr.Store(&ringSnapshot{
		vnodes: vs,
		pods:   podMap,
	})
}

// Get returns the pod responsible for key, or zero Pod when the ring is
// empty. Get is lock-free: it loads the current snapshot atomically and does
// a binary search on its immutable vnode slice.
func (r *Ring) Get(key string) Pod {
	snap := r.ptr.Load()
	if snap == nil || len(snap.vnodes) == 0 {
		return Pod{}
	}

	h := hashKey(key)
	vs := snap.vnodes

	// Binary search for the first vnode with hash ≥ h (ring wrap-around).
	idx := sort.Search(len(vs), func(i int) bool { return vs[i].hash >= h })
	if idx == len(vs) {
		idx = 0
	}

	podID := vs[idx].podID
	return snap.pods[podID]
}

// Pods returns a snapshot of all real pods currently in the ring. The slice is
// a copy; modifications do not affect the ring. Returns nil when the ring is
// empty. Safe for concurrent use.
func (r *Ring) Pods() []Pod {
	snap := r.ptr.Load()
	if snap == nil || len(snap.pods) == 0 {
		return nil
	}
	out := make([]Pod, 0, len(snap.pods))
	for _, p := range snap.pods {
		out = append(out, p)
	}
	return out
}

// GetNext returns the pod responsible for key that is different from skipID,
// i.e. the "next" pod in ring order after the one that would normally own key.
// It is used by the router to find a retry target when the primary pod returns
// 503. Returns zero Pod when the ring has fewer than two pods or all remaining
// pods share the same ID as skipID.
func (r *Ring) GetNext(key, skipID string) Pod {
	snap := r.ptr.Load()
	if snap == nil || len(snap.vnodes) == 0 {
		return Pod{}
	}

	h := hashKey(key)
	vs := snap.vnodes

	// Find the starting index (same as Get).
	start := sort.Search(len(vs), func(i int) bool { return vs[i].hash >= h })
	if start == len(vs) {
		start = 0
	}

	// Walk the ring from start+1 until we find a pod with a different ID.
	for i := 1; i < len(vs); i++ {
		idx := (start + i) % len(vs)
		podID := vs[idx].podID
		if podID != skipID {
			return snap.pods[podID]
		}
	}
	return Pod{}
}

// ── Hash helpers ──────────────────────────────────────────────────────────────

// hashVnode produces a deterministic hash for the i-th virtual node of a pod.
func hashVnode(podID string, i int) uint64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s:%d", podID, i)
	return h.Sum64()
}

// hashKey produces the ring lookup hash for a request key (typically a
// session ID).
func hashKey(key string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return h.Sum64()
}
