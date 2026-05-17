package events_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
)

// dbCounter is used to create unique in-memory SQLite database names.
var dbCounter atomic.Int64

// openStore opens a fresh in-memory SQLite store with all migrations applied.
// Uses a named shared-cache in-memory database (file:testN?mode=memory&cache=shared)
// so that concurrent goroutines sharing the same *sql.DB all see the same schema.
// MaxOpenConns is set to 1 to ensure a single connection owns the in-memory DB.
func openStore(t *testing.T) store.Store {
	t.Helper()
	n := dbCounter.Add(1)
	dsn := fmt.Sprintf("file:events_test_%d?mode=memory&cache=shared", n)
	s, err := db.Open(context.Background(), "sqlite", dsn, db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	// Restrict to one connection so all goroutines share the in-memory schema.
	// This is safe: SQLite serialises writes anyway; the single connection
	// avoids "no such table" errors that arise when a second connection opens a
	// fresh (empty) in-memory database view.
	if rawDB := sqliteRawDB(s); rawDB != nil {
		rawDB.SetMaxOpenConns(1)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// sqliteRawDB extracts the underlying *sql.DB from a store via a type assertion.
// Returns nil if the store is not a SQLite adapter exposing RawDB().
func sqliteRawDB(s store.Store) *sql.DB {
	type rawDBer interface {
		RawDB() *sql.DB
	}
	if r, ok := s.(rawDBer); ok {
		return r.RawDB()
	}
	return nil
}

// sessionFixture bundles the IDs needed to emit events.
type sessionFixture struct {
	orgID     string
	sessionID string
	accountID string
}

var fixtureCounter int
var fixtureCounterMu sync.Mutex

func nextFixtureID(prefix string) string {
	fixtureCounterMu.Lock()
	defer fixtureCounterMu.Unlock()
	fixtureCounter++
	return fmt.Sprintf("%s-%04d", prefix, fixtureCounter)
}

func mustSetupSession(t *testing.T, ctx context.Context, s store.Store) sessionFixture {
	t.Helper()
	orgID := nextFixtureID("org")
	accountID := nextFixtureID("acc")
	sessionID := nextFixtureID("sess")
	now := time.Now().UTC()

	_, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        orgID,
		Name:      "Org " + orgID,
		Slug:      orgID + "-slug",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	_, err = s.CreateAccount(ctx, store.CreateAccountParams{
		ID:          accountID,
		Email:       accountID + "@example.com",
		DisplayName: "Test User",
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	_, err = s.CreateSession(ctx, store.CreateSessionParams{
		ID:            sessionID,
		OrgID:         orgID,
		Name:          "Test Session",
		Goal:          "goal",
		WritableScope: `["src/"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return sessionFixture{
		orgID:     orgID,
		sessionID: sessionID,
		accountID: accountID,
	}
}

func mustMarshal(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestLog_EmitSingleMonotonic(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess := mustSetupSession(t, ctx, s)
	log := events.New(s)

	seq1, err := log.Emit(ctx, sess.orgID, sess.sessionID, "commit.arrived", mustMarshal(t, map[string]string{"sha": "abc"}))
	if err != nil {
		t.Fatalf("Emit 1: %v", err)
	}
	if seq1 != 1 {
		t.Errorf("first emit: want seq=1, got %d", seq1)
	}

	seq2, err := log.Emit(ctx, sess.orgID, sess.sessionID, "commit.arrived", mustMarshal(t, map[string]string{"sha": "def"}))
	if err != nil {
		t.Fatalf("Emit 2: %v", err)
	}
	if seq2 != 2 {
		t.Errorf("second emit: want seq=2, got %d", seq2)
	}
}

func TestLog_EmitBatch(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess := mustSetupSession(t, ctx, s)
	log := events.New(s)

	drafts := []events.DraftEvent{
		{Type: "commit.arrived", Payload: mustMarshal(t, map[string]string{"sha": "aaa"})},
		{Type: "commit.arrived", Payload: mustMarshal(t, map[string]string{"sha": "bbb"})},
		{Type: "commit.arrived", Payload: mustMarshal(t, map[string]string{"sha": "ccc"})},
	}
	firstSeq, err := log.EmitBatch(ctx, sess.orgID, sess.sessionID, drafts)
	if err != nil {
		t.Fatalf("EmitBatch: %v", err)
	}
	if firstSeq != 1 {
		t.Errorf("EmitBatch: want firstSeq=1, got %d", firstSeq)
	}

	// Verify all 3 rows present with contiguous seqs.
	evs, err := log.ListSince(ctx, sess.sessionID, 0, 100)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("want 3 events, got %d", len(evs))
	}
	for i, e := range evs {
		want := int64(i + 1)
		if e.Seq != want {
			t.Errorf("event[%d]: want seq=%d, got %d", i, want, e.Seq)
		}
	}
}

func TestLog_EmitBatch_Empty(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess := mustSetupSession(t, ctx, s)
	log := events.New(s)

	firstSeq, err := log.EmitBatch(ctx, sess.orgID, sess.sessionID, nil)
	if err != nil {
		t.Fatalf("EmitBatch(nil): %v", err)
	}
	if firstSeq != 0 {
		t.Errorf("want firstSeq=0 for empty batch, got %d", firstSeq)
	}
}

func TestLog_ConcurrentEmit(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess := mustSetupSession(t, ctx, s)
	log := events.New(s)

	const n = 10
	results := make([]int64, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			seq, err := log.Emit(ctx, sess.orgID, sess.sessionID, "commit.arrived",
				mustMarshal(t, map[string]int{"goroutine": i}))
			results[i] = seq
			errs[i] = err
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	// All seqs must be unique and span exactly [1, 10].
	sort.Slice(results, func(i, j int) bool { return results[i] < results[j] })
	for i, seq := range results {
		want := int64(i + 1)
		if seq != want {
			t.Errorf("sorted seq[%d]: want %d, got %d (full slice: %v)", i, want, seq, results)
			break
		}
	}
}

func TestLog_DifferentSessionsIndependentSeqs(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess1 := mustSetupSession(t, ctx, s)
	sess2 := mustSetupSession(t, ctx, s)
	log := events.New(s)

	seq1, err := log.Emit(ctx, sess1.orgID, sess1.sessionID, "commit.arrived", mustMarshal(t, map[string]string{"sha": "aaa"}))
	if err != nil {
		t.Fatalf("sess1 emit: %v", err)
	}
	seq2, err := log.Emit(ctx, sess2.orgID, sess2.sessionID, "commit.arrived", mustMarshal(t, map[string]string{"sha": "bbb"}))
	if err != nil {
		t.Fatalf("sess2 emit: %v", err)
	}

	// Both independent sessions start at seq=1.
	if seq1 != 1 {
		t.Errorf("sess1: want seq=1, got %d", seq1)
	}
	if seq2 != 1 {
		t.Errorf("sess2: want seq=1, got %d", seq2)
	}
}

func TestLog_UpdatePresence(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess := mustSetupSession(t, ctx, s)
	log := events.New(s)

	err := log.UpdatePresence(ctx, sess.orgID, sess.sessionID, sess.accountID, "main", "sha123")
	if err != nil {
		t.Fatalf("UpdatePresence: %v", err)
	}

	// A presence.updated event must have been emitted.
	evs, err := log.ListSince(ctx, sess.sessionID, 0, 10)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("want 1 event after UpdatePresence, got %d", len(evs))
	}
	if evs[0].Type != "presence.updated" {
		t.Errorf("event type: want presence.updated, got %s", evs[0].Type)
	}
	if evs[0].Seq != 1 {
		t.Errorf("event seq: want 1, got %d", evs[0].Seq)
	}

	// Second UpdatePresence increments seq.
	err = log.UpdatePresence(ctx, sess.orgID, sess.sessionID, sess.accountID, "main", "sha456")
	if err != nil {
		t.Fatalf("UpdatePresence 2: %v", err)
	}
	evs2, err := log.ListSince(ctx, sess.sessionID, 0, 10)
	if err != nil {
		t.Fatalf("ListSince 2: %v", err)
	}
	if len(evs2) != 2 {
		t.Fatalf("want 2 events, got %d", len(evs2))
	}
	if evs2[1].Seq != 2 {
		t.Errorf("second presence.updated seq: want 2, got %d", evs2[1].Seq)
	}
}

func TestLog_ListSince_Cursor(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess := mustSetupSession(t, ctx, s)
	log := events.New(s)

	// Emit 5 events.
	for i := 0; i < 5; i++ {
		_, err := log.Emit(ctx, sess.orgID, sess.sessionID, "commit.arrived",
			mustMarshal(t, map[string]int{"n": i}))
		if err != nil {
			t.Fatalf("emit %d: %v", i, err)
		}
	}

	// Cursor from seq=2 (sinceSeq=2) should return [3,4,5].
	evs, err := log.ListSince(ctx, sess.sessionID, 2, 100)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("want 3 events after cursor, got %d", len(evs))
	}
	for i, e := range evs {
		want := int64(3 + i)
		if e.Seq != want {
			t.Errorf("evs[%d].Seq: want %d, got %d", i, want, e.Seq)
		}
	}
}

func TestLog_ListSince_Limit(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess := mustSetupSession(t, ctx, s)
	log := events.New(s)

	for i := 0; i < 5; i++ {
		_, err := log.Emit(ctx, sess.orgID, sess.sessionID, "commit.arrived",
			mustMarshal(t, map[string]int{"n": i}))
		if err != nil {
			t.Fatalf("emit %d: %v", i, err)
		}
	}

	// Limit to 2 events.
	evs, err := log.ListSince(ctx, sess.sessionID, 0, 2)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}
	if len(evs) != 2 {
		t.Errorf("want 2 events (limit), got %d", len(evs))
	}
}
