package store_test

// TestSQLiteWithTxImmediateNoDeadlock verifies that concurrent read-then-write
// WithTx calls on a SQLite store (with _txlock=immediate in the DSN) all
// succeed without spurious SQLITE_BUSY errors on lock upgrade.
//
// Without _txlock=immediate the driver opens DEFERRED transactions; two goroutines
// can each hold a read lock and then deadlock when both try to upgrade to a write
// lock. With IMMEDIATE, the write lock is acquired upfront and busy_timeout handles
// the brief wait.

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
)

func TestSQLiteWithTxImmediateNoDeadlock(t *testing.T) {
	ctx := context.Background()
	// Use a file-backed database so multiple connections share the same schema.
	// MaxOpenConns=5 allows concurrent connections so lock-upgrade contention
	// can actually occur with DEFERRED semantics (but not with IMMEDIATE).
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "immediate_test.db")
	s, rawDB, err := db.Open(ctx, "sqlite", dbPath, db.PoolConfig{MaxOpenConns: 5})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer s.Close()
	rawDB.SetMaxOpenConns(5)

	now := time.Now().UTC()

	// Seed: create an org + account + session so each goroutine has something
	// to read and then write inside its transaction.
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: "org-imm", Name: "Org", Slug: "org-imm", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: "acc-imm", Email: "imm@example.com", DisplayName: "Imm", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            "sess-imm",
		OrgID:         org.ID,
		Name:          "Imm sess",
		Goal:          "test",
		WritableScope: `["src/"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := s.EnsureEventSeqRow(ctx, sess.ID); err != nil {
		t.Fatalf("EnsureEventSeqRow: %v", err)
	}

	const goroutines = 6
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			// Read inside the transaction then write — the classic lock-upgrade
			// pattern that deadlocks with DEFERRED but not IMMEDIATE.
			errs[i] = s.WithTx(ctx, func(tx store.TxStore) error {
				// Read
				if _, err := tx.GetSession(ctx, org.ID, sess.ID); err != nil {
					return err
				}
				// Write
				eventID := acc.ID + "-ev" + string(rune('0'+i))
				seq, err := tx.AllocateNextSeq(ctx, sess.ID)
				if err != nil {
					return err
				}
				return tx.InsertEvent(ctx, store.InsertEventParams{
					ID:        eventID,
					OrgID:     org.ID,
					SessionID: sess.ID,
					Seq:       seq,
					Type:      "test.event",
					Payload:   "{}",
					CreatedAt: time.Now().UTC(),
				})
			})
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error (want no SQLITE_BUSY): %v", i, err)
		}
	}
}
