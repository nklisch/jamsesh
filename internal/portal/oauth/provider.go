// Package oauth defines the Provider interface and Identity type for
// delegated OAuth authentication. Each OAuth provider (GitHub, Google, etc.)
// is a separate file implementing Provider. Adding a new provider is a
// single new file — no other code is touched.
package oauth

import (
	"context"
	"errors"
)

// Identity is the normalised user identity returned by every Provider
// after a successful code exchange. It is dialect-independent: callers
// never see provider-specific types.
type Identity struct {
	// Provider is the canonical provider name, e.g. "github".
	Provider string
	// ProviderID is the stable numeric user ID expressed as a string.
	// For GitHub this is the integer /user.id field — stable across
	// username changes.
	ProviderID string
	// Email is the verified primary email address.
	Email string
	// DisplayName is the human-readable name from the provider, or the
	// username/login when the display name is absent.
	DisplayName string
}

// Provider is the seam every OAuth backend implements.
//
// Implementations must be safe for concurrent use. The Provider value is
// constructed once at startup and shared across all requests.
type Provider interface {
	// Name returns the canonical lowercase provider name, e.g. "github".
	// This is the value compared against the state-nonce "provider" column
	// and the request body "provider" field.
	Name() string

	// AuthorizeURL builds the redirect URL the browser should be sent to.
	// state is the opaque nonce stored server-side for CSRF protection.
	// redirectURI is the portal callback URL registered with the provider.
	AuthorizeURL(state, redirectURI string) string

	// Exchange converts an authorization code into a normalised Identity.
	// It performs all necessary API calls (token exchange + profile fetch)
	// and returns ErrExchange wrapping the provider error on failure.
	Exchange(ctx context.Context, code, redirectURI string) (Identity, error)
}

// ErrExchange is returned by Provider.Exchange when the upstream provider
// rejects or fails the code exchange.
type ErrExchange struct {
	Provider string
	Cause    error
}

func (e *ErrExchange) Error() string {
	return "oauth: " + e.Provider + " exchange failed: " + e.Cause.Error()
}

func (e *ErrExchange) Unwrap() error { return e.Cause }

// ErrBadGrant is returned (typically wrapped in *ErrExchange via Cause)
// when the provider explicitly rejects the authorization code at the
// business-logic layer — RFC 6749 `invalid_grant` / GitHub
// `bad_verification_code`. The upstream token endpoint returned a
// well-formed JSON body with an `error` field, NOT a transport failure.
//
// Callers (auth/oauth.go > OauthCallback) classify with
// errors.Is(err, oauth.ErrBadGrant) before falling back to a dep-class
// wrap so the SPA receives a 400 `oauth.invalid_grant` instead of
// 503 `dep.oauth_provider_unavailable` (which would suggest a futile
// retry).
var ErrBadGrant = errors.New("oauth: provider rejected the authorization code")
