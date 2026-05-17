// Package sessions implements the lifecycle REST endpoints for portal sessions:
// POST /api/orgs/{orgID}/sessions — create
// PATCH /api/orgs/{orgID}/sessions/{sessionID} — update goal/scope/mode
// POST /api/orgs/{orgID}/sessions/{sessionID}/finalize — transition to finalizing
// POST /api/orgs/{orgID}/sessions/{sessionID}/abandon — terminate session
package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// Handler implements the openapi.StrictServerInterface session lifecycle methods.
type Handler struct {
	store   store.Store
	storage storage.Service
	events  *events.Log
}

// New constructs a Handler.
func New(s store.Store, stor storage.Service, log *events.Log) *Handler {
	return &Handler{store: s, storage: stor, events: log}
}

// ---------------------------------------------------------------------------
// CreateSession — POST /api/orgs/{orgID}/sessions
// ---------------------------------------------------------------------------

// CreateSession inserts a session row + creator member row in a Tx, then
// creates the bare git repo on disk. On repo creation failure the session row
// is deleted (compensation, since disk ops can't be rolled back in a DB Tx).
func (h *Handler) CreateSession(ctx context.Context, req openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.CreateSession401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID

	// Verify org membership — caller must be a member to create a session.
	_, err := h.store.GetOrgMember(ctx, store.GetOrgMemberParams{
		OrgID:     orgID,
		AccountID: acc.ID,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.CreateSession403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this org",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: get org member: %w", err)
	}

	now := time.Now().UTC()
	sessionID := ulid.Make().String()

	var sess store.Session
	txErr := h.store.WithTx(ctx, func(tx store.TxStore) error {
		var err error
		sess, err = tx.CreateSession(ctx, store.CreateSessionParams{
			ID:            sessionID,
			OrgID:         orgID,
			Name:          req.Body.Name,
			Goal:          req.Body.Goal,
			WritableScope: req.Body.Scope,
			DefaultMode:   string(req.Body.DefaultMode),
			Status:        "active",
			CreatedAt:     now,
		})
		if err != nil {
			return fmt.Errorf("insert session: %w", err)
		}

		if err := tx.AddSessionMember(ctx, store.AddSessionMemberParams{
			OrgID:     orgID,
			SessionID: sessionID,
			AccountID: acc.ID,
			Role:      "creator",
			JoinedAt:  now,
		}); err != nil {
			return fmt.Errorf("insert session member: %w", err)
		}

		return nil
	})
	if txErr != nil {
		return nil, fmt.Errorf("sessions: create transaction: %w", txErr)
	}

	// Create the bare git repo AFTER the Tx commits. On failure, delete the
	// session row as compensation (the "momentary inconsistency" window is
	// acceptable — a reconciliation sweep handles process-crash cases).
	if repoErr := h.storage.CreateRepo(ctx, orgID, sessionID); repoErr != nil {
		_ = h.store.DeleteSession(ctx, store.DeleteSessionParams{OrgID: orgID, ID: sessionID})
		return nil, fmt.Errorf("sessions: create repo: %w", repoErr)
	}

	// Emit session.created event (best-effort; failure does not abort the response).
	type sessionCreatedPayload struct {
		SessionID string `json:"session_id"`
		OrgID     string `json:"org_id"`
		Name      string `json:"name"`
		CreatorID string `json:"creator_id"`
	}
	payload, _ := json.Marshal(sessionCreatedPayload{
		SessionID: sessionID,
		OrgID:     orgID,
		Name:      sess.Name,
		CreatorID: acc.ID,
	})
	_, _ = h.events.Emit(ctx, orgID, sessionID, "session.created", payload)

	// Fetch the creator member for the response.
	members, _ := h.store.ListSessionMembers(ctx, store.ListSessionMembersParams{
		OrgID:     orgID,
		SessionID: sessionID,
	})

	return openapi.CreateSession201JSONResponse(sessionToOpenAPI(sess, members)), nil
}

// ---------------------------------------------------------------------------
// PatchSession — PATCH /api/orgs/{orgID}/sessions/{sessionID}
// ---------------------------------------------------------------------------

func (h *Handler) PatchSession(ctx context.Context, req openapi.PatchSessionRequestObject) (openapi.PatchSessionResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.PatchSession401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID

	sess, err := h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.PatchSession404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: get session: %w", err)
	}

	// Verify caller is the session creator.
	member, err := h.store.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: acc.ID,
	})
	if err != nil || member.Role != "creator" {
		return openapi.PatchSession403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "only the session creator can patch the session",
			},
		}, nil
	}

	// Apply patch fields — use zero-value sentinel for "not provided" since
	// oapi-codegen emits optional fields as value types with omitempty.
	goal := sess.Goal
	scope := sess.WritableScope
	mode := sess.DefaultMode

	if req.Body.Goal != "" {
		goal = req.Body.Goal
	}
	if req.Body.Scope != "" {
		newScope := req.Body.Scope
		if isScopeNarrowing(scope, newScope) {
			return openapi.PatchSession400JSONResponse(openapi.ErrorEnvelope{
				Error:   "session.scope_narrowing_rejected",
				Message: "scope may only be widened, not narrowed",
			}), nil
		}
		scope = newScope
	}
	if req.Body.DefaultMode != "" {
		mode = string(req.Body.DefaultMode)
	}

	if err := h.store.UpdateSessionGoalScopeMode(ctx, store.UpdateSessionGoalScopeModeParams{
		OrgID:         orgID,
		ID:            sessionID,
		Goal:          goal,
		WritableScope: scope,
		DefaultMode:   mode,
	}); err != nil {
		return nil, fmt.Errorf("sessions: update session: %w", err)
	}

	// Re-fetch to return consistent response.
	sess, err = h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sessions: re-fetch session after patch: %w", err)
	}

	members, _ := h.store.ListSessionMembers(ctx, store.ListSessionMembersParams{
		OrgID:     orgID,
		SessionID: sessionID,
	})

	return openapi.PatchSession200JSONResponse(sessionToOpenAPI(sess, members)), nil
}

