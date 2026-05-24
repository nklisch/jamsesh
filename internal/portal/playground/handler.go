// Package playground implements the REST endpoints for the ephemeral anonymous
// playground subsystem:
//
//	POST /api/playground/sessions          — CreatePlaygroundSession
//	POST /api/playground/sessions/{id}/join — JoinPlaygroundSession
//	GET  /api/playground/sessions/{id}     — GetPlaygroundSession
//	GET  /api/playground/sessions/{id}/tombstone — GetPlaygroundTombstone
package playground

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/handlerauth"
	"jamsesh/internal/portal/playground/wordlist"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// Config holds the playground-specific tuning knobs threaded from the main
// portal config. It is populated once at startup and treated as immutable.
type Config struct {
	Enabled         bool
	IdleTimeout     time.Duration
	HardCap         time.Duration
	MaxParticipants int
}

// Handler implements the openapi.StrictServerInterface playground methods.
// It is constructed by main.go and composed into combinedHandler.
type Handler struct {
	Store   store.Store
	Tokens  tokens.Service
	Storage storage.Service
	Cfg     Config
	Clock   Clock
	Logger  *slog.Logger
}

// ---------------------------------------------------------------------------
// CreatePlaygroundSession — POST /api/playground/sessions
// ---------------------------------------------------------------------------

// CreatePlaygroundSession creates an ephemeral anonymous playground session.
// No authentication is required. The server assigns a pronounceable handle to
// the creator and issues an anonymous bearer token scoped to the session.
func (h *Handler) CreatePlaygroundSession(ctx context.Context, req openapi.CreatePlaygroundSessionRequestObject) (openapi.CreatePlaygroundSessionResponseObject, error) {
	if !h.Cfg.Enabled {
		return openapi.CreatePlaygroundSession503JSONResponse(openapi.ErrorEnvelope{
			Error:   "playground.disabled",
			Message: "playground sessions are not enabled on this server",
		}), nil
	}

	now := h.Clock.Now().UTC()
	sessionID := ulid.Make().String()

	// Pick a unique handle for the creator before the TX so collisions can be
	// resolved without a nested query inside the transaction.
	nickname := h.uniqueHandle(ctx, sessionID)

	// Apply defaults for optional body fields.
	var body openapi.CreatePlaygroundSessionRequest
	if req.Body != nil {
		body = *req.Body
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		// Use the first 8 chars of the session ID as the short suffix.
		shortID := sessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		name = "playground-" + strings.ToLower(shortID)
	}
	goal := body.Goal
	scope := strings.TrimSpace(body.Scope)
	if scope == "" {
		scope = `["**"]`
	}

	hardCapAt := now.Add(h.Cfg.HardCap)
	idleTimeoutAt := now.Add(h.Cfg.IdleTimeout)

	// Step 1: insert the session row in a TX. The member row is NOT added here
	// because we don't yet know the accountID — IssueAnonymousSessionBearer
	// creates the anon account. We commit the session row first, then issue the
	// bearer + add the member row in separate calls. This avoids a SQLite
	// deadlock (IssueAnonymousSessionBearer opens its own TX on the same pool,
	// which would block on the outer TX's write lock under SQLite).
	var sess store.Session
	txErr := h.Store.WithTx(ctx, func(tx store.TxStore) error {
		var err error
		sess, err = tx.CreateSession(ctx, store.CreateSessionParams{
			ID:                        sessionID,
			OrgID:                     ReservedOrgID,
			Name:                      name,
			Goal:                      goal,
			WritableScope:             scope,
			DefaultMode:               "sync",
			Status:                    "active",
			CreatedAt:                 now,
			LastSubstantiveActivityAt: &now,
			HardCapAt:                 &hardCapAt,
			IdleTimeoutAt:             &idleTimeoutAt,
		})
		if err != nil {
			return fmt.Errorf("insert playground session: %w", err)
		}
		return nil
	})
	if txErr != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: create session tx: %w", txErr))
	}

	// Step 2: issue the bearer (creates anon account + token in its own TX).
	// If this fails, the session row remains without a creator member; the
	// destruction sweep will clean it up within the next sweep interval.
	rawToken, accountID, expiresAt, err := h.Tokens.IssueAnonymousSessionBearer(ctx, sessionID, nickname, h.Cfg.HardCap)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: issue anonymous bearer: %w", err))
	}

	// Step 3: add the creator member row.
	if err := h.Store.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID:     ReservedOrgID,
		SessionID: sessionID,
		AccountID: accountID,
		Role:      "creator",
		JoinedAt:  now,
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: add session member: %w", err))
	}

	resp := openapi.PlaygroundSessionCreated{
		Session:   sessionToAPISummary(sess, 1, hardCapAt, idleTimeoutAt),
		Bearer:    rawToken,
		ExpiresAt: expiresAt,
		Nickname:  nickname,
	}

	// Create the bare git repo AFTER the TX commits (disk ops cannot be rolled
	// back in a DB TX). On failure, log and return error; the destruction sweep
	// will clean the orphaned session row within the next sweep interval.
	if repoErr := h.Storage.CreateRepo(ctx, ReservedOrgID, sessionID); repoErr != nil {
		h.Logger.Error("playground: bare-repo create failed after session insert",
			"session_id", sessionID, "err", repoErr)
		return nil, fmt.Errorf("playground: create repo: %w", repoErr)
	}

	return openapi.CreatePlaygroundSession201JSONResponse(resp), nil
}

