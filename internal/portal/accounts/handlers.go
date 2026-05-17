// Package accounts implements the /api/me and /api/orgs endpoints.
package accounts

import (
	"context"
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/auth"
	"jamsesh/internal/portal/tokens"
)

// Handler implements the openapi.StrictServerInterface methods for the
// accounts endpoints: GET /api/me and POST /api/orgs.
type Handler struct {
	store store.Store
}

// New returns a Handler backed by s.
func New(s store.Store) *Handler {
	return &Handler{store: s}
}

// GetMe implements GET /api/me.
// It requires BearerMiddleware upstream to have placed the *store.Account in
// the request context.
func (h *Handler) GetMe(ctx context.Context, _ openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.GetMe401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgs, err := h.store.ListOrgsForAccount(ctx, acc.ID)
	if err != nil {
		return nil, fmt.Errorf("accounts: list orgs for account %s: %w", acc.ID, err)
	}

	// Build memberships by loading each org_member row.
	memberships := make([]openapi.MeOrgMembership, 0, len(orgs))
	for _, org := range orgs {
		m, err := h.store.GetOrgMember(ctx, store.GetOrgMemberParams{
			OrgID:     org.ID,
			AccountID: acc.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("accounts: get org member (org=%s account=%s): %w", org.ID, acc.ID, err)
		}
		memberships = append(memberships, openapi.MeOrgMembership{
			Id:   org.ID,
			Name: org.Name,
			Slug: org.Slug,
			Role: m.Role,
		})
	}

	return openapi.GetMe200JSONResponse{
		Id:          acc.ID,
		Email:       openapi_types.Email(acc.Email),
		DisplayName: acc.DisplayName,
		Orgs:        memberships,
	}, nil
}

// CreateOrg implements POST /api/orgs.
// It requires BearerMiddleware upstream to have placed the *store.Account in
// the request context. The authenticated account becomes the creator of the
// new org.
func (h *Handler) CreateOrg(ctx context.Context, req openapi.CreateOrgRequestObject) (openapi.CreateOrgResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.CreateOrg401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	now := time.Now().UTC()
	org, err := auth.CreateOrgWithSlug(ctx, h.store, req.Body.Name, now)
	if err != nil {
		return nil, fmt.Errorf("accounts: create org: %w", err)
	}

	if err := h.store.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID:     org.ID,
		AccountID: acc.ID,
		Role:      "creator",
		CreatedAt: now,
	}); err != nil {
		return nil, fmt.Errorf("accounts: add org member: %w", err)
	}

	return openapi.CreateOrg201JSONResponse{
		Id:   org.ID,
		Name: org.Name,
		Slug: org.Slug,
	}, nil
}
