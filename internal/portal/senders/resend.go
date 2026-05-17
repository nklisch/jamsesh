package senders

import (
	"context"
	"errors"
	"fmt"

	"github.com/resend/resend-go/v3"
)

// resendSender implements Sender using resend-go/v3.
// Always calls SendWithContext (NOT plain Send) for proper cancellation support.
type resendSender struct {
	client *resend.Client
	from   string
}

func newResendSender(apiKey, from string) *resendSender {
	return &resendSender{
		client: resend.NewClient(apiKey),
		from:   from,
	}
}

func (s *resendSender) Send(ctx context.Context, recipient, subject, body string) error {
	req := &resend.SendEmailRequest{
		From:    s.from,
		To:      []string{recipient},
		Subject: subject,
		Text:    body,
	}

	_, err := s.client.Emails.SendWithContext(ctx, req)
	if err != nil {
		return classifyResendError(err)
	}
	return nil
}

// classifyResendError maps resend-go error types to package sentinels.
// See references/error-tables.md for the full classification table.
//
// Note: resend-go/v3 currently returns plain errors.New strings from its
// HTTP error handler rather than typed error values (except for ErrRateLimit
// which uses the Is pattern). Classification relies on the sentinel check;
// unknown errors fall through to ErrTransient.
func classifyResendError(err error) error {
	if err == nil {
		return nil
	}
	// Rate limit has its own sentinel type with Is() support.
	if errors.Is(err, resend.ErrRateLimit) {
		return fmt.Errorf("%w: resend rate limited: %v", ErrTransient, err)
	}
	// Context cancellation / timeout — transient.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: resend request cancelled: %v", ErrTransient, err)
	}
	// All other errors from the library are plain errors.New strings that
	// don't carry structured status codes in this SDK version. Treat as
	// transient (retryable) so callers have a chance to succeed.
	return fmt.Errorf("%w: resend request: %v", ErrTransient, err)
}
