package senders

import (
	"fmt"

	"jamsesh/internal/portal/config"
)

// New constructs a Sender from the portal email config. The selected provider's
// credentials must be non-empty; the factory validates the minimum required
// fields so callers get a clear error at startup rather than at first send.
//
// Adding a fifth provider is: a new *Sender file + a new case here. Nothing
// else needs to change.
func New(cfg config.EmailConfig) (Sender, error) {
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
