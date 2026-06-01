// Package sessions implements the lifecycle REST endpoints for portal sessions:
// POST /api/orgs/{orgID}/sessions — create
// PATCH /api/orgs/{orgID}/sessions/{sessionID} — update goal/scope/mode
// POST /api/orgs/{orgID}/sessions/{sessionID}/finalize — transition to finalizing
// POST /api/orgs/{orgID}/sessions/{sessionID}/abandon — terminate session
// POST /api/orgs/{orgID}/sessions/{sessionID}/invites — invite member by email
// POST /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept — accept invite
// POST /api/orgs/{orgID}/sessions/{sessionID}/members/{accountID}/remove — remove member
package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/handlerauth"
	"jamsesh/internal/portal/prereceive"
	"jamsesh/internal/portal/senders"
	"jamsesh/internal/portal/storage"
)

// playgroundOrgID is the hard-coded org_id for the reserved playground org.
// Defined locally to avoid an import cycle. Value must match playground.ReservedOrgID.
const playgroundOrgID = "org_playground"

// sessionsStore is the minimal store interface consumed by Handler.
type sessionsStore interface {
	store.SessionStore
	store.SessionMemberStore
	store.OrgStore
	store.OrgMemberStore
	store.AccountStore
	store.PlaygroundSessionStore
	store.SessionInviteStore
	store.RefModeStore
	store.EventLogStore
	WithTx(ctx context.Context, fn func(store.TxStore) error) error
}

// Handler implements the openapi.StrictServerInterface session lifecycle methods.
type Handler struct {
	store     sessionsStore
	storage   storage.Service
	events    *events.Log
	sender    senders.Sender
	portalURL string
	clock     Clock
	// playgroundIdleTimeout, when > 0, enables activity-reset on successful
	// finalize-attempt for playground sessions. Zero disables the reset.
	playgroundIdleTimeout time.Duration
}

// New constructs a Handler with the real system clock. Production callers use
// this.
func New(s sessionsStore, stor storage.Service, log *events.Log, sender senders.Sender, portalURL string) *Handler {
	return NewWithClock(s, stor, log, sender, portalURL, realClock{})
}

// NewWithClock constructs a Handler with the supplied clock. Used by unit
// tests (fakeClock) and the e2etest-tagged binary (testclock.AdvanceableClock).
func NewWithClock(s sessionsStore, stor storage.Service, log *events.Log, sender senders.Sender, portalURL string, clock Clock) *Handler {
	return &Handler{store: s, storage: stor, events: log, sender: sender, portalURL: portalURL, clock: clock}
}

// WithPlaygroundIdleTimeout returns a copy of h with the given playground idle
// timeout wired in. Call after New/NewWithClock to enable activity-reset for
// playground finalize-attempts.
func (h *Handler) WithPlaygroundIdleTimeout(d time.Duration) *Handler {
	h2 := *h
	h2.playgroundIdleTimeout = d
	return &h2
}

// ---------------------------------------------------------------------------
// CreateSession — POST /api/orgs/{orgID}/sessions
// ---------------------------------------------------------------------------

