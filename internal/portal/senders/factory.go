package senders

import (
	"context"
	"fmt"

	"jamsesh/internal/portal/config"
)

// disabledSender is returned by New when neither email.provider nor email.from
// is configured. It satisfies the Sender interface so the portal starts
// normally; any actual send attempt returns ErrMagicLinkNotEnabled so callers
// can translate it to a client-facing 4xx rather than a 5xx.
type disabledSender struct{}

func (disabledSender) Send(_ context.Context, _, _, _ string) error {
	return fmt.Errorf("%w", ErrMagicLinkNotEnabled)
}

// New constructs a Sender from the portal email config. The selected provider's
// credentials must be non-empty; the factory validates the minimum required
// fields so callers get a clear error at startup rather than at first send.
//
// Special case: when both email.provider and email.from are empty (entirely
// unconfigured), New returns a disabledSender with no error. This allows
// OAuth-only and no-auth deployments to start cleanly; magic-link request
// handlers detect ErrMagicLinkNotEnabled and return a client-facing 4xx.
//
// Adding a fifth provider is: a new *Sender file + a new case here. Nothing
// else needs to change.
func New(cfg config.EmailConfig) (Sender, error) {
	// Provider is the on/off switch for email. When empty, magic-link delivery
	// is disabled regardless of any partially-set email fields — From is only
	// meaningful when a Provider is selected. This lets OAuth-only and no-auth
	// deployments start cleanly, and keeps stray JAMSESH_EMAIL_FROM values
	// (e.g. test fixtures) from forcing a misleading "unknown provider" error.
	if cfg.Provider == "" {
		return disabledSender{}, nil
	}

	// Provider is set but from is missing — fail fast; the operator made a
	// partial configuration mistake.
	if cfg.From == "" {
		return nil, fmt.Errorf("senders: email.from must not be empty")
	}

	switch cfg.Provider {
	case "smtp":
		if cfg.SMTP.Host == "" {
			return nil, fmt.Errorf("senders: smtp.host is required for provider=smtp")
		}
		if cfg.SMTP.Port == 0 {
			return nil, fmt.Errorf("senders: smtp.port is required for provider=smtp")
		}
		return newSMTPSender(
			cfg.SMTP.Host,
			cfg.SMTP.Port,
			cfg.SMTP.User,
			cfg.SMTP.Pass,
			cfg.From,
			cfg.SMTP.TLSMode,
		), nil

	case "sendgrid":
		if cfg.SendGrid.APIKey == "" {
			return nil, fmt.Errorf("senders: sendgrid.api_key is required for provider=sendgrid")
		}
		return newSendGridSender(cfg.SendGrid.APIKey, cfg.From), nil

	case "postmark":
		if cfg.Postmark.ServerToken == "" {
			return nil, fmt.Errorf("senders: postmark.server_token is required for provider=postmark")
		}
		return newPostmarkSender(cfg.Postmark.ServerToken, cfg.From, cfg.Postmark.MessageStream), nil

	case "resend":
		if cfg.Resend.APIKey == "" {
			return nil, fmt.Errorf("senders: resend.api_key is required for provider=resend")
		}
		return newResendSender(cfg.Resend.APIKey, cfg.From), nil

	default:
		return nil, fmt.Errorf("senders: unknown email provider %q (want smtp|sendgrid|postmark|resend)", cfg.Provider)
	}
}
