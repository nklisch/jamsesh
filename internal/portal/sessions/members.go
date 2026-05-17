package sessions

import (
	"context"
	"errors"
	"fmt"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/tokens"
)

// RemoveSessionMember implements POST /api/orgs/{orgID}/sessions/{sessionID}/members/{accountID}/remove.
// Creator-only: the caller must be the session creator.
func (h *Handler) RemoveSessionMember(ctx context.Context, req openapi.RemoveSessionMemberRequestObject) (openapi.RemoveSessionMemberResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.RemoveSessionMember401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID
	targetAccountID := req.AccountID

	// Verify session exists.
	if _, err := h.store.GetSession(ctx, orgID, sessionID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.RemoveSessionMember404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: remove member: get session: %w", err))
	}

	// Verify caller is the session creator.
	member, err := h.store.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: acc.ID,
	})
	if err != nil || member.Role != "creator" {
		return openapi.RemoveSessionMember403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "only the session creator can remove members",
			},
		}, nil
	}

	// Prevent the creator from removing themselves.
	if targetAccountID == acc.ID {
		return openapi.RemoveSessionMember403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "session.creator_cannot_self_remove",
				Message: "the session creator cannot remove themselves",
			},
		}, nil
	}

	// Verify target is actually a member.
	if _, err := h.store.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: targetAccountID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.RemoveSessionMember404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "member.not_found",
					Message: "account is not a member of this session",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: remove member: get target session member: %w", err))
	}

	if err := h.store.RemoveSessionMember(ctx, store.RemoveSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: targetAccountID,
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: remove member: %w", err))
	}

	return openapi.RemoveSessionMember204Response{}, nil
}
