package finalize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/tokens"
)

// ErrOverrideRaceLost is returned (wrapped) when two callers simultaneously
// attempt AcquireFinalizeLock(override=true) and this caller's INSERT lost the
// unique-index race. The other caller's row is now the active lock; callers
// should re-query rather than retry blindly.
//
// Contract: when this error is returned, NO new lock row was inserted for the
// loser. The session's active lock is held by whichever caller won the race.
var ErrOverrideRaceLost = errors.New("finalize: override race lost — another caller inserted the active lock first")

// errStaleLockRefreshed is a private sentinel returned from inside the tx
// closure when the holder refreshed last_activity_at between the preflight
// read and the conditional release UPDATE (TOCTOU window). The tx is rolled
// back and the caller receives a 409 (lock_held_by_other).
var errStaleLockRefreshed = errors.New("finalize: stale lock was refreshed before conditional release")

// AcquireFinalizeLock implements POST
// /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock.
//
// Branches:
//
//  1. No active lock — insert a new lock row, transition session.status
//     active → finalizing (idempotent if already finalizing), set
//     sessions.finalize_locked_by_account_id = caller, emit
//     session.finalizing.
//  2. Active lock held by caller — idempotent: return the existing lock
//     status without writing.
//  3. Active lock held by another member AND idle > 30 min — release the
//     stale row (set released_at) and proceed as in (1).
//  4. Active lock held by another member AND fresh AND override=false
//     — return 409 finalize.lock_held_by_other.
//  5. Active lock held by another member AND fresh AND override=true
//     — supersede the existing row (set superseded_by_lock_id to the new
//     lock's id) and proceed as in (1) with sessions pointer reassigned
//     to caller. session.finalizing is NOT re-emitted in this branch
//     (status was already finalizing).
func (h *Handler) AcquireFinalizeLock(ctx context.Context, req openapi.AcquireFinalizeLockRequestObject) (openapi.AcquireFinalizeLockResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.AcquireFinalizeLock401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID

	override := false
	if req.Body != nil {
		override = req.Body.Override
	}

	verdict, err := checkSessionMembership(ctx, h.store, orgID, sessionID, acc.ID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: membership check: %w", err))
	}
	switch verdict {
	case memberNotOrgMember:
		return openapi.AcquireFinalizeLock403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this org",
			},
		}, nil
	case memberNotSessionMember:
		return openapi.AcquireFinalizeLock403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this session",
			},
		}, nil
	case memberSessionNotFound:
		return openapi.AcquireFinalizeLock404JSONResponse{
			NotFoundJSONResponse: openapi.NotFoundJSONResponse{
				Error:   "session.not_found",
				Message: "session not found",
			},
		}, nil
	}

	sess, err := h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.AcquireFinalizeLock404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: get session: %w", err))
	}

	now := h.clock.Now()

	// Look up the current active lock (if any).
	existing, err := h.store.GetActiveFinalizeLockForSession(ctx, sessionID)
	hadExisting := true
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: get active lock: %w", err))
		}
		hadExisting = false
	}

	var newLockID string
	var newLockAcquiredAt time.Time
	var newLockLastActivity time.Time
	supersedeOldID := "" // set in branch 5; supersede is performed after insert
	emitFinalizing := false

	if hadExisting {
		// Branch 2: caller already holds an active lock — idempotent (read-only, no tx needed).
		if existing.AcquiredByAccountID == acc.ID {
			return openapi.AcquireFinalizeLock201JSONResponse(lockStatus(existing, true)), nil
		}

		// Branch 4: fresh lock, held by another member, no override — 409 (no mutation).
		if !IsLockExpired(existing.LastActivityAt, now) && !override {
			details := map[string]interface{}{
				"held_by_account_id": existing.AcquiredByAccountID,
				"lock_id":            existing.ID,
				"expires_at":         LockExpiresAt(existing.LastActivityAt).Format(time.RFC3339Nano),
			}
			return openapi.AcquireFinalizeLock409JSONResponse(openapi.ErrorEnvelope{
				Error:   "finalize.lock_held_by_other",
				Message: "another member holds the finalize lock for this session",
				Details: details,
			}), nil
		}

		// Branch 3: stale lock; branch 5: override. Both proceed to the tx below.
		// stale: hadExisting=true, IsLockExpired=true → needRelease=true, supersedeOldID=""
		// override: hadExisting=true, fresh, override=true → needRelease=true, supersedeOldID=existing.ID
		if IsLockExpired(existing.LastActivityAt, now) {
			// Branch 3: stale — release inside the tx, no supersede marker needed.
			// supersedeOldID stays ""
		} else {
			// Branch 5: fresh override — release inside tx, set supersede marker after insert.
			supersedeOldID = existing.ID
		}
	}

	newLockID = ulid.Make().String()
	newLockAcquiredAt = now
	newLockLastActivity = now

	// Wrap the entire mutation sequence in a single transaction so a failure at
	// any step rolls back all prior mutations in this sequence. This prevents
	// partial state (e.g. ReleaseFinalizeLock succeeded but InsertFinalizeLock
	// failed) that would leave the session in an inconsistent state.
	// Pre-flight READS (existing-lock lookup, session load) are kept OUTSIDE the
	// tx above; only mutations belong in the closure.
	existingID := "" // captured for use inside tx closure
	staleRelease := false
	if hadExisting {
		existingID = existing.ID
		// Branch 3 (stale takeover): the preflight decided the lock was expired.
		// We must perform the release atomically, conditioned on last_activity_at
		// still being below the staleness cutoff, to close the TOCTOU window
		// where the holder refreshes last_activity_at between the preflight read
		// and the release. Override (branch 5) does not need this guard because
		// it applies regardless of freshness.
		staleRelease = supersedeOldID == "" // only branch 3, not branch 5
	}
	// staleCutoff is the exclusive upper bound used by the conditional UPDATE:
	// the lock row must still have last_activity_at < staleCutoff at the moment
	// the UPDATE executes. This is now - FinalizeLockTTL (the expiry threshold).
	staleCutoff := now.Add(-FinalizeLockTTL)
	err = h.store.WithTx(ctx, func(tx store.TxStore) error {
		// Branch 3 or 5: release the current lock (stale or override) first so
		// InsertFinalizeLock can satisfy the unique partial index.
		if hadExisting {
			if staleRelease {
				// Branch 3 (stale takeover): use a single conditional UPDATE that
				// atomically guards on last_activity_at < staleCutoff. If the holder
				// refreshed the lock after the preflight read, the WHERE predicate
				// evaluates to false under the row lock and 0 rows are affected —
				// we abort the takeover and return 409. This is the only correct
				// way to close the TOCTOU window; a separate SELECT then UPDATE
				// cannot close it because the holder can refresh between the two
				// statements.
				n, err := tx.ReleaseFinalizeLockIfStale(ctx, store.ReleaseFinalizeLockIfStaleParams{
					ID:         existingID,
					ReleasedAt: now,
					Cutoff:     staleCutoff,
				})
				if err != nil {
					return fmt.Errorf("conditional release stale lock: %w", err)
				}
				if n == 0 {
					// Holder refreshed the lock between preflight and conditional
					// UPDATE — the lock is now live. Abort the takeover.
					return errStaleLockRefreshed
				}
			} else {
				// Branch 5 (override): unconditional release; override applies
				// regardless of freshness.
				if err := tx.ReleaseFinalizeLock(ctx, store.ReleaseFinalizeLockParams{
					ID:         existingID,
					ReleasedAt: now,
				}); err != nil {
					return fmt.Errorf("release lock (override): %w", err)
				}
			}
		}

		if err := tx.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
			ID:                  newLockID,
			OrgID:               orgID,
			SessionID:           sessionID,
			AcquiredByAccountID: acc.ID,
			AcquiredAt:          newLockAcquiredAt,
			LastActivityAt:      newLockLastActivity,
			SelectedCommitSHAs:  "[]",
			TargetBranch:        "",
			BaseSHA:             "",
			Mode:                "squash",
		}); err != nil {
			return fmt.Errorf("insert lock: %w", err)
		}

		if supersedeOldID != "" {
			if err := tx.SupersedeFinalizeLock(ctx, store.SupersedeFinalizeLockParams{
				ID:                 supersedeOldID,
				SupersededByLockID: newLockID,
			}); err != nil {
				return fmt.Errorf("supersede lock: %w", err)
			}
		}

		accIDPtr := acc.ID
		if err := tx.SetFinalizeLock(ctx, store.SetFinalizeLockParams{
			OrgID:     orgID,
			ID:        sessionID,
			AccountID: &accIDPtr,
		}); err != nil {
			return fmt.Errorf("set sessions pointer: %w", err)
		}

		if sess.Status == "active" {
			if err := tx.UpdateSessionStatus(ctx, store.UpdateSessionStatusParams{
				OrgID:  orgID,
				ID:     sessionID,
				Status: "finalizing",
			}); err != nil {
				return fmt.Errorf("update session status: %w", err)
			}
			emitFinalizing = true
		}

		return nil
	})
	// Return 409 when the stale lock was refreshed between preflight and tx
	// (TOCTOU: holder is still alive — do not takeover).
	if errors.Is(err, errStaleLockRefreshed) {
		details := map[string]interface{}{
			"held_by_account_id": existing.AcquiredByAccountID,
			"lock_id":            existing.ID,
			"expires_at":         LockExpiresAt(existing.LastActivityAt).Format(time.RFC3339Nano),
		}
		return openapi.AcquireFinalizeLock409JSONResponse(openapi.ErrorEnvelope{
			Error:   "finalize.lock_held_by_other",
			Message: "another member holds the finalize lock for this session",
			Details: details,
		}), nil
	}
	// Return 409 on unique-violation regardless of supersedeOldID — a fresh-insert
	// race (no prior override release) also hits the unique partial index.
	if errors.Is(err, store.ErrUniqueViolation) {
		return openapi.AcquireFinalizeLock409JSONResponse(openapi.ErrorEnvelope{
			Error:   "finalize.override_race_lost",
			Message: "another caller acquired the finalize lock simultaneously; re-query to see the current lock holder",
		}), nil
	}
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: acquire lock tx: %w", err))
	}

	// Emit session.finalizing AFTER the transaction commits (tx-emit-then-fanout).
	if emitFinalizing {
		type sessionFinalizingPayload struct {
			SessionID string `json:"session_id"`
			OrgID     string `json:"org_id"`
		}
		payload, _ := json.Marshal(sessionFinalizingPayload{SessionID: sessionID, OrgID: orgID})
		_, _ = h.events.Emit(ctx, orgID, sessionID, "session.finalizing", payload)
	}

	return openapi.AcquireFinalizeLock201JSONResponse(openapi.LockStatus{
		LockId:          newLockID,
		HeldByAccountId: acc.ID,
		AcquiredAt:      newLockAcquiredAt,
		LastActivityAt:  newLockLastActivity,
		ExpiresAt:       LockExpiresAt(newLockLastActivity),
		IsCaller:        true,
	}), nil
}

// lockStatus projects a domain FinalizeLock + caller relationship into the
// openapi.LockStatus DTO. Used by acquire/patch return paths.
func lockStatus(lock store.FinalizeLock, isCaller bool) openapi.LockStatus {
	return openapi.LockStatus{
		LockId:          lock.ID,
		HeldByAccountId: lock.AcquiredByAccountID,
		AcquiredAt:      lock.AcquiredAt,
		LastActivityAt:  lock.LastActivityAt,
		ExpiresAt:       LockExpiresAt(lock.LastActivityAt),
		IsCaller:        isCaller,
	}
}
