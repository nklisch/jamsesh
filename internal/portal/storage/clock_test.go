package storage_test

import (
	"context"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/storage"
)

// fakeClock is a controllable time source used to exercise the clock-
// injection path on the storage service. Mirrors the shape of fakeClock
// in internal/portal/auth/magic_link_test.go.
type fakeClock struct {
	t time.Time
}

func (f *fakeClock) Now() time.Time { return f.t }

// TestArchiveSession_UsesInjectedClock asserts that the ArchivedAt stamp
// written by ArchiveSession reflects the clock supplied to NewWithClock
// — proving the injected clock replaced the real time source.
func TestArchiveSession_UsesInjectedClock(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	fixed := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{t: fixed}
	svc := storage.NewWithClock(t.TempDir(), s, clk)

	// Seed an org/account/session so we can archive it.
	now := time.Now().UTC()
	orgID := "org-clock-test"
	sessionID := "sess-clock-test"
	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "T", Slug: "t-org", CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	if _, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: "acc-1", Email: "u@example.com", DisplayName: "U", CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessionID, OrgID: orgID, Name: "s", Goal: "g",
		WritableScope: `["**"]`, DefaultMode: "sync",
		Status: "ended", CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := svc.CreateRepo(ctx, orgID, sessionID); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	if err := svc.ArchiveSession(ctx, orgID, sessionID, storage.ArchiveInfo{
		Name:             "s",
		GoalText:         "g",
		MemberAccountIDs: []string{"acc-1"},
		EndedAt:          now,
		EndReason:        "finalize",
	}); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}

	rec, err := svc.LookupArchived(ctx, orgID, sessionID)
	if err != nil {
		t.Fatalf("LookupArchived: %v", err)
	}
	if !rec.ArchivedAt.Equal(fixed) {
		t.Errorf("ArchivedAt: want %v, got %v", fixed, rec.ArchivedAt)
	}
}