// ---------------------------------------------------------------------------
// FinalizeSession — POST /api/orgs/{orgID}/sessions/{sessionID}/finalize
// ---------------------------------------------------------------------------

func (h *Handler) FinalizeSession(ctx context.Context, req openapi.FinalizeSessionRequestObject) (openapi.FinalizeSessionResponseObject, error) {
	_, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.FinalizeSession401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID

	sess, err := h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.FinalizeSession404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: get session: %w", err)
	}

	switch sess.Status {
	case "finalizing":
		// Already finalizing — idempotent no-op.
		members, _ := h.store.ListSessionMembers(ctx, store.ListSessionMembersParams{
			OrgID:     orgID,
			SessionID: sessionID,
		})
		return openapi.FinalizeSession200JSONResponse(sessionToOpenAPI(sess, members)), nil
	case "ended", "archived":
		return openapi.FinalizeSession409JSONResponse(openapi.ErrorEnvelope{
			Error:   "session.already_ended",
			Message: "session has already ended",
		}), nil
	}

	// Transition active → finalizing.
	if err := h.store.UpdateSessionStatus(ctx, store.UpdateSessionStatusParams{
		OrgID:  orgID,
		ID:     sessionID,
		Status: "finalizing",
	}); err != nil {
		return nil, fmt.Errorf("sessions: update session status: %w", err)
	}

	sess.Status = "finalizing"

	// Emit session.finalizing event (best-effort).
	type sessionFinalizingPayload struct {
		SessionID string `json:"session_id"`
		OrgID     string `json:"org_id"`
	}
	payload, _ := json.Marshal(sessionFinalizingPayload{SessionID: sessionID, OrgID: orgID})
	_, _ = h.events.Emit(ctx, orgID, sessionID, "session.finalizing", payload)

	members, _ := h.store.ListSessionMembers(ctx, store.ListSessionMembersParams{
		OrgID:     orgID,
		SessionID: sessionID,
	})

	return openapi.FinalizeSession200JSONResponse(sessionToOpenAPI(sess, members)), nil
}

// ---------------------------------------------------------------------------
// AbandonSession — POST /api/orgs/{orgID}/sessions/{sessionID}/abandon
// ---------------------------------------------------------------------------

func (h *Handler) AbandonSession(ctx context.Context, req openapi.AbandonSessionRequestObject) (openapi.AbandonSessionResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.AbandonSession401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID

	sess, err := h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.AbandonSession404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: get session: %w", err)
	}

	// Creator-only check.
	member, err := h.store.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: acc.ID,
	})
	if err != nil || member.Role != "creator" {
		return openapi.AbandonSession403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "only the session creator can abandon the session",
			},
		}, nil
	}

	if sess.Status == "ended" || sess.Status == "archived" {
		return openapi.AbandonSession409JSONResponse(openapi.ErrorEnvelope{
			Error:   "session.already_ended",
			Message: "session has already ended",
		}), nil
	}

	now := time.Now().UTC()
	endReason := "abandoned"

	// Update status and end_reason atomically.
	if err := h.store.UpdateSessionStatus(ctx, store.UpdateSessionStatusParams{
		OrgID:  orgID,
		ID:     sessionID,
		Status: "ended",
	}); err != nil {
		return nil, fmt.Errorf("sessions: update session status: %w", err)
	}
	if err := h.store.SetSessionEndReason(ctx, store.SetSessionEndReasonParams{
		OrgID:     orgID,
		ID:        sessionID,
		EndReason: &endReason,
		EndedAt:   &now,
	}); err != nil {
		return nil, fmt.Errorf("sessions: set session end reason: %w", err)
	}

	sess.Status = "ended"
	sess.EndReason = &endReason
	sess.EndedAt = &now

	// Emit session.ended event (best-effort).
	type sessionEndedPayload struct {
		SessionID string `json:"session_id"`
		OrgID     string `json:"org_id"`
		EndReason string `json:"end_reason"`
	}
	payload, _ := json.Marshal(sessionEndedPayload{SessionID: sessionID, OrgID: orgID, EndReason: endReason})
	_, _ = h.events.Emit(ctx, orgID, sessionID, "session.ended", payload)

	members, _ := h.store.ListSessionMembers(ctx, store.ListSessionMembersParams{
		OrgID:     orgID,
		SessionID: sessionID,
	})

	return openapi.AbandonSession200JSONResponse(sessionToOpenAPI(sess, members)), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isScopeNarrowing returns true if any glob in oldScope is absent from newScope.
// This enforces the strict append-only rule: new scope must be a superset of old scope.
func isScopeNarrowing(oldScope, newScope string) bool {
	var oldGlobs, newGlobs []string
	if err := json.Unmarshal([]byte(oldScope), &oldGlobs); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(newScope), &newGlobs); err != nil {
		return true // can't parse new scope → treat as narrowing
	}
	newSet := make(map[string]bool, len(newGlobs))
	for _, g := range newGlobs {
		newSet[g] = true
	}
	for _, g := range oldGlobs {
		if !newSet[g] {
			return true
		}
	}
	return false
}

// sessionToOpenAPI maps a store.Session + member list to the openapi.Session type.
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
