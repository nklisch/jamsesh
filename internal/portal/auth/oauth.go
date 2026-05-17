package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	portaloauth "jamsesh/internal/portal/oauth"
	"jamsesh/internal/portal/tokens"
)

// OAuthHandler implements the StartOAuth and OauthCallback endpoints.
// It satisfies the oapi-codegen StrictServerInterface methods for those two
// operations; main.go mixes it into the shared strict handler.
type OAuthHandler struct {
	providers map[string]portaloauth.Provider
	store     store.Store
	tokensSvc tokens.Service
	portalURL string // e.g. "https://example.com"
}

// NewOAuthHandler constructs an OAuthHandler. providers is a map from
// provider name to Provider implementation. For v1 this is {"github": ...}.
func NewOAuthHandler(
	providers map[string]portaloauth.Provider,
	s store.Store,
	tokensSvc tokens.Service,
	portalURL string,
) *OAuthHandler {
	return &OAuthHandler{
		providers: providers,
		store:     s,
		tokensSvc: tokensSvc,
		portalURL: portalURL,
	}
}

// StartOAuth implements POST /api/auth/oauth/start.
//
// Generates a cryptographically random state nonce, stores it in
// oauth_state with a 5-minute TTL, builds the provider's authorization
// URL, and returns it to the caller.
func (h *OAuthHandler) StartOAuth(
	ctx context.Context,
	req openapi.StartOAuthRequestObject,
) (openapi.StartOAuthResponseObject, error) {
	providerName := req.Body.Provider
	provider, ok := h.providers[providerName]
	if !ok {
		return openapi.StartOAuth400JSONResponse{
			Error:   "oauth.unknown_provider",
			Message: fmt.Sprintf("unknown provider %q", providerName),
		}, nil
	}

	// If the provider map entry is nil, the provider is known but not
	// configured (e.g., missing client_id/secret in config).
	if provider == nil {
		return openapi.StartOAuth503JSONResponse{
			Error:   "oauth.provider_not_configured",
			Message: fmt.Sprintf("provider %q is not configured on this server", providerName),
		}, nil
	}

	nonce, err := portaloauth.GenerateNonce()
	if err != nil {
		return nil, fmt.Errorf("oauth start: %w", err)
	}

	redirectURI := h.portalURL + "/auth/oauth/callback"

	if err := portaloauth.StoreState(ctx, h.store, nonce, providerName, redirectURI); err != nil {
		return nil, fmt.Errorf("oauth start: store state: %w", err)
	}

	authorizeURL := provider.AuthorizeURL(nonce, redirectURI)
	return openapi.StartOAuth200JSONResponse{
		AuthorizeUrl: authorizeURL,
	}, nil
}

// OauthCallback implements POST /api/auth/oauth/callback.
//
// Validates the state nonce (consuming it atomically), calls
// Provider.Exchange to get an Identity, calls FindOrProvision to
// find-or-create the account+org, issues a TokenPair, and returns it.
func (h *OAuthHandler) OauthCallback(
	ctx context.Context,
	req openapi.OauthCallbackRequestObject,
) (openapi.OauthCallbackResponseObject, error) {
	providerName := req.Body.Provider
	code := req.Body.Code
	nonce := req.Body.State

	// Consume the nonce atomically — deleted on first use, preventing replay.
	stateRow, err := portaloauth.ConsumeState(ctx, h.store, nonce)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return oauthBadRequest("oauth.invalid_state", "invalid or already-used state nonce"), nil
		}
		return nil, fmt.Errorf("oauth callback: consume state: %w", err)
	}

	// Guard: expired nonce (ConsumeOAuthState returns it regardless of expiry
	// so we validate after consuming to prevent timing attacks on state).
	if time.Now().UTC().After(stateRow.ExpiresAt) {
		return oauthBadRequest("oauth.expired_state", "state nonce has expired"), nil
	}

	// Guard: provider mismatch (stored provider must match request provider).
	if stateRow.Provider != providerName {
		return oauthBadRequest("oauth.provider_mismatch", "provider does not match state"), nil
	}

	provider, ok := h.providers[providerName]
	if !ok || provider == nil {
		return oauthBadRequest("oauth.unknown_provider", fmt.Sprintf("unknown provider %q", providerName)), nil
	}

	// Exchange the authorization code for an Identity via the provider.
	//
	// Every error path inside Provider.Exchange (token-exchange non-2xx,
	// transport failures, /user lookup, /user/emails lookup, decode
	// errors) is a dep-class failure — the GitHub provider has no
	// business-error returns at this layer. Wrap so the strict-handler
	// translator (httperr.WriteFromError) emits a typed
	// `dep.oauth_provider_unavailable` envelope (503, Retry-After: 10).
	ghIdentity, err := provider.Exchange(ctx, code, stateRow.RedirectURI)
	if err != nil {
		return nil, deperr.WrapOAuthProvider(
			fmt.Errorf("oauth callback: exchange: %w", err))
	}

	// Map the provider Identity to the shared auth.Identity type used by
	// FindOrProvision.
	id := Identity{
		Provider:    ghIdentity.Provider,
		ProviderID:  ghIdentity.ProviderID,
		Email:       ghIdentity.Email,
		DisplayName: ghIdentity.DisplayName,
	}

	acc, _, err := FindOrProvision(ctx, h.store, id)
	if err != nil {
		return nil, fmt.Errorf("oauth callback: provision account: %w", err)
	}

	pair, err := h.tokensSvc.Issue(ctx, acc.ID)
	if err != nil {
		return nil, fmt.Errorf("oauth callback: issue tokens: %w", err)
	}

	return openapi.OauthCallback200JSONResponse{
		AccessToken:      pair.AccessToken,
		RefreshToken:     pair.RefreshToken,
		AccessExpiresAt:  pair.AccessExpiresAt,
		RefreshExpiresAt: pair.RefreshExpiresAt,
	}, nil
}

// --- helpers ----------------------------------------------------------------

func oauthBadRequest(code, message string) openapi.OauthCallbackResponseObject {
	return openapi.OauthCallback400JSONResponse{
		Error:   code,
		Message: message,
	}
}
