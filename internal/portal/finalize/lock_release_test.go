package finalize_test

import (
	"context"
	"errors"
	"testing"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
)

func TestReleaseFinalizeLock_HappyPath(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	// Acquire to set up sessions pointer + status.
	if _, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	}); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	row, _ := env.store.GetActiveFinalizeLockForSession(ctx, env.sessID)

	resp, err := env.handler.ReleaseFinalizeLock(env.callerCtx, openapi.ReleaseFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    row.ID,
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, ok := resp.(openapi.ReleaseFinalizeLock204Response); !ok {
		t.Fatalf("expected 204, got %T", resp)
	}

	// Lock row released_at set.
	row2, _ := env.store.GetFinalizeLockByID(ctx, row.ID)
	if row2.ReleasedAt == nil {
		t.Error("released_at not set")
	}

	// Sessions pointer cleared.
	sess, _ := env.store.GetSession(ctx, env.orgID, env.sessID)
	if sess.FinalizeLockedByAccountID != nil {
		t.Errorf("FinalizeLockedByAccountID = %v, want nil", sess.FinalizeLockedByAccountID)
	}

	// Session status STAYS finalizing — release is not abandon.
	if sess.Status != "finalizing" {
		t.Errorf("session.status = %q, want finalizing", sess.Status)
	}
}

func TestReleaseFinalizeLock_Idempotent(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	if _, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	}); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	row, _ := env.store.GetActiveFinalizeLockForSession(ctx, env.sessID)

	for i := 0; i < 3; i++ {
		resp, err := env.handler.ReleaseFinalizeLock(env.callerCtx, openapi.ReleaseFinalizeLockRequestObject{
			OrgID:     env.orgID,
			SessionID: env.sessID,
			LockID:    row.ID,
		})
		if err != nil {
			t.Fatalf("release #%d: %v", i, err)
		}
		if _, ok := resp.(openapi.ReleaseFinalizeLock204Response); !ok {
			t.Errorf("release #%d: expected 204, got %T", i, resp)
		}
	}
}

func TestReleaseFinalizeLock_NonCaller_403(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	if _, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	}); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	row, _ := env.store.GetActiveFinalizeLockForSession(ctx, env.sessID)

	resp, err := env.handler.ReleaseFinalizeLock(env.otherCtx, openapi.ReleaseFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    row.ID,
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, ok := resp.(openapi.ReleaseFinalizeLock403JSONResponse); !ok {
		t.Fatalf("expected 403, got %T", resp)
	}

	// Lock still active.
	row2, _ := env.store.GetFinalizeLockByID(ctx, row.ID)
	if row2.ReleasedAt != nil {
		t.Error("lock should not be released by non-caller attempt")
	}
}

// Build-time check that storage stub satisfies storage.Service.
var _ store.FinalizeLock // keeps store import live in this file

// ---------------------------------------------------------------------------
// Dep-failure test
// ---------------------------------------------------------------------------

// failingReleaseLockStore wraps a real store and returns a transient error
// from ReleaseFinalizeLock, simulating a DB connection failure during the
// write-side of the release flow (after the lock has been read).
type failingReleaseLockStore struct {
	store.Store
}

func (f *failingReleaseLockStore) ReleaseFinalizeLock(_ context.Context, _ store.ReleaseFinalizeLockParams) error {
	return errors.New("conn refused")
}

func TestReleaseFinalizeLock_DBUnavailable_WrapsAsDepDB(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()

	// Acquire to set up the active lock row in the underlying store.
	if _, err := env.handler.AcquireFinalizeLock(env.callerCtx, openapi.AcquireFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	}); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	row, _ := env.store.GetActiveFinalizeLockForSession(ctx, env.sessID)

	// Build a fresh handler against a wrapping store that fails the
	// release-write call. The strict-handler translator (wired in
	// production via cmd/portal/main.go) turns the dep-wrapped error
	// into a 503 envelope; here we assert that the wrap is in place so
	// the translator has the right input.
	depHandler := newFinalizeHandlerWith(t, &failingReleaseLockStore{Store: env.store})

	_, err := depHandler.ReleaseFinalizeLock(env.callerCtx, openapi.ReleaseFinalizeLockRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
		LockID:    row.ID,
	})
	if err == nil {
		t.Fatalf("expected dep-wrapped error, got nil")
	}
	if !errors.Is(err, deperr.ErrDB) {
		t.Errorf("expected errors.Is(err, deperr.ErrDB) = true, got err=%v", err)
	}
}
