package accounts

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
	orgInviteTTL        = 7 * 24 * time.Hour
	orgInviteTokenBytes = 32
)

// ListOrgMembers implements GET /api/orgs/{orgID}/members.
// RequireOrgRole(creator, member) middleware must be upstream.
func (h *Handler) ListOrgMembers(ctx context.Context, req openapi.ListOrgMembersRequestObject) (openapi.ListOrgMembersResponseObject, error) {
	members, err := h.store.ListOrgMembers(ctx, req.OrgID)
	if err != nil {
		return nil, fmt.Errorf("accounts: list org members (org=%s): %w", req.OrgID, err)
	}

	refs := make(openapi.ListOrgMembers200JSONResponse, 0, len(members))
	for _, m := range members {
		joinedAt := m.CreatedAt
		refs = append(refs, openapi.MemberRef{
			AccountId:   m.AccountID,
			Email:       openapi_types.Email(m.Email),
			DisplayName: m.DisplayName,
			Role:        m.Role,
			JoinedAt:    joinedAt,
		})
	}

	return refs, nil
}

// CreateOrgInvite implements POST /api/orgs/{orgID}/invites.
// RequireOrgRole(creator) middleware must be upstream.
func (h *Handler) CreateOrgInvite(ctx context.Context, req openapi.CreateOrgInviteRequestObject) (openapi.CreateOrgInviteResponseObject, error) {
	inviter, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.CreateOrgInvite401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	raw, hash, err := generateInviteToken()
	if err != nil {
		return nil, fmt.Errorf("accounts: generate invite token: %w", err)
	}

	now := time.Now().UTC()
	id := ulid.Make().String()
	recipientEmail := string(req.Body.Email)

	invite, err := h.store.InsertOrgInvite(ctx, store.InsertOrgInviteParams{
		ID:                  id,
		OrgID:               req.OrgID,
		InviterAccountID:    inviter.ID,
		RecipientEmail:      recipientEmail,
		TokenHash:           hash,
		CreatedAt:           now,
		ExpiresAt:           now.Add(orgInviteTTL),
		AcceptedAt:          nil,
		AcceptedByAccountID: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("accounts: insert org invite: %w", err)
	}

	org, err := h.store.GetOrgByID(ctx, req.OrgID)
	if err != nil {
		return nil, fmt.Errorf("accounts: get org (org=%s): %w", req.OrgID, err)
	}

	acceptURL := h.portalURL + "/orgs/" + req.OrgID + "/invites/" + invite.ID + "/accept?token=" + raw
	subject := "You're invited to " + org.Name + " on jamsesh"
	body := "Hi,\n\n" +
		inviter.DisplayName + " has invited you to join " + org.Name + " on jamsesh.\n\n" +
		"Click the link below to accept the invite:\n\n" + acceptURL +
		"\n\nThis invite expires in 7 days.\n"

	if err := h.sender.Send(ctx, recipientEmail, subject, body); err != nil {
		return nil, fmt.Errorf("accounts: send invite email: %w", err)
	}

	return openapi.CreateOrgInvite201JSONResponse{
		Id:             invite.ID,
		RecipientEmail: openapi_types.Email(invite.RecipientEmail),
		ExpiresAt:      invite.ExpiresAt,
	}, nil
}

// AcceptOrgInvite implements POST /api/orgs/{orgID}/invites/{inviteID}/accept.
// BearerMiddleware must be upstream (no org-role gate — the user is joining).
func (h *Handler) AcceptOrgInvite(ctx context.Context, req openapi.AcceptOrgInviteRequestObject) (openapi.AcceptOrgInviteResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.AcceptOrgInvite401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	invite, err := h.store.GetOrgInviteByID(ctx, req.InviteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.AcceptOrgInvite401JSONResponse{
				UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
					Error:   "auth.invalid_token",
					Message: "invalid token",
				},
			}, nil
		}
		return nil, fmt.Errorf("accounts: get org invite (id=%s): %w", req.InviteID, err)
	}

	// Verify token hash.
	tokenHash := hashInviteToken(req.Body.Token)
	if tokenHash != invite.TokenHash {
		return openapi.AcceptOrgInvite401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	// Verify not expired.
	now := time.Now().UTC()
	if now.After(invite.ExpiresAt) {
		return openapi.AcceptOrgInvite401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invite expired",
			},
		}, nil
	}

	// Verify not already accepted.
	if invite.AcceptedAt != nil {
		return openapi.AcceptOrgInvite409JSONResponse{
			Error:   "invite.already_accepted",
			Message: "invite already accepted",
		}, nil
	}

	// Verify recipient email matches authenticated account (case-insensitive).
	if !strings.EqualFold(invite.RecipientEmail, acc.Email) {
		return openapi.AcceptOrgInvite403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "invite is not for this account",
			},
		}, nil
	}

	// Tx: mark invite accepted + add org member.
	var org store.Org
	err = h.store.WithTx(ctx, func(tx store.TxStore) error {
		if err := tx.MarkOrgInviteAccepted(ctx, store.MarkOrgInviteAcceptedParams{
			ID:                  invite.ID,
			AcceptedAt:          now,
			AcceptedByAccountID: acc.ID,
		}); err != nil {
			return fmt.Errorf("mark invite accepted: %w", err)
		}

		if err := tx.AddOrgMember(ctx, store.AddOrgMemberParams{
			OrgID:     req.OrgID,
			AccountID: acc.ID,
			Role:      "member",
			CreatedAt: now,
		}); err != nil {
			if errors.Is(err, store.ErrUniqueViolation) {
				// Account is already a member — idempotent, continue.
				return nil
			}
			return fmt.Errorf("add org member: %w", err)
		}

		var txErr error
		org, txErr = tx.GetOrgByID(ctx, req.OrgID)
		if txErr != nil {
			return fmt.Errorf("get org: %w", txErr)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("accounts: accept org invite tx: %w", err)
	}

	return openapi.AcceptOrgInvite200JSONResponse{
		Id:   org.ID,
		Name: org.Name,
		Slug: org.Slug,
	}, nil
}

// --- helpers ----------------------------------------------------------------

func generateInviteToken() (raw, hash string, err error) {
	b := make([]byte, orgInviteTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("rand read: %w", err)
	}
	raw = hex.EncodeToString(b)
	hash = hashInviteToken(raw)
	return raw, hash, nil
}

func hashInviteToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
