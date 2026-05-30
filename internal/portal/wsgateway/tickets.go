package wsgateway

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"jamsesh/internal/db/store"
)

const (
	// TicketTTL is the lifetime of a WS upgrade ticket. It is intentionally
	// short — the SPA fetches a ticket immediately before opening the socket,
	// so 60 s is more than enough for the RTT between POST and the WS upgrade.
	TicketTTL = 60 * time.Second

	// ticketJanitorInterval controls how often the janitor goroutine sweeps
	// expired entries. Expired tickets are already rejected on consume; the
	// janitor is purely a memory hygiene measure.
	ticketJanitorInterval = 30 * time.Second

	// ticketBytes is the number of random bytes in a ticket. 32 bytes → 256
	// bits of entropy, more than sufficient for a single-use credential.
	ticketBytes = 32
)

// ticket is the value stored in the TicketStore.
type ticket struct {
	account   *store.Account
	expiresAt time.Time
}

// TicketStore is an in-memory single-use ticket store for WebSocket upgrade
// authentication. It is safe for concurrent use.
//
// Lifecycle: call Start() after construction; call Stop() (or cancel the
// parent context) during shutdown.
//
// Design notes:
//   - Tickets are 32-byte random values encoded as base64url (URL-safe,
//     no padding). This is intentionally opaque — the gateway does not
//     need to inspect the ticket's content.
//   - Consume is atomic: it deletes the entry and returns its value in
//     one step (via sync.Map.LoadAndDelete). Two concurrent upgrade
//     requests presenting the same ticket can only succeed once because
//     LoadAndDelete is guaranteed by the sync.Map contract.
//   - The trade-off for the 60-second TTL: if a bearer token is revoked
//     (via POST /api/auth/revoke) after a ticket is issued but before it
//     is consumed, the ticket is still valid because it carries the already-
//     resolved *store.Account. Acceptable given the 60-second window.
type TicketStore struct {
	mu       sync.Mutex // guards the stopCh and started flag
	entries  sync.Map   // map[string]*ticket
	stopCh   chan struct{}
	stopOnce sync.Once // guards close(stopCh) so a second Stop() is a no-op
	started  bool
	clock    Clock
}

// NewTicketStore creates a TicketStore that uses the real wall clock.
func NewTicketStore() *TicketStore {
	return &TicketStore{
		stopCh: make(chan struct{}),
		clock:  realClock{},
	}
}

// NewTicketStoreWithClock creates a TicketStore that uses the provided clock.
// Intended for tests that need to advance time to simulate ticket expiry.
func NewTicketStoreWithClock(clk Clock) *TicketStore {
	return &TicketStore{
		stopCh: make(chan struct{}),
		clock:  clk,
	}
}

// Start launches the background janitor goroutine. It is idempotent; a second
// call is a no-op. Safe to call from main before serving traffic.
func (ts *TicketStore) Start() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.started {
		return
	}
	ts.started = true
	go ts.janitor()
}

// Stop halts the janitor goroutine. After Stop returns, no further sweeps
// will occur. Existing entries are not cleared (they expire and are rejected
// on the next consume attempt). Stop is idempotent — a second call is a no-op.
func (ts *TicketStore) Stop() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if !ts.started {
		return
	}
	ts.stopOnce.Do(func() { close(ts.stopCh) })
}

// Issue generates a fresh ticket for the given account and stores it with a
// 60-second TTL. Returns the opaque base64url ticket string and the TTL.
func (ts *TicketStore) Issue(acct *store.Account) (string, time.Duration, error) {
	raw := make([]byte, ticketBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", 0, err
	}
	tok := base64.RawURLEncoding.EncodeToString(raw)

	ts.entries.Store(tok, &ticket{
		account:   acct,
		expiresAt: ts.clock.Now().Add(TicketTTL),
	})
	return tok, TicketTTL, nil
}

// Consume atomically removes the ticket and returns the associated account.
// Returns nil if the ticket is unknown or has expired. Because the entry is
// removed on the first call, a second call with the same token always returns
// nil — this enforces single-use.
func (ts *TicketStore) Consume(tok string) *store.Account {
	v, loaded := ts.entries.LoadAndDelete(tok)
	if !loaded {
		return nil
	}
	t := v.(*ticket)
	if ts.clock.Now().After(t.expiresAt) {
		return nil
	}
	return t.account
}

// janitor periodically removes expired entries to keep memory bounded.
func (ts *TicketStore) janitor() {
	ticker := time.NewTicker(ticketJanitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ts.stopCh:
			return
		case <-ticker.C:
			now := ts.clock.Now()
			ts.entries.Range(func(k, v any) bool {
				if v.(*ticket).expiresAt.Before(now) {
					ts.entries.Delete(k)
				}
				return true
			})
		}
	}
}
