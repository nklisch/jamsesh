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

// MarkSessionShipped implements POST
// /api/orgs/{orgID}/sessions/{sessionID}/mark-shipped.
//
// Transitions the session from `finalizing` to `ended` with
// `end_reason = "shipped"`. Any session member may call (not creator-
// only; the user shipping may not be the session creator). Behaviour:
//
//   - If the session is already `ended` with `end_reason = "shipped"`,
//     the call is idempotent and returns 200 with the existing row.
//     No event is emitted.
//   - If the session is `ended` with a different end_reason, returns
//     409 session.already_ended (details.end_reason).
//   - If the session is `active`, returns 409 session.not_finalizing —
//     the caller must POST /finalize first.
//   - Otherwise (`finalizing`): status flips to `ended`, end_reason is
//     set to `shipped`, ended_at is stamped. Any active finalize lock
//     for the session is released (released_at set, sessions pointer
//     cleared). A `session.ended` event with reason `shipped` and the
//     optional `final_branch_name` is emitted best-effort.
func (h *Handler) MarkSessionShipped(ctx context.Context, req openapi.MarkSessionShippedRequestObject) (openapi.MarkSessionShippedResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.MarkSessionShipped401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID

	verdict, err := checkSessionMembership(ctx, h.store, orgID, sessionID, acc.ID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: membership check: %w", err))
	}
	switch verdict {
	case memberNotOrgMember:
		return openapi.MarkSessionShipped403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this org",
			},
		}, nil
	case memberNotSessionMember:
		return openapi.MarkSessionShipped403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this session",
			},
		}, nil
	case memberSessionNotFound:
		return openapi.MarkSessionShipped404JSONResponse{
			NotFoundJSONResponse: openapi.NotFoundJSONResponse{
				Error:   "session.not_found",
				Message: "session not found",
			},
		}, nil
	}

	sess, err := h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.MarkSessionShipped404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: get session: %w", err))
	}

	finalBranch := ""
	if req.Body != nil {
		finalBranch = req.Body.FinalBranchName
	}

	switch sess.Status {
	case "ended", "archived":
		// Already ended — idempotent only when reason matches "shipped".
		if sess.EndReason != nil && *sess.EndReason == "shipped" {
			members, _ := h.store.ListSessionMembers(ctx, store.ListSessionMembersParams{
				OrgID:     orgID,
				SessionID: sessionID,
			})
			return openapi.MarkSessionShipped200JSONResponse(sessionToOpenAPI(sess, members)), nil
		}
		details := map[string]interface{}{}
		if sess.EndReason != nil {
			details["end_reason"] = *sess.EndReason
		}
		return openapi.MarkSessionShipped409JSONResponse(openapi.ErrorEnvelope{
			Error:   "session.already_ended",
			Message: "session has already ended with a different end_reason",
			Details: details,
		}), nil
	case "active":
		return openapi.MarkSessionShipped409JSONResponse(openapi.ErrorEnvelope{
			Error:   "session.not_finalizing",
			Message: "session must be finalizing before it can be marked shipped",
		}), nil
	case "finalizing":
		// Fall through to the transition below.
	default:
		// Defensive: any other status is a 409 with current value in details.
		return openapi.MarkSessionShipped409JSONResponse(openapi.ErrorEnvelope{
			Error:   "session.not_finalizing",
			Message: fmt.Sprintf("session is in unexpected status %q", sess.Status),
			Details: map[string]interface{}{"status": sess.Status},
		}), nil
	}

	now := h.clock.Now()
	endReason := "shipped"

	if err := h.store.UpdateSessionStatus(ctx, store.UpdateSessionStatusParams{
		OrgID:  orgID,
		ID:     sessionID,
		Status: "ended",
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: update session status: %w", err))
	}
	if err := h.store.SetSessionEndReason(ctx, store.SetSessionEndReasonParams{
		OrgID:     orgID,
		ID:        sessionID,
		EndReason: &endReason,
		EndedAt:   &now,
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: set session end reason: %w", err))
	}

	// Release any held finalize lock for the session — the run completed,
	// the lock is no longer needed. Best-effort: not having an active lock
	// is normal (the holder may have released manually), and lock release
	// failures should not undo the shipped transition.
	if existing, lockErr := h.store.GetActiveFinalizeLockForSession(ctx, sessionID); lockErr == nil {
		if relErr := h.store.ReleaseFinalizeLock(ctx, store.ReleaseFinalizeLockParams{
			ID:         existing.ID,
			ReleasedAt: now,
		}); relErr != nil {
			return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: release finalize lock on ship: %w", relErr))
		}
		// Clear the sessions pointer only if it still points at this holder
		// (mirror lock_release.go's defensive check).
		if sess.FinalizeLockedByAccountID != nil && *sess.FinalizeLockedByAccountID == existing.AcquiredByAccountID {
			if clearErr := h.store.ClearFinalizeLock(ctx, store.ClearFinalizeLockParams{
				OrgID: orgID,
				ID:    sessionID,
			}); clearErr != nil {
				return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: clear sessions pointer on ship: %w", clearErr))
			}
		}
	} else if !errors.Is(lockErr, store.ErrNotFound) {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: lookup active lock on ship: %w", lockErr))
	}

	// Reflect the updated columns onto the in-memory row before serializing.
	sess.Status = "ended"
	sess.EndReason = &endReason
	sess.EndedAt = &now
	sess.FinalizeLockedByAccountID = nil

	// Emit session.ended event (best-effort).
	type sessionEndedPayload struct {
		Reason          string `json:"reason"`
		FinalBranchName string `json:"final_branch_name,omitempty"`
	}
	payload, _ := json.Marshal(sessionEndedPayload{
		Reason:          endReason,
		FinalBranchName: finalBranch,
	})
	_, _ = h.events.Emit(ctx, orgID, sessionID, "session.ended", payload)

	members, _ := h.store.ListSessionMembers(ctx, store.ListSessionMembersParams{
		OrgID:     orgID,
		SessionID: sessionID,
	})

	return openapi.MarkSessionShipped200JSONResponse(sessionToOpenAPI(sess, members)), nil
}

// sessionToOpenAPI projects a domain store.Session + member list into the
// openapi.Session DTO. Kept package-local to avoid coupling finalize to
// sessions; mirrors sessions.sessionToOpenAPI byte-for-byte.
func sessionToOpenAPI(s store.Session, members []store.SessionMember) openapi.Session {
	ms := make([]openapi.MemberSummary, len(members))
	for i, m := range members {
		ms[i] = openapi.MemberSummary{
			AccountId: m.AccountID,
			Role:      m.Role,
			JoinedAt:  m.JoinedAt,
		}
	}
	sess := openapi.Session{
		Id:          s.ID,
		OrgId:       s.OrgID,
		Name:        s.Name,
		Goal:        s.Goal,
		Scope:       s.WritableScope,
		DefaultMode: openapi.SessionDefaultMode(s.DefaultMode),
		Status:      openapi.SessionStatus(s.Status),
		CreatedAt:   s.CreatedAt,
		Members:     ms,
	}
	if s.BaseSHA != nil {
		sess.BaseSha = *s.BaseSHA
	}
	if s.EndReason != nil {
		sess.EndReason = *s.EndReason
	}
	if s.FinalizeLockedByAccountID != nil {
		sess.FinalizeLockedByAccountId = *s.FinalizeLockedByAccountID
	}
	if s.EndedAt != nil {
		sess.EndedAt = *s.EndedAt
	}
	return sess
}