// ---------------------------------------------------------------------------
// JoinPlaygroundSession — POST /api/playground/sessions/{id}/join
// ---------------------------------------------------------------------------

// JoinPlaygroundSession adds a new anonymous participant to an existing
// playground session. No authentication is required. The server assigns a
// pronounceable handle (client may suggest one) and issues a fresh bearer.
func (h *Handler) JoinPlaygroundSession(ctx context.Context, req openapi.JoinPlaygroundSessionRequestObject) (openapi.JoinPlaygroundSessionResponseObject, error) {
	if !h.Cfg.Enabled {
		return openapi.JoinPlaygroundSession503JSONResponse(openapi.ErrorEnvelope{
			Error:   "playground.disabled",
			Message: "playground sessions are not enabled on this server",
		}), nil
	}

	sessionID := req.Id

	sess, err := h.Store.GetSession(ctx, ReservedOrgID, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.JoinPlaygroundSession404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: get session: %w", err))
	}

	// Hard-cap check: if the session has expired, reject with 410.
	if sess.HardCapAt != nil && !h.Clock.Now().UTC().Before(*sess.HardCapAt) {
		return openapi.JoinPlaygroundSession410JSONResponse(openapi.ErrorEnvelope{
			Error:   "playground.session_ended",
			Message: "this session has ended (hard-cap elapsed)",
		}), nil
	}

	// Status check: non-active sessions are gone.
	if sess.Status != "active" {
		return openapi.JoinPlaygroundSession410JSONResponse(openapi.ErrorEnvelope{
			Error:   "playground.session_ended",
			Message: "this session has ended",
		}), nil
	}

	// Capacity check.
	count, err := h.Store.CountSessionMembers(ctx, store.CountSessionMembersParams{
		OrgID:     ReservedOrgID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: count session members: %w", err))
	}
	if int(count) >= h.Cfg.MaxParticipants {
		return openapi.JoinPlaygroundSession409JSONResponse(openapi.ErrorEnvelope{
			Error:   "playground.session_full",
			Message: fmt.Sprintf("session is full (%d/%d participants)", count, h.Cfg.MaxParticipants),
		}), nil
	}

	// Determine nickname: use client suggestion if provided, else pick one.
	var candidates []string
	if req.Body != nil && strings.TrimSpace(req.Body.Nickname) != "" {
		candidates = []string{strings.TrimSpace(req.Body.Nickname)}
	}
	nickname := h.uniqueHandle(ctx, sessionID, candidates...)

	// TTL is the remaining hard-cap window.
	var ttl time.Duration
	if sess.HardCapAt != nil {
		ttl = time.Until(*sess.HardCapAt)
		if ttl <= 0 {
			return openapi.JoinPlaygroundSession410JSONResponse(openapi.ErrorEnvelope{
				Error:   "playground.session_ended",
				Message: "this session has ended",
			}), nil
		}
	} else {
		ttl = h.Cfg.HardCap
	}

	rawToken, accountID, expiresAt, err := h.Tokens.IssueAnonymousSessionBearer(ctx, sessionID, nickname, ttl)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: issue bearer for joiner: %w", err))
	}

	now := h.Clock.Now().UTC()
	if err := h.Store.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID:     ReservedOrgID,
		SessionID: sessionID,
		AccountID: accountID,
		Role:      "member",
		JoinedAt:  now,
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: add session member: %w", err))
	}

	// Compute the hard_cap_at and idle_timeout_at for the summary.
	hardCapAt := now.Add(h.Cfg.HardCap)
	if sess.HardCapAt != nil {
		hardCapAt = *sess.HardCapAt
	}
	idleTimeoutAt := now.Add(h.Cfg.IdleTimeout)
	if sess.IdleTimeoutAt != nil {
		idleTimeoutAt = *sess.IdleTimeoutAt
	}

	return openapi.JoinPlaygroundSession200JSONResponse(openapi.PlaygroundJoinResult{
		Session:   sessionToAPISummary(sess, int(count)+1, hardCapAt, idleTimeoutAt),
		Bearer:    rawToken,
		ExpiresAt: expiresAt,
		Nickname:  nickname,
	}), nil
}

