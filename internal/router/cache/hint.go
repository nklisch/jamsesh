// Package cache provides the soft-coordinator hint cache for the jamsesh router.
// It maps session_id → pod ID with LRU eviction and per-entry TTL.
package cache

import (
	"container/list"
	"sync"
	"time"
)

// entry is the value stored in each list element.
type entry struct {
	sessionID string
	podID     string
	expiry    time.Time
}

// Hint is an LRU-bounded in-memory cache mapping session_id → pod ID.
// Entries expire after TTL or on explicit Invalidate.
// Safe for concurrent use.
type Hint struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	ll         *list.List               // front = most-recently used
	items      map[string]*list.Element // sessionID → *list.Element
}

// New returns a Hint with the given max entries and per-entry TTL.
// maxEntries must be > 0; TTL must be > 0.
func New(maxEntries int, ttl time.Duration) *Hint {
	if maxEntries <= 0 {
		panic("cache: maxEntries must be > 0")
	}
	if ttl <= 0 {
		panic("cache: ttl must be > 0")
	}
	return &Hint{
		maxEntries: maxEntries,
		ttl:        ttl,
		ll:         list.New(),
		items:      make(map[string]*list.Element, maxEntries),
	}
}

// Get returns the cached pod ID and true if the entry is present and unexpired.
// An expired entry is removed and a miss is returned.
func (h *Hint) Get(sessionID string) (podID string, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	el, exists := h.items[sessionID]
	if !exists {
		return "", false
	}

	e := el.Value.(*entry)
	if time.Now().After(e.expiry) {
		// Expired — evict and report miss.
		h.removeElement(el)
		return "", false
	}

	// Cache hit — promote to front (most-recently used).
	h.ll.MoveToFront(el)
	return e.podID, true
}

// Set records or refreshes the sessionID → podID mapping.
// If the session is already cached, its pod ID and TTL are updated and the
// entry is promoted to the front. If the cache is at capacity, the least-
// recently used entry is evicted before inserting the new one.
func (h *Hint) Set(sessionID, podID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	expiry := time.Now().Add(h.ttl)

	if el, exists := h.items[sessionID]; exists {
		// Refresh existing entry.
		e := el.Value.(*entry)
		e.podID = podID
		e.expiry = expiry
		h.ll.MoveToFront(el)
		return
	}

	// Evict LRU if at capacity.
	if h.ll.Len() >= h.maxEntries {
		h.removeElement(h.ll.Back())
	}

	// Insert new entry at front.
	e := &entry{
		sessionID: sessionID,
		podID:     podID,
		expiry:    expiry,
	}
	el := h.ll.PushFront(e)
	h.items[sessionID] = el
}

// Invalidate removes the entry for sessionID. No-op if absent.
func (h *Hint) Invalidate(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if el, exists := h.items[sessionID]; exists {
		h.removeElement(el)
	}
}

// removeElement removes el from the list and the items map.
// Must be called with h.mu held.
func (h *Hint) removeElement(el *list.Element) {
	h.ll.Remove(el)
	delete(h.items, el.Value.(*entry).sessionID)
}
