// Package sessions — ref-modes endpoint.
// POST /api/orgs/{orgID}/sessions/{sessionID}/ref-modes
package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

// UpsertRefMode upserts the collaboration mode for a specific ref in a session
// and emits a mode.changed event.
func (h *Handler) UpsertRefMode(ctx context.Context, req openapi.UpsertRefModeRequestObject) (openapi.UpsertRefModeResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.UpsertRefMode401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	if req.Body == nil {
		return openapi.UpsertRefMode400JSONResponse(openapi.ErrorEnvelope{
			Error:   "request.invalid",
			Message: "request body is required",
		}), nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID
	ref := req.Body.Ref
	mode := string(req.Body.Mode)

	// Require org membership.
	if _, err := h.store.GetOrgMember(ctx, store.GetOrgMemberParams{
		OrgID:     orgID,
		AccountID: acc.ID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.UpsertRefMode403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this org",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: ref-modes: get org member: %w", err)
	}

	// Require session membership.
	if _, err := h.store.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: acc.ID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.UpsertRefMode403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this session",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: ref-modes: get session member: %w", err)
	}

	// Verify session exists.
	if _, err := h.store.GetSession(ctx, orgID, sessionID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.UpsertRefMode404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: ref-modes: get session: %w", err)
	}

	// Fetch the old mode for the event payload (best-effort).
	oldMode := ""
	existing, err := h.store.GetRefMode(ctx, store.GetRefModeParams{
		SessionID: sessionID,
		Ref:       ref,
	})
	if err == nil {
		oldMode = existing.Mode
	}

	// Upsert the ref mode.
	if err := h.store.UpsertRefMode(ctx, store.UpsertRefModeParams{
		SessionID: sessionID,
		Ref:       ref,
		Mode:      mode,
	}); err != nil {
		return nil, fmt.Errorf("sessions: ref-modes: upsert: %w", err)
	}

	// Emit mode.changed event (best-effort; failure does not abort the response).
	type modeChangedPayload struct {
		Ref     string `json:"ref"`
		OldMode string `json:"old_mode"`
		NewMode string `json:"new_mode"`
	}
	payload, _ := json.Marshal(modeChangedPayload{
		Ref:     ref,
		OldMode: oldMode,
		NewMode: mode,
	})
	_, _ = h.events.Emit(ctx, orgID, sessionID, "mode.changed", payload)

	return openapi.UpsertRefMode204Response{}, nil
}
