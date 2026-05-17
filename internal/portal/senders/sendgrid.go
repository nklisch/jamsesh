package senders

import (
	"context"
	"fmt"

	sendgrid "github.com/sendgrid/sendgrid-go"
	sgmail "github.com/sendgrid/sendgrid-go/helpers/mail"
)

// sendgridSender implements Sender using the sendgrid-go v3 library.
// Note: sendgrid-go's Send returns (*rest.Response, error). HTTP 4xx/5xx come
// back with err==nil and a non-2xx StatusCode — the error-table pitfall
// documented in the email-senders skill.
type sendgridSender struct {
	client *sendgrid.Client
	from   string
}

func newSendGridSender(apiKey, from string) *sendgridSender {
	return &sendgridSender{
		client: sendgrid.NewSendClient(apiKey),
		from:   from,
	}
}

func (s *sendgridSender) Send(ctx context.Context, recipient, subject, body string) error {
	from := sgmail.NewEmail("", s.from)
	to := sgmail.NewEmail("", recipient)
	msg := sgmail.NewSingleEmail(from, subject, to, body, "")

	resp, err := s.client.SendWithContext(ctx, msg)
	if err != nil {
		// Network-level error (DNS, TLS, timeout).
		return fmt.Errorf("%w: sendgrid request: %v", ErrTransient, err)
	}

	return classifySendGridStatus(resp.StatusCode)
}

// classifySendGridStatus maps HTTP response codes to sentinels.
// Success is 202; everything else is an error.
func classifySendGridStatus(code int) error {
	switch {
	case code == 202:
		return nil
	case code == 401 || code == 403:
		return fmt.Errorf("%w: sendgrid auth (HTTP %d)", ErrAuth, code)
	case code == 429:
		return fmt.Errorf("%w: sendgrid rate limited (HTTP 429)", ErrTransient)
	case code >= 400 && code < 500:
		return fmt.Errorf("%w: sendgrid rejected (HTTP %d)", ErrPermanent, code)
	case code >= 500:
		return fmt.Errorf("%w: sendgrid server error (HTTP %d)", ErrTransient, code)
	default:
		// Unexpected non-202 success range — treat as transient.
		return fmt.Errorf("%w: sendgrid unexpected status (HTTP %d)", ErrTransient, code)
	}
}
