package store_test

// TestSeqBIGINTSQLiteRoundTrip verifies that seq values round-trip correctly
// as int64 through the SQLite adapter (SQLite always stores int64 for INTEGER
// columns; this is the canonical working path and a regression guard).
//
// TestSeqBIGINTPostgresMigration tests the Postgres migration that widens
// events.seq and event_seq.next from INTEGER to BIGINT. It requires
// JAMSESH_TEST_PG_DSN and is skipped otherwise.

import (
	"context"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
)

func TestSeqBIGINTSQLiteRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	org, _ := s.CreateOrg(ctx, store.CreateOrgParams{ID: "o1", Name: "Org", Slug: "o1", CreatedAt: now})
	acc, _ := s.CreateAccount(ctx, store.CreateAccountParams{ID: "a1", Email: "a@b.com", DisplayName: "A", CreatedAt: now})
	sess, _ := s.CreateSession(ctx, store.CreateSessionParams{
		ID: "s1", OrgID: org.ID, Name: "S", Goal: "g", WritableScope: `["src/"]`,
		DefaultMode: "sync", Status: "active", CreatedAt: now,
	})

	if err := s.EnsureEventSeqRow(ctx, sess.ID); err != nil {
		t.Fatalf("EnsureEventSeqRow: %v", err)
	}

	// Insert several events and verify seq is a monotonically increasing int64.
	var prevSeq int64
	for i := 0; i < 5; i++ {
		var seq int64
		err = s.WithTx(ctx, func(tx store.TxStore) error {
			var e error
			seq, e = tx.AllocateNextSeq(ctx, sess.ID)
			if e != nil {
				return e
			}
			return tx.InsertEvent(ctx, store.InsertEventParams{
				ID:        acc.ID + strings.Repeat("x", i+1), // unique id per iteration
				OrgID:     org.ID,
				SessionID: sess.ID,
				Seq:       seq,
				Type:      "test.event",
				Payload:   "{}",
				CreatedAt: now,
			})
		})
		if err != nil {
			t.Fatalf("tx %d: %v", i, err)
		}
		if seq <= 0 {
			t.Errorf("iteration %d: seq = %d, want > 0", i, seq)
		}
		if seq <= prevSeq {
			t.Errorf("iteration %d: seq = %d not greater than prev %d", i, seq, prevSeq)
		}
		prevSeq = seq
	}

	// Verify the events are retrievable with the correct int64 seq values.
	events, err := s.ListEventsSince(ctx, store.ListEventsSinceParams{
		SessionID: sess.ID,
		SinceSeq:  0,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("ListEventsSince: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("got %d events, want 5", len(events))
	}
	for i, e := range events {
		wantSeq := int64(i + 1)
		if e.Seq != wantSeq {
			t.Errorf("events[%d].Seq = %d, want %d", i, e.Seq, wantSeq)
		}
	}
}

func TestSeqBIGINTPostgresMigration(t *testing.T) {
	dsn := skipIfNoPGDSN(t)
	ctx := context.Background()

	// Open a fresh Postgres store (runs all migrations including the BIGINT one).
	s, _, err := db.Open(ctx, "postgres", dsn, db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open postgres: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	org, _ := s.CreateOrg(ctx, store.CreateOrgParams{ID: "pgbig-o", Name: "Org", Slug: "pgbig-o", CreatedAt: now})
	acc, _ := s.CreateAccount(ctx, store.CreateAccountParams{ID: "pgbig-a", Email: "pgbig@b.com", DisplayName: "A", CreatedAt: now})
	sess, _ := s.CreateSession(ctx, store.CreateSessionParams{
		ID: "pgbig-s", OrgID: org.ID, Name: "S", Goal: "g", WritableScope: `["src/"]`,
		DefaultMode: "sync", Status: "active", CreatedAt: now,
	})

	if err := s.EnsureEventSeqRow(ctx, sess.ID); err != nil {
		t.Fatalf("EnsureEventSeqRow: %v", err)
	}

	// Insert an event and verify it round-trips as int64 (not int32).
	var seq int64
	if err := s.WithTx(ctx, func(tx store.TxStore) error {
		var e error
		seq, e = tx.AllocateNextSeq(ctx, sess.ID)
		if e != nil {
			return e
		}
		return tx.InsertEvent(ctx, store.InsertEventParams{
			ID: acc.ID + "-ev", OrgID: org.ID, SessionID: sess.ID,
			Seq: seq, Type: "test.event", Payload: "{}", CreatedAt: now,
		})
	}); err != nil {
		t.Fatalf("tx: %v", err)
	}
	if seq != 1 {
		t.Errorf("first seq = %d, want 1", seq)
	}

	events, err := s.ListEventsSince(ctx, store.ListEventsSinceParams{
		SessionID: sess.ID, SinceSeq: 0, Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListEventsSince: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Seq != 1 {
		t.Errorf("events[0].Seq = %d, want 1", events[0].Seq)
	}
}
