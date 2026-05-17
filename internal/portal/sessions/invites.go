package sessions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

const (
	sessionInviteTTL        = 7 * 24 * time.Hour
	sessionInviteTokenBytes = 32
)

// InviteToSession implements POST /api/orgs/{orgID}/sessions/{sessionID}/invites.
// The caller must be an org member AND a session member.
func (h *Handler) InviteToSession(ctx context.Context, req openapi.InviteToSessionRequestObject) (openapi.InviteToSessionResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.InviteToSession401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID

	// Verify caller is an org member.
	if _, err := h.store.GetOrgMember(ctx, store.GetOrgMemberParams{
		OrgID:     orgID,
		AccountID: acc.ID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.InviteToSession403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this org",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: invite: get org member: %w", err)
	}

	// Verify session exists.
	if _, err := h.store.GetSession(ctx, orgID, sessionID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.InviteToSession404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: invite: get session: %w", err)
	}

	// Verify caller is a session member.
	if _, err := h.store.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: acc.ID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.InviteToSession403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this session",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: invite: get session member: %w", err)
	}

	raw, hash, err := generateSessionInviteToken()
	if err != nil {
		return nil, fmt.Errorf("sessions: invite: generate token: %w", err)
	}

	now := time.Now().UTC()
	id := ulid.Make().String()
	inviteeEmail := string(req.Body.Email)

	invite, err := h.store.InsertSessionInvite(ctx, store.InsertSessionInviteParams{
		ID:                  id,
		OrgID:               orgID,
		SessionID:           sessionID,
		InviterAccountID:    acc.ID,
		InviteeEmail:        inviteeEmail,
		TokenHash:           hash,
		CreatedAt:           now,
		ExpiresAt:           now.Add(sessionInviteTTL),
		AcceptedAt:          nil,
		AcceptedByAccountID: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("sessions: invite: insert invite: %w", err)
	}

	// Send email via the Sender. Build the accept URL.
	acceptURL := h.portalURL + "/sessions/" + sessionID + "/invites/" + invite.ID + "/accept?token=" + raw
	subject := "You're invited to a jamsesh session"
	body := "Hi,\n\n" +
		acc.DisplayName + " has invited you to join a collaborative coding session on jamsesh.\n\n" +
		"Click the link below to accept the invite:\n\n" + acceptURL +
		"\n\nThis invite expires in 7 days.\n"

	if err := h.sender.Send(ctx, inviteeEmail, subject, body); err != nil {
		return nil, fmt.Errorf("sessions: invite: send email: %w", err)
	}

	return openapi.InviteToSession201JSONResponse{
		Id:           invite.ID,
		SessionId:    invite.SessionID,
		InviteeEmail: openapi_types.Email(invite.InviteeEmail),
		ExpiresAt:    invite.ExpiresAt,
	}, nil
}

// AcceptSessionInvite implements POST /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept.
// Bearer-only: no org-role gate; the user is joining the session.
func (h *Handler) AcceptSessionInvite(ctx context.Context, req openapi.AcceptSessionInviteRequestObject) (openapi.AcceptSessionInviteResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.AcceptSessionInvite401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID

	invite, err := h.store.GetSessionInviteByID(ctx, req.InviteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.AcceptSessionInvite401JSONResponse{
				UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
					Error:   "auth.invalid_token",
					Message: "invalid token",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: accept invite: get invite: %w", err)
	}

	// Verify token hash.
	tokenHash := hashSessionInviteToken(req.Body.Token)
	if tokenHash != invite.TokenHash {
		return openapi.AcceptSessionInvite401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	// Verify not expired.
	now := time.Now().UTC()
	if now.After(invite.ExpiresAt) {
		return openapi.AcceptSessionInvite401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invite expired",
			},
		}, nil
	}

	// Verify not already accepted.
	if invite.AcceptedAt != nil {
		return openapi.AcceptSessionInvite409JSONResponse(openapi.ErrorEnvelope{
			Error:   "invite.already_accepted",
			Message: "invite already accepted",
		}), nil
	}

	// Verify invitee email matches authenticated account (case-insensitive).
	if !strings.EqualFold(invite.InviteeEmail, acc.Email) {
		return openapi.AcceptSessionInvite403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "invite is not for this account",
			},
		}, nil
	}

	// Tx: mark invite accepted + add session member.
	txErr := h.store.WithTx(ctx, func(tx store.TxStore) error {
		if err := tx.MarkSessionInviteAccepted(ctx, store.MarkSessionInviteAcceptedParams{
			ID:                  invite.ID,
			AcceptedAt:          now,
			AcceptedByAccountID: acc.ID,
		}); err != nil {
			return fmt.Errorf("mark invite accepted: %w", err)
		}

		if err := tx.AddSessionMember(ctx, store.AddSessionMemberParams{
			OrgID:     orgID,
			SessionID: sessionID,
			AccountID: acc.ID,
			Role:      "member",
			JoinedAt:  now,
		}); err != nil {
			if errors.Is(err, store.ErrUniqueViolation) {
				// Already a member — idempotent.
				return nil
			}
			return fmt.Errorf("add session member: %w", err)
		}
		return nil
	})
	if txErr != nil {
		return nil, fmt.Errorf("sessions: accept invite tx: %w", txErr)
	}

	// Fetch the updated session + members for the response.
	sess, err := h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("sessions: accept invite: get session: %w", err)
	}
	members, _ := h.store.ListSessionMembers(ctx, store.ListSessionMembersParams{
		OrgID:     orgID,
		SessionID: sessionID,
	})

	return openapi.AcceptSessionInvite200JSONResponse(sessionToOpenAPI(sess, members)), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func generateSessionInviteToken() (raw, hash string, err error) {
	b := make([]byte, sessionInviteTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("rand read: %w", err)
	}
	raw = hex.EncodeToString(b)
	hash = hashSessionInviteToken(raw)
	return raw, hash, nil
}

func hashSessionInviteToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
