package store_test

// TestTombstoneBIGINTSQLiteRoundTrip verifies that tombstone aggregate fields
// (members_count, commits_count, auto_merges_count, duration_seconds)
// round-trip correctly as int64 through the SQLite adapter. SQLite stores all
// INTEGER columns as 64-bit, so this is the canonical working path and a
// regression guard.
//
// TestTombstoneBIGINTPostgresMigration tests the Postgres migration that widens
// those four columns from INTEGER to BIGINT. It requires JAMSESH_TEST_PG_DSN
// and is skipped otherwise.

import (
	"context"
	"math"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
)

func TestTombstoneBIGINTSQLiteRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: "ts-bigint-o", Name: "Org", Slug: "ts-bigint-o", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	_, err = s.CreateSession(ctx, store.CreateSessionParams{
		ID: "ts-bigint-s", OrgID: org.ID, Name: "S", Goal: "g",
		WritableScope: `["src/"]`, DefaultMode: "sync", Status: "active",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Use values > math.MaxInt32 to exercise the actual truncation class the
	// BIGINT widening guards against — these literals would not compile if the
	// generated params were still int32, and would truncate if any int32 cast
	// remained on the write path.
	want := store.RecordTombstoneParams{
		SessionID:       "ts-bigint-s",
		OrgID:           org.ID,
		MembersCount:    42,
		CommitsCount:    int64(math.MaxInt32) + 1000, // > 2^31
		AutoMergesCount: 7,
		DurationSeconds: 5_000_000_000, // > math.MaxInt32
		EndReason:       "idle",
		EndedAt:         now.Truncate(time.Second),
		ExpiresAt:       now.Add(30 * 24 * time.Hour).Truncate(time.Second),
	}
	if err := s.RecordTombstone(ctx, want); err != nil {
		t.Fatalf("RecordTombstone: %v", err)
	}

	got, err := s.GetTombstone(ctx, want.SessionID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}

	if got.MembersCount != want.MembersCount {
		t.Errorf("MembersCount = %d, want %d", got.MembersCount, want.MembersCount)
	}
	if got.CommitsCount != want.CommitsCount {
		t.Errorf("CommitsCount = %d, want %d", got.CommitsCount, want.CommitsCount)
	}
	if got.AutoMergesCount != want.AutoMergesCount {
		t.Errorf("AutoMergesCount = %d, want %d", got.AutoMergesCount, want.AutoMergesCount)
	}
	if got.DurationSeconds != want.DurationSeconds {
		t.Errorf("DurationSeconds = %d, want %d", got.DurationSeconds, want.DurationSeconds)
	}
}

func TestTombstoneBIGINTPostgresMigration(t *testing.T) {
	dsn := skipIfNoPGDSN(t)
	ctx := context.Background()

	// Open a fresh Postgres store (runs all migrations including the BIGINT one).
	s, _, err := db.Open(ctx, "postgres", dsn, db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open postgres: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: "pgts-bigint-o", Name: "Org", Slug: "pgts-bigint-o", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	_, err = s.CreateSession(ctx, store.CreateSessionParams{
		ID: "pgts-bigint-s", OrgID: org.ID, Name: "S", Goal: "g",
		WritableScope: `["src/"]`, DefaultMode: "sync", Status: "active",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Values > math.MaxInt32 — an INTEGER column (pre-migration) could not hold
	// these; round-tripping them confirms the BIGINT migration applied.
	want := store.RecordTombstoneParams{
		SessionID:       "pgts-bigint-s",
		OrgID:           org.ID,
		MembersCount:    5,
		CommitsCount:    int64(math.MaxInt32) + 200, // > 2^31
		AutoMergesCount: 3,
		DurationSeconds: 5_000_000_000, // > math.MaxInt32
		EndReason:       "hard_cap",
		EndedAt:         now.Truncate(time.Second),
		ExpiresAt:       now.Add(30 * 24 * time.Hour).Truncate(time.Second),
	}
	if err := s.RecordTombstone(ctx, want); err != nil {
		t.Fatalf("RecordTombstone: %v", err)
	}

	got, err := s.GetTombstone(ctx, want.SessionID)
	if err != nil {
		t.Fatalf("GetTombstone: %v", err)
	}

	if got.MembersCount != want.MembersCount {
		t.Errorf("MembersCount = %d, want %d", got.MembersCount, want.MembersCount)
	}
	if got.CommitsCount != want.CommitsCount {
		t.Errorf("CommitsCount = %d, want %d", got.CommitsCount, want.CommitsCount)
	}
	if got.AutoMergesCount != want.AutoMergesCount {
		t.Errorf("AutoMergesCount = %d, want %d", got.AutoMergesCount, want.AutoMergesCount)
	}
	if got.DurationSeconds != want.DurationSeconds {
		t.Errorf("DurationSeconds = %d, want %d", got.DurationSeconds, want.DurationSeconds)
	}
}
