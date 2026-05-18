package finalize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/tokens"
)

// PatchFinalizeLock implements PATCH
// /api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}.
//
// Preconditions:
//   - Lock row exists.
//   - Lock belongs to the path session.
//   - Lock is not released.
//   - Lock is not superseded.
//   - Caller is the lock holder.
//   - Lock is not idle (>30 min). If idle, the row is auto-released and
//     409 finalize.lock_expired is returned.
//
// Effects: replaces selected_commit_shas, target_branch, base_sha, mode,
// commit_message; bumps last_activity_at.
func (h *Handler) PatchFinalizeLock(ctx context.Context, req openapi.PatchFinalizeLockRequestObject) (openapi.PatchFinalizeLockResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.PatchFinalizeLock401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID
	lockID := req.LockID

	if req.Body == nil {
		return openapi.PatchFinalizeLock400JSONResponse(openapi.ErrorEnvelope{
			Error:   "request.invalid",
			Message: "request body is required",
		}), nil
	}

	verdict, err := checkSessionMembership(ctx, h.store, orgID, sessionID, acc.ID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: membership check: %w", err))
	}
	switch verdict {
	case memberNotOrgMember:
		return openapi.PatchFinalizeLock403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this org",
			},
		}, nil
	case memberNotSessionMember:
		return openapi.PatchFinalizeLock403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this session",
			},
		}, nil
	case memberSessionNotFound:
		return openapi.PatchFinalizeLock404JSONResponse{
			NotFoundJSONResponse: openapi.NotFoundJSONResponse{
				Error:   "session.not_found",
				Message: "session not found",
			},
		}, nil
	}

	lock, err := h.store.GetFinalizeLockByID(ctx, lockID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.PatchFinalizeLock404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "finalize.lock_not_found",
					Message: "finalize lock not found",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: get lock: %w", err))
	}

	// Cross-session id mismatch ⇒ 404 (don't leak existence on other sessions).
	if lock.SessionID != sessionID || lock.OrgID != orgID {
		return openapi.PatchFinalizeLock404JSONResponse{
			NotFoundJSONResponse: openapi.NotFoundJSONResponse{
				Error:   "finalize.lock_not_found",
				Message: "finalize lock not found",
			},
		}, nil
	}

	// Caller-only.
	if lock.AcquiredByAccountID != acc.ID {
		return openapi.PatchFinalizeLock403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "only the lock holder can patch this lock",
			},
		}, nil
	}

	// Already released / superseded — clients should re-acquire.
	// Check superseded before released: a lock that was overridden in the
	// concurrent-override path will have both fields set (released_at is set
	// first to remove it from the unique-active-index scope, then
	// superseded_by_lock_id is set for audit). "Superseded" is the more
	// actionable signal for callers — they should re-query to find who won.
	if lock.SupersededByLockID != nil {
		details := map[string]interface{}{
			"superseded_by_lock_id": *lock.SupersededByLockID,
		}
		return openapi.PatchFinalizeLock409JSONResponse(openapi.ErrorEnvelope{
			Error:   "finalize.lock_superseded",
			Message: "finalize lock has been superseded",
			Details: details,
		}), nil
	}
	if lock.ReleasedAt != nil {
		return openapi.PatchFinalizeLock409JSONResponse(openapi.ErrorEnvelope{
			Error:   "finalize.lock_released",
			Message: "finalize lock has been released",
		}), nil
	}

	now := h.clock.Now()

	// Idle check — releases the row on expiry and returns 409.
	if IsLockExpired(lock.LastActivityAt, now) {
		_ = h.store.ReleaseFinalizeLock(ctx, store.ReleaseFinalizeLockParams{
			ID:         lock.ID,
			ReleasedAt: now,
		})
		return openapi.PatchFinalizeLock409JSONResponse(openapi.ErrorEnvelope{
			Error:   "finalize.lock_expired",
			Message: "finalize lock idle for more than 30 minutes; lock has been released",
		}), nil
	}

	// Validate mode + commit_message coupling. mode=squash with empty
	// commit_message is allowed at PATCH time (the curation UI may bind
	// the message later); the contract requirement is enforced at
	// plan-generation time (story 2).
	mode := string(req.Body.Mode)
	if mode != "squash" && mode != "preserve" {
		return openapi.PatchFinalizeLock400JSONResponse(openapi.ErrorEnvelope{
			Error:   "finalize.invalid_mode",
			Message: "mode must be one of: squash, preserve",
		}), nil
	}

	// Validate target_branch: must be non-empty, match ^[A-Za-z0-9._/-]+$,
	// and must not start with '-' (which git treats as a flag).
	if !ValidateTargetBranch(req.Body.TargetBranch) {
		return openapi.PatchFinalizeLock400JSONResponse(openapi.ErrorEnvelope{
			Error:   "session.invalid_target_branch",
			Message: "target_branch must be non-empty, contain only [A-Za-z0-9._/-], and must not start with '-'",
		}), nil
	}

	// Validate base_sha: must be a full 40-hex-character SHA-1.
	if !ValidateBaseSHA(req.Body.BaseSha) {
		return openapi.PatchFinalizeLock400JSONResponse(openapi.ErrorEnvelope{
			Error:   "session.invalid_base_sha",
			Message: "base_sha must be a 40-character lowercase hex SHA-1",
		}), nil
	}

	selected := req.Body.SelectedCommitShas
	if selected == nil {
		selected = []string{}
	}
	shasJSON, err := json.Marshal(selected)
	if err != nil {
		return nil, fmt.Errorf("finalize: marshal selected_commit_shas: %w", err)
	}

	var commitMessage *string
	if req.Body.CommitMessage != "" {
		cm := req.Body.CommitMessage
		commitMessage = &cm
	}

	if err := h.store.UpdateFinalizeLockCuration(ctx, store.UpdateFinalizeLockCurationParams{
		ID:                 lock.ID,
		SelectedCommitSHAs: string(shasJSON),
		TargetBranch:       req.Body.TargetBranch,
		BaseSHA:            req.Body.BaseSha,
		Mode:               mode,
		CommitMessage:      commitMessage,
		LastActivityAt:     now,
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: update lock curation: %w", err))
	}

	// Re-fetch for response consistency.
	updated, err := h.store.GetFinalizeLockByID(ctx, lock.ID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: re-fetch lock: %w", err))
	}

	return openapi.PatchFinalizeLock200JSONResponse(finalizeLockToOpenAPI(updated)), nil
}

// finalizeLockToOpenAPI projects a domain FinalizeLock into the openapi
// response shape. selected_commit_shas is unmarshalled from its JSON
// storage form back to a []string for the client.
func finalizeLockToOpenAPI(l store.FinalizeLock) openapi.FinalizeLock {
	var shas []string
	if l.SelectedCommitSHAs != "" {
		_ = json.Unmarshal([]byte(l.SelectedCommitSHAs), &shas)
	}
	if shas == nil {
		shas = []string{}
	}
	out := openapi.FinalizeLock{
		Id:                  l.ID,
		SessionId:           l.SessionID,
		AcquiredByAccountId: l.AcquiredByAccountID,
		AcquiredAt:          l.AcquiredAt,
		LastActivityAt:      l.LastActivityAt,
		ExpiresAt:           LockExpiresAt(l.LastActivityAt),
		SelectedCommitShas:  shas,
		TargetBranch:        l.TargetBranch,
		BaseSha:             l.BaseSHA,
		Mode:                openapi.PlanMode(l.Mode),
	}
	if l.CommitMessage != nil {
		out.CommitMessage = *l.CommitMessage
	}
	return out
}
