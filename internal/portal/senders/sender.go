// Package senders provides the Sender interface and concrete implementations
// for transactional email delivery. The Sender interface is the seam between
// auth flows and email infrastructure; adding a fifth provider is a single new
// file implementing Sender plus a case in the factory.
package senders

import (
	"context"
	"errors"
)

// Sender is the only interface auth flow code depends on. Implementations are
// selected at startup by the factory (New); callers never reference concrete types.
type Sender interface {
	// Send delivers a plain-text email to recipient. Subject and body are
	// mandatory. Implementations set the From address from their config.
	//
	// Errors are always wrapped in one of the three package sentinels so the
	// caller can make retry decisions without provider-specific introspection.
	Send(ctx context.Context, recipient, subject, body string) error
}

// Error sentinels — wrap with fmt.Errorf("%w: ...", ErrXxx) so errors.Is works.
var (
	// ErrTransient signals a failure the caller should retry with backoff.
	// Examples: network timeouts, 5xx responses, SMTP 4xx temp-fail codes.
	ErrTransient = errors.New("senders: transient error")

	// ErrPermanent signals a failure the caller should NOT retry.
	// Examples: invalid recipient address, spam rejection, 4xx non-rate-limit responses.
	ErrPermanent = errors.New("senders: permanent error")

	// ErrAuth signals a provider credentials / configuration problem.
	// The operator must fix the config; retrying will not help.
	ErrAuth = errors.New("senders: auth/config error")

	// ErrMagicLinkNotEnabled signals that the portal was started without an
	// email provider configured, so magic-link auth is unavailable. Callers
	// (magic-link request handler) translate this to a client-facing 4xx; it
	// is not a transient or retryable failure.
	ErrMagicLinkNotEnabled = errors.New("senders: magic-link auth not enabled (no email provider configured)")
)
