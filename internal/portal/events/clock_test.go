package events_test

import (
	"context"
	"testing"
	"time"

	"jamsesh/internal/portal/events"
)

// fakeClock is a controllable time source used to exercise the clock-
// injection path on the Log. Mirrors the shape of the fakeClock in
// internal/portal/auth's magic_link_test.go — kept local so the events_test
// package owns its own copy (test packages can't share unexported types).
type fakeClock struct {
	t time.Time
}

func (f *fakeClock) Now() time.Time { return f.t }

// TestLog_EmitUsesInjectedClock asserts that the CreatedAt timestamp on
// events written through a Log built with NewWithClock matches the fake
// clock's Now() — i.e. the injected clock fully replaced the real time
// source for Emit.
func TestLog_EmitUsesInjectedClock(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess := mustSetupSession(t, ctx, s)

	fixed := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{t: fixed}
	log := events.NewWithClock(s, clk)

	if _, err := log.Emit(ctx, sess.orgID, sess.sessionID, "commit.arrived", mustMarshal(t, map[string]string{"sha": "abc"})); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	rows, err := log.ListSince(ctx, sess.sessionID, 0, 10)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if !rows[0].CreatedAt.Equal(fixed) {
		t.Errorf("CreatedAt: want %v, got %v", fixed, rows[0].CreatedAt)
	}
}

// TestLog_EmitBatchUsesInjectedClock asserts EmitBatch writes use the
// injected clock for all rows in the batch.
func TestLog_EmitBatchUsesInjectedClock(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	sess := mustSetupSession(t, ctx, s)

	fixed := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{t: fixed}
	log := events.NewWithClock(s, clk)

	drafts := []events.DraftEvent{
		{Type: "commit.arrived", Payload: mustMarshal(t, map[string]string{"sha": "a"})},
		{Type: "commit.arrived", Payload: mustMarshal(t, map[string]string{"sha": "b"})},
	}
	if _, err := log.EmitBatch(ctx, sess.orgID, sess.sessionID, drafts); err != nil {
		t.Fatalf("EmitBatch: %v", err)
	}

	rows, err := log.ListSince(ctx, sess.sessionID, 0, 10)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	for i, r := range rows {
		if !r.CreatedAt.Equal(fixed) {
			t.Errorf("rows[%d].CreatedAt: want %v, got %v", i, fixed, r.CreatedAt)
		}
	}
}