// ---------------------------------------------------------------------------
// GetPlaygroundSession — GET /api/playground/sessions/{id}
// ---------------------------------------------------------------------------

// GetPlaygroundSession returns the compact session descriptor for a playground
// session. The caller must present a valid anonymous bearer for this session
// (i.e. be a session member).
func (h *Handler) GetPlaygroundSession(ctx context.Context, req openapi.GetPlaygroundSessionRequestObject) (openapi.GetPlaygroundSessionResponseObject, error) {
	sessionID := req.Id

	// Require a valid bearer — the bearer must belong to a session member.
	acc, fail, ok := handlerauth.RequireAccount(ctx)
	if !ok {
		if fail.Err != nil {
			return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: get session auth: %w", fail.Err))
		}
		return openapi.GetPlaygroundSession401JSONResponse{
			UnauthorizedJSONResponse: fail.Unauthorized,
		}, nil
	}

	sess, err := h.Store.GetSession(ctx, ReservedOrgID, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.GetPlaygroundSession404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: get session: %w", err))
	}

	// Verify the bearer's account is a member of this session.
	_, err = h.Store.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     ReservedOrgID,
		SessionID: sessionID,
		AccountID: acc.ID,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.GetPlaygroundSession401JSONResponse{
				UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
					Error:   "auth.not_a_member",
					Message: "your bearer token is not associated with this session",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: get session member: %w", err))
	}

	count, err := h.Store.CountSessionMembers(ctx, store.CountSessionMembersParams{
		OrgID:     ReservedOrgID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: count session members: %w", err))
	}

	now := h.Clock.Now().UTC()
	hardCapAt := now.Add(h.Cfg.HardCap)
	if sess.HardCapAt != nil {
		hardCapAt = *sess.HardCapAt
	}
	idleTimeoutAt := now.Add(h.Cfg.IdleTimeout)
	if sess.IdleTimeoutAt != nil {
		idleTimeoutAt = *sess.IdleTimeoutAt
	}

	return openapi.GetPlaygroundSession200JSONResponse(
		sessionToAPISummary(sess, int(count), hardCapAt, idleTimeoutAt),
	), nil
}

// ---------------------------------------------------------------------------
// GetPlaygroundTombstone — GET /api/playground/sessions/{id}/tombstone
// ---------------------------------------------------------------------------

// GetPlaygroundTombstone returns the destruction summary for a destroyed
// playground session. Returns 404 while the session is still active, and 404
// once the tombstone's own TTL has elapsed.
func (h *Handler) GetPlaygroundTombstone(ctx context.Context, req openapi.GetPlaygroundTombstoneRequestObject) (openapi.GetPlaygroundTombstoneResponseObject, error) {
	sessionID := req.Id

	tombstone, err := h.Store.GetTombstone(ctx, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.GetPlaygroundTombstone404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "no tombstone found for this session ID (session may still be active or tombstone may have expired)",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: get tombstone: %w", err))
	}

	// Tombstone TTL check: if the tombstone has expired, treat as 404.
	if !h.Clock.Now().UTC().Before(tombstone.ExpiresAt) {
		return openapi.GetPlaygroundTombstone404JSONResponse{
			NotFoundJSONResponse: openapi.NotFoundJSONResponse{
				Error:   "session.not_found",
				Message: "tombstone has expired",
			},
		}, nil
	}

	return openapi.GetPlaygroundTombstone200JSONResponse(tombstoneToAPI(tombstone)), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// uniqueHandle picks a pronounceable handle and retries up to 10 times when
// it collides with an existing nickname in the session. If candidates are
// provided, they are tried first (in order) before falling back to fresh
// wordlist picks. If all 10 attempts are exhausted, a random-suffix handle
// is returned as a last resort.
func (h *Handler) uniqueHandle(ctx context.Context, sessionID string, candidates ...string) string {
	tried := make(map[string]bool, 16)
	for i := 0; i < 10; i++ {
		var nick string
		if i < len(candidates) {
			nick = candidates[i]
		} else {
			nick = wordlist.Pick()
		}
		if tried[nick] {
			continue
		}
		tried[nick] = true
		taken, _ := h.Store.NicknameTakenInSession(ctx, store.NicknameTakenInSessionParams{
			OrgID:       ReservedOrgID,
			SessionID:   sessionID,
			DisplayName: nick,
		})
		if !taken {
			return nick
		}
	}
	// Last resort: fresh wordlist pick + random 4-byte hex suffix.
	return wordlist.Pick() + "-" + randHex(4)
}

// randHex returns n random bytes encoded as a lowercase hex string.
func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is a fatal system error; panic is appropriate.
		panic(fmt.Sprintf("playground: rand.Read: %v", err))
	}
	return hex.EncodeToString(b)
}

