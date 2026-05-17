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
	"jamsesh/internal/portal/tokens"
)

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
		return nil, fmt.Errorf("finalize: membership check: %w", err)
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
		return nil, fmt.Errorf("finalize: get session: %w", err)
	}

	now := time.Now().UTC()

	// Look up the current active lock (if any).
	existing, err := h.store.GetActiveFinalizeLockForSession(ctx, sessionID)
	hadExisting := true
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("finalize: get active lock: %w", err)
		}
		hadExisting = false
	}

	var newLockID string
	var newLockAcquiredAt time.Time
	var newLockLastActivity time.Time
	supersedeOldID := "" // set in branch 5; supersede is performed after insert
	emitFinalizing := false

	if hadExisting {
		// Branch 2: caller already holds an active lock — idempotent.
		if existing.AcquiredByAccountID == acc.ID {
			return openapi.AcquireFinalizeLock201JSONResponse(lockStatus(existing, true)), nil
		}

		// Branch 3: stale lock — release and proceed.
		if IsLockExpired(existing.LastActivityAt, now) {
			if err := h.store.ReleaseFinalizeLock(ctx, store.ReleaseFinalizeLockParams{
				ID:         existing.ID,
				ReleasedAt: now,
			}); err != nil {
				return nil, fmt.Errorf("finalize: release stale lock: %w", err)
			}
			hadExisting = false
		} else if !override {
			// Branch 4: fresh, held by another member, no override — 409.
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
		} else {
			// Branch 5: fresh, held by another member, override — defer
			// the supersede until the new row exists (self-FK on
			// superseded_by_lock_id requires the target to exist first).
			supersedeOldID = existing.ID
		}
	}

	newLockID = ulid.Make().String()
	newLockAcquiredAt = now
	newLockLastActivity = now

	if err := h.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
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
		return nil, fmt.Errorf("finalize: insert lock: %w", err)
	}

	if supersedeOldID != "" {
		if err := h.store.SupersedeFinalizeLock(ctx, store.SupersedeFinalizeLockParams{
			ID:                 supersedeOldID,
			SupersededByLockID: newLockID,
		}); err != nil {
			return nil, fmt.Errorf("finalize: supersede lock: %w", err)
		}
	}

	accID := acc.ID
	if err := h.store.SetFinalizeLock(ctx, store.SetFinalizeLockParams{
		OrgID:     orgID,
		ID:        sessionID,
		AccountID: &accID,
	}); err != nil {
		return nil, fmt.Errorf("finalize: set sessions pointer: %w", err)
	}

	if sess.Status == "active" {
		if err := h.store.UpdateSessionStatus(ctx, store.UpdateSessionStatusParams{
			OrgID:  orgID,
			ID:     sessionID,
			Status: "finalizing",
		}); err != nil {
			return nil, fmt.Errorf("finalize: update session status: %w", err)
		}
		emitFinalizing = true
	}

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
