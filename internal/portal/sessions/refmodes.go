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
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/handlerauth"
)

// UpsertRefMode upserts the collaboration mode for a specific ref in a session
// and emits a mode.changed event.
func (h *Handler) UpsertRefMode(ctx context.Context, req openapi.UpsertRefModeRequestObject) (openapi.UpsertRefModeResponseObject, error) {
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

	_, _, fail, ok := handlerauth.RequireSessionMember(ctx, h.store, orgID, sessionID)
	if !ok {
		if fail.Err != nil {
			return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: ref-modes: get session member: %w", fail.Err))
		}
		return upsertRefModeFail(fail), nil
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
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: ref-modes: get session: %w", err))
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
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: ref-modes: upsert: %w", err))
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
