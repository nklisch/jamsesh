package finalize

import (
	"context"
	"errors"
	"fmt"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/tokens"
)

// ReleaseFinalizeLock implements DELETE
// /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}.
//
// Idempotent: releasing an already-released lock returns 204. Only the
// lock holder can release. On success, clears the sessions
// finalize_locked_by pointer when the released lock was the active one.
// Session status stays finalizing — release is not the same as abandon.
func (h *Handler) ReleaseFinalizeLock(ctx context.Context, req openapi.ReleaseFinalizeLockRequestObject) (openapi.ReleaseFinalizeLockResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.ReleaseFinalizeLock401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID
	lockID := req.LockID

	verdict, err := checkSessionMembership(ctx, h.store, orgID, sessionID, acc.ID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: membership check: %w", err))
	}
	switch verdict {
	case memberNotOrgMember:
		return openapi.ReleaseFinalizeLock403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this org",
			},
		}, nil
	case memberNotSessionMember:
		return openapi.ReleaseFinalizeLock403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this session",
			},
		}, nil
	case memberSessionNotFound:
		return openapi.ReleaseFinalizeLock404JSONResponse{
			NotFoundJSONResponse: openapi.NotFoundJSONResponse{
				Error:   "session.not_found",
				Message: "session not found",
			},
		}, nil
	}

	lock, err := h.store.GetFinalizeLockByID(ctx, lockID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.ReleaseFinalizeLock404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "finalize.lock_not_found",
					Message: "finalize lock not found",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: get lock: %w", err))
	}

	// Cross-session id mismatch ⇒ 404.
	if lock.SessionID != sessionID || lock.OrgID != orgID {
		return openapi.ReleaseFinalizeLock404JSONResponse{
			NotFoundJSONResponse: openapi.NotFoundJSONResponse{
				Error:   "finalize.lock_not_found",
				Message: "finalize lock not found",
			},
		}, nil
	}

	// Caller-only.
	if lock.AcquiredByAccountID != acc.ID {
		return openapi.ReleaseFinalizeLock403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "only the lock holder can release this lock",
			},
		}, nil
	}

	// Idempotent: if already released, return 204 without re-touching.
	if lock.ReleasedAt != nil {
		return openapi.ReleaseFinalizeLock204Response{}, nil
	}

	now := h.clock.Now()
	if err := h.store.ReleaseFinalizeLock(ctx, store.ReleaseFinalizeLockParams{
		ID:         lock.ID,
		ReleasedAt: now,
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: release lock: %w", err))
	}

	// Clear the sessions pointer if it points at this lock's holder.
	// (If a parallel acquire from another member already reassigned the
	// pointer, we leave it alone — release should never blast a
	// different active holder's pointer.)
	sess, sErr := h.store.GetSession(ctx, orgID, sessionID)
	if sErr == nil && sess.FinalizeLockedByAccountID != nil && *sess.FinalizeLockedByAccountID == lock.AcquiredByAccountID {
		_ = h.store.ClearFinalizeLock(ctx, store.ClearFinalizeLockParams{
			OrgID: orgID,
			ID:    sessionID,
		})
	}

	return openapi.ReleaseFinalizeLock204Response{}, nil
}