// CreateSession inserts a session row + creator member row in a Tx, then
// creates the bare git repo on disk. On repo creation failure the session row
// is deleted (compensation, since disk ops can't be rolled back in a DB Tx).
func (h *Handler) CreateSession(ctx context.Context, req openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	orgID := req.OrgID

	// Verify org membership — caller must be a member to create a session.
	acc, _, fail, ok := handlerauth.RequireOrgMember(ctx, h.store, orgID)
	if !ok {
		if fail.Err != nil {
			return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: get org member: %w", fail.Err))
		}
		return createSessionFail(fail), nil
	}

	// Validate the writable_scope globs at the front door so malformed
	// patterns are surfaced as an immediate 400 rather than as an opaque
	// push-time failure. Empty scope means deny-all and is allowed.
	if msg, ok := prereceive.ValidateWritableScope(req.Body.Scope); !ok {
		return openapi.CreateSession400JSONResponse(openapi.ErrorEnvelope{
			Error:   "session.invalid_writable_scope",
			Message: msg,
		}), nil
	}

	now := h.clock.Now()
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
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: create transaction: %w", txErr))
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
	orgID := req.OrgID
	sessionID := req.SessionID

	// Verify caller is a session member.
	_, member, fail, ok := handlerauth.RequireSessionMember(ctx, h.store, orgID, sessionID)
	if !ok {
		if fail.Err != nil {
			return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: get session member: %w", fail.Err))
		}
		return patchSessionFail(fail), nil
	}

	// Only the session creator may patch the session.
	if member.Role != "creator" {
		return openapi.PatchSession403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "only the session creator can patch the session",
			},
		}, nil
	}

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
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: get session: %w", err))
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
		if msg, ok := prereceive.ValidateWritableScope(newScope); !ok {
			return openapi.PatchSession400JSONResponse(openapi.ErrorEnvelope{
				Error:   "session.invalid_writable_scope",
				Message: msg,
			}), nil
		}
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
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: update session: %w", err))
	}

	// Re-fetch to return consistent response.
	sess, err = h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: re-fetch session after patch: %w", err))
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
	_, fail, ok := handlerauth.RequireAccount(ctx)
	if !ok {
		return finalizeSessionFail(fail), nil
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
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: get session: %w", err))
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
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: update session status: %w", err))
	}

	sess.Status = "finalizing"

	// Emit session.finalizing event (best-effort).
	type sessionFinalizingPayload struct {
		SessionID string `json:"session_id"`
		OrgID     string `json:"org_id"`
	}
	payload, _ := json.Marshal(sessionFinalizingPayload{SessionID: sessionID, OrgID: orgID})
	_, _ = h.events.Emit(ctx, orgID, sessionID, "session.finalizing", payload)

	// Activity-reset for playground sessions (best-effort). A finalize-attempt
	// is substantive collaboration — it means at least one participant believes
	// the session is ready. Resets the idle timer so the destruction worker
	// doesn't race with the finalizing transition.
	if orgID == playgroundOrgID && h.playgroundIdleTimeout > 0 {
		now := h.clock.Now().UTC()
		if resetErr := h.store.ResetSessionIdleTimer(ctx, store.ResetSessionIdleTimerParams{
			OrgID:                     orgID,
			SessionID:                 sessionID,
			LastSubstantiveActivityAt: now,
			IdleTimeoutAt:             now.Add(h.playgroundIdleTimeout),
		}); resetErr != nil {
			slog.WarnContext(ctx, "sessions: finalize: reset idle timer failed (best-effort)",
				"org", orgID, "session", sessionID, "err", resetErr)
		}
	}

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
	orgID := req.OrgID
	sessionID := req.SessionID

	// Verify caller is a session member.
	_, member, fail, ok := handlerauth.RequireSessionMember(ctx, h.store, orgID, sessionID)
	if !ok {
		if fail.Err != nil {
			return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: get session member: %w", fail.Err))
		}
		return abandonSessionFail(fail), nil
	}

	// Creator-only check.
	if member.Role != "creator" {
		return openapi.AbandonSession403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "only the session creator can abandon the session",
			},
		}, nil
	}

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
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: get session: %w", err))
	}

	if sess.Status == "ended" || sess.Status == "archived" {
		return openapi.AbandonSession409JSONResponse(openapi.ErrorEnvelope{
			Error:   "session.already_ended",
			Message: "session has already ended",
		}), nil
	}

	now := h.clock.Now()
	endReason := "abandoned"

	// Update status and end_reason atomically.
	if err := h.store.UpdateSessionStatus(ctx, store.UpdateSessionStatusParams{
		OrgID:  orgID,
		ID:     sessionID,
		Status: "ended",
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: update session status: %w", err))
	}
	if err := h.store.SetSessionEndReason(ctx, store.SetSessionEndReasonParams{
		OrgID:     orgID,
		ID:        sessionID,
		EndReason: &endReason,
		EndedAt:   &now,
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: set session end reason: %w", err))
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

// ---------------------------------------------------------------------------
// Per-handler auth-fail wrappers
//
// Each function wraps a handlerauth.AuthFail into the operation-specific
// response type required by oapi-codegen's strict-server interface.
// ---------------------------------------------------------------------------

func createSessionFail(f handlerauth.AuthFail) openapi.CreateSessionResponseObject {
	if f.Status == 401 {
		return openapi.CreateSession401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.CreateSession403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}

func getSessionFail(f handlerauth.AuthFail) openapi.GetSessionResponseObject {
	if f.Status == 401 {
		return openapi.GetSession401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.GetSession403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}

func patchSessionFail(f handlerauth.AuthFail) openapi.PatchSessionResponseObject {
	if f.Status == 401 {
		return openapi.PatchSession401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.PatchSession403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}

func listSessionRefsFail(f handlerauth.AuthFail) openapi.ListSessionRefsResponseObject {
	if f.Status == 401 {
		return openapi.ListSessionRefs401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.ListSessionRefs403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}

func getSessionDigestFail(f handlerauth.AuthFail) openapi.GetSessionDigestResponseObject {
	if f.Status == 401 {
		return openapi.GetSessionDigest401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.GetSessionDigest403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}

func getSessionFileFail(f handlerauth.AuthFail) openapi.GetSessionFileResponseObject {
	if f.Status == 401 {
		return openapi.GetSessionFile401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.GetSessionFile403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}

func upsertRefModeFail(f handlerauth.AuthFail) openapi.UpsertRefModeResponseObject {
	if f.Status == 401 {
		return openapi.UpsertRefMode401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.UpsertRefMode403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}

func finalizeSessionFail(f handlerauth.AuthFail) openapi.FinalizeSessionResponseObject {
	if f.Status == 401 {
		return openapi.FinalizeSession401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.FinalizeSession403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}

func abandonSessionFail(f handlerauth.AuthFail) openapi.AbandonSessionResponseObject {
	if f.Status == 401 {
		return openapi.AbandonSession401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.AbandonSession403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
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