// sessionToAPISummary maps a store.Session to the compact openapi.PlaygroundSessionSummary.
// membersCount is passed explicitly because the session row does not carry it;
// hardCapAt and idleTimeoutAt are passed explicitly to handle nil pointer
// fields (nullable in the store).
func sessionToAPISummary(s store.Session, membersCount int, hardCapAt, idleTimeoutAt time.Time) openapi.PlaygroundSessionSummary {
	return openapi.PlaygroundSessionSummary{
		Id:            s.ID,
		OrgId:         s.OrgID,
		Name:          s.Name,
		Goal:          s.Goal,
		Scope:         s.WritableScope,
		Status:        openapi.PlaygroundSessionSummaryStatus(s.Status),
		CreatedAt:     s.CreatedAt,
		HardCapAt:     hardCapAt,
		IdleTimeoutAt: idleTimeoutAt,
		MembersCount:  membersCount,
	}
}

// tombstoneToAPI maps a store.Tombstone to the openapi.PlaygroundTombstone.
func tombstoneToAPI(t store.Tombstone) openapi.PlaygroundTombstone {
	return openapi.PlaygroundTombstone{
		SessionId:       t.SessionID,
		OrgId:           t.OrgID,
		MembersCount:    int(t.MembersCount),
		CommitsCount:    int(t.CommitsCount),
		AutoMergesCount: int(t.AutoMergesCount),
		DurationSeconds: int(t.DurationSeconds),
		EndReason:       openapi.PlaygroundTombstoneEndReason(t.EndReason),
		EndedAt:         t.EndedAt,
		ExpiresAt:       t.ExpiresAt,
	}
}
