package wsgateway_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/wsgateway"
)

// ---------------------------------------------------------------------------
// fakeClock — a simple in-test clock that satisfies wsgateway.Clock
// ---------------------------------------------------------------------------

// fakeClock is a simple test-local clock that starts at a fixed time
// and can be advanced by the test. It satisfies the wsgateway.Clock interface
// via structural typing.
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

// ---------------------------------------------------------------------------
// TicketStore unit tests
// ---------------------------------------------------------------------------

// makeAccount returns a minimal *store.Account for test use.
func makeAccount(id string) *store.Account {
	return &store.Account{
		ID:          id,
		Email:       id + "@example.com",
		DisplayName: "Test " + id,
		CreatedAt:   time.Now().UTC(),
	}
}

// TestTicketStore_IssueAndConsume verifies the basic happy path: issue a
// ticket and immediately consume it — should return the account.
func TestTicketStore_IssueAndConsume(t *testing.T) {
	ts := wsgateway.NewTicketStore()
	ts.Start()
	t.Cleanup(ts.Stop)

	acct := makeAccount("acc-1")
	tok, ttl, err := ts.Issue(acct)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if tok == "" {
		t.Error("Issue returned empty token")
	}
	if ttl != wsgateway.TicketTTL {
		t.Errorf("ttl: want %v, got %v", wsgateway.TicketTTL, ttl)
	}

	got := ts.Consume(tok)
	if got == nil {
		t.Fatal("Consume returned nil for valid ticket")
	}
	if got.ID != acct.ID {
		t.Errorf("account ID: want %s, got %s", acct.ID, got.ID)
	}
}

// TestTicketStore_DoubleConsume verifies that consuming the same ticket twice
// only succeeds once — the second call returns nil.
func TestTicketStore_DoubleConsume(t *testing.T) {
	ts := wsgateway.NewTicketStore()
	ts.Start()
	t.Cleanup(ts.Stop)

	acct := makeAccount("acc-2")
	tok, _, err := ts.Issue(acct)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	first := ts.Consume(tok)
	if first == nil {
		t.Fatal("first Consume returned nil")
	}

	second := ts.Consume(tok)
	if second != nil {
		t.Error("second Consume: want nil (ticket already consumed), got non-nil")
	}
}

// TestTicketStore_ExpiredTicket verifies that a ticket consumed after its TTL
// is rejected even if the entry is still present in the map.
func TestTicketStore_ExpiredTicket(t *testing.T) {
	clk := newFakeClock(time.Now().UTC())
	ts := wsgateway.NewTicketStoreWithClock(clk)
	ts.Start()
	t.Cleanup(ts.Stop)

	acct := makeAccount("acc-3")
	tok, _, err := ts.Issue(acct)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Advance the clock past the TTL.
	clk.Advance(wsgateway.TicketTTL + time.Second)

	got := ts.Consume(tok)
	if got != nil {
		t.Error("Consume after TTL: want nil (expired), got non-nil")
	}
}

// TestTicketStore_UnknownToken verifies that consuming a token that was never
// issued returns nil.
func TestTicketStore_UnknownToken(t *testing.T) {
	ts := wsgateway.NewTicketStore()
	ts.Start()
	t.Cleanup(ts.Stop)

	got := ts.Consume("totally-made-up-token")
	if got != nil {
		t.Error("Consume unknown token: want nil, got non-nil")
	}
}

// TestTicketStore_IssueTokensAreUnique verifies that each call to Issue
// returns a different token (no reuse / collision in practice).
func TestTicketStore_IssueTokensAreUnique(t *testing.T) {
	ts := wsgateway.NewTicketStore()
	ts.Start()
	t.Cleanup(ts.Stop)

	acct := makeAccount("acc-unique")
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		tok, _, err := ts.Issue(acct)
		if err != nil {
			t.Fatalf("Issue %d: %v", i, err)
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("duplicate token on iteration %d: %s", i, tok)
		}
		seen[tok] = struct{}{}
	}
}

// TestTicketStore_ConcurrentConsume verifies that when many goroutines race to
// consume the same ticket, exactly one succeeds and all others get nil.
//
// Note on atomicity: sync.Map.LoadAndDelete is documented as atomic in Go's
// standard library — "LoadAndDelete deletes the value for a key, returning the
// previous value if any". This test confirms that property empirically across
// goroutines.
func TestTicketStore_ConcurrentConsume(t *testing.T) {
	ts := wsgateway.NewTicketStore()
	ts.Start()
	t.Cleanup(ts.Stop)

	acct := makeAccount("acc-concurrent")
	tok, _, err := ts.Issue(acct)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	const n = 20
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners []*store.Account
	)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // wait for the gun
			got := ts.Consume(tok)
			if got != nil {
				mu.Lock()
				winners = append(winners, got)
				mu.Unlock()
			}
		}()
	}

	close(start) // start all goroutines simultaneously
	wg.Wait()

	if len(winners) != 1 {
		t.Errorf("concurrent Consume: want exactly 1 winner, got %d", len(winners))
	}
}

// TestTicketStore_MultipleAccounts verifies that independent tickets for
// different accounts are correctly isolated — consuming one ticket does not
// affect another.
func TestTicketStore_MultipleAccounts(t *testing.T) {
	ts := wsgateway.NewTicketStore()
	ts.Start()
	t.Cleanup(ts.Stop)

	const n = 5
	toks := make([]string, n)
	accts := make([]*store.Account, n)
	for i := 0; i < n; i++ {
		accts[i] = makeAccount(fmt.Sprintf("acc-multi-%d", i))
		tok, _, err := ts.Issue(accts[i])
		if err != nil {
			t.Fatalf("Issue %d: %v", i, err)
		}
		toks[i] = tok
	}

	for i := 0; i < n; i++ {
		got := ts.Consume(toks[i])
		if got == nil {
			t.Fatalf("Consume[%d]: got nil", i)
		}
		if got.ID != accts[i].ID {
			t.Errorf("Consume[%d]: want account %s, got %s", i, accts[i].ID, got.ID)
		}
	}
}
