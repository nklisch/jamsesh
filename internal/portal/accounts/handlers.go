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
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/handlerauth"
	"jamsesh/internal/portal/senders"
)

// Clock is an injectable time source. The default realClock calls
// time.Now().UTC(); tests inject a fakeClock to simulate org-invite
// expiry. Shape mirrors internal/portal/auth.Clock and
// internal/portal/tokens.Clock so a single AdvanceableClock instance
// satisfies all three.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// accountsStore is the minimal store interface consumed by Handler.
type accountsStore interface {
	store.OrgStore
	store.OrgMemberStore
	store.OrgInviteStore
	WithTx(ctx context.Context, fn func(store.TxStore) error) error
}

// Handler implements the openapi.StrictServerInterface methods for the
// accounts endpoints: GET /api/me, POST /api/orgs, GET/POST /api/orgs/{orgID}/...
type Handler struct {
	store     accountsStore
	sender    senders.Sender
	portalURL string
	clock     Clock
}

// New returns a Handler backed by s with the real system clock.
// sender and portalURL are required for CreateOrgInvite to send invite emails.
func New(s accountsStore, sender senders.Sender, portalURL string) *Handler {
	return NewWithClock(s, sender, portalURL, realClock{})
}

// NewWithClock returns a Handler backed by s with the supplied clock.
// Used by unit tests (fakeClock) and the e2etest-tagged binary
// (testclock.AdvanceableClock).
func NewWithClock(s accountsStore, sender senders.Sender, portalURL string, clock Clock) *Handler {
	return &Handler{store: s, sender: sender, portalURL: portalURL, clock: clock}
}

// GetMe implements GET /api/me.
// It requires BearerMiddleware upstream to have placed the *store.Account in
// the request context.
func (h *Handler) GetMe(ctx context.Context, _ openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	acc, fail, ok := handlerauth.RequireAccount(ctx)
	if !ok {
		return getMeFail(fail), nil
	}

	orgs, err := h.store.ListOrgsForAccount(ctx, acc.ID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("accounts: list orgs for account %s: %w", acc.ID, err))
	}

	// Build memberships by loading each org_member row.
	// Note: GetOrgMember here is NOT an auth guard — it loads role data for the
	// response body. It is intentionally not migrated through handlerauth.
	memberships := make([]openapi.MeOrgMembership, 0, len(orgs))
	for _, org := range orgs {
		m, err := h.store.GetOrgMember(ctx, store.GetOrgMemberParams{
			OrgID:     org.ID,
			AccountID: acc.ID,
		})
		if err != nil {
			return nil, deperr.WrapDBIfTransient(fmt.Errorf("accounts: get org member (org=%s account=%s): %w", org.ID, acc.ID, err))
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
	acc, fail, ok := handlerauth.RequireAccount(ctx)
	if !ok {
		return createOrgFail(fail), nil
	}

	now := h.clock.Now()
	org, err := auth.CreateOrgWithSlug(ctx, h.store, req.Body.Name, now)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("accounts: create org: %w", err))
	}

	if err := h.store.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID:     org.ID,
		AccountID: acc.ID,
		Role:      "creator",
		CreatedAt: now,
	}); err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("accounts: add org member: %w", err))
	}

	return openapi.CreateOrg201JSONResponse{
		Id:   org.ID,
		Name: org.Name,
		Slug: org.Slug,
	}, nil
}

// ---------------------------------------------------------------------------
// Per-handler auth-fail wrappers
// ---------------------------------------------------------------------------

// getMeFail wraps an AuthFail for GetMe. RequireAccount only returns 401, so
// no 403 branch is needed.
func getMeFail(f handlerauth.AuthFail) openapi.GetMeResponseObject {
	return openapi.GetMe401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
}

func createOrgFail(f handlerauth.AuthFail) openapi.CreateOrgResponseObject {
	return openapi.CreateOrg401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
}
