package finalize_test

import (
	"context"
	"testing"
	"time"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/finalize"
	"jamsesh/internal/portal/tokens"
)

// handlerFakeClock is a controllable time source used to exercise the clock-
// injection path on the finalize.Handler. Mirrors the shape of handlerFakeClock
// in internal/portal/auth/magic_link_test.go.
type handlerFakeClock struct {
	t time.Time
}

func (f *handlerFakeClock) Now() time.Time { return f.t }

// TestHandler_AcquireLockUsesInjectedClock asserts that the LastActivityAt
// and AcquiredAt stamps written by AcquireFinalizeLock reflect the clock
// supplied to NewWithClock — i.e. the injected clock fully replaced the
// real time source.
func TestHandler_AcquireLockUsesInjectedClock(t *testing.T) {
	env := newFinalizeEnv(t)

	fixed := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	clk := &handlerFakeClock{t: fixed}

	log := events.New(env.store)
	tokSvc := tokens.New(env.store)
	handler := finalize.NewWithClock(env.store, &stubStorage{}, log, tokSvc, "https://portal.test", clk)

	resp, err := handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("AcquireFinalizeLock: %v", err)
	}
	ok, isOK := resp.(openapi.AcquireFinalizeLock201JSONResponse)
	if !isOK {
		t.Fatalf("want 201 response, got %T (resp=%+v)", resp, resp)
	}
	if !ok.AcquiredAt.Equal(fixed) {
		t.Errorf("AcquiredAt: want %v, got %v", fixed, ok.AcquiredAt)
	}

	// Re-fetch from store and confirm the same.
	lock, err := env.store.GetActiveFinalizeLockForSession(context.Background(), env.sessID)
	if err != nil {
		t.Fatalf("GetActiveFinalizeLockForSession: %v", err)
	}
	if !lock.LastActivityAt.Equal(fixed) {
		t.Errorf("LastActivityAt: want %v, got %v", fixed, lock.LastActivityAt)
	}
}

// TestHandler_NewVsNewWithClock_ProductionPathClean asserts that the
// default New() constructor produces a Handler whose acquire still works
// (the realClock path). Regression check that the constructor refactor
// didn't break the default path.
func TestHandler_NewVsNewWithClock_ProductionPathClean(t *testing.T) {
	env := newFinalizeEnv(t)
	// env.handler was built via finalize.New(...) — exercise it.
	before := time.Now().UTC()
	resp, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("AcquireFinalizeLock: %v", err)
	}
	if _, ok := resp.(openapi.AcquireFinalizeLock201JSONResponse); !ok {
		t.Fatalf("want 201 response, got %T", resp)
	}
	after := time.Now().UTC()

	// The realClock should produce a stamp inside [before, after].
	lock, err := env.store.GetActiveFinalizeLockForSession(context.Background(), env.sessID)
	if err != nil {
		t.Fatalf("GetActiveFinalizeLockForSession: %v", err)
	}
	if lock.AcquiredAt.Before(before) || lock.AcquiredAt.After(after) {
		t.Errorf("AcquiredAt %v not in [%v, %v]", lock.AcquiredAt, before, after)
	}
}
