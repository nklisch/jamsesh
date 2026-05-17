package senders

import (
	"context"
	"errors"
	"fmt"

	"github.com/mrz1836/postmark"
)

// postmarkSender implements Sender using mrz1836/postmark.
// IMPORTANT: mrz1836/postmark returns (EmailResponse, error) from SendEmail.
// The Go error covers network/HTTP failures; the ErrorCode field on the
// response covers Postmark-level rejections. Both must be checked.
type postmarkSender struct {
	client        *postmark.Client
	from          string
	messageStream string
}

func newPostmarkSender(serverToken, from, messageStream string) *postmarkSender {
	if messageStream == "" {
		messageStream = "outbound"
	}
	return &postmarkSender{
		// accountToken ("") is only needed for account-level admin operations —
		// not transactional email. Pass empty string to keep the adapter minimal.
		client:        postmark.NewClient(serverToken, ""),
		from:          from,
		messageStream: messageStream,
	}
}

func (s *postmarkSender) Send(ctx context.Context, recipient, subject, body string) error {
	email := postmark.Email{
		From:          s.from,
		To:            recipient,
		Subject:       subject,
		TextBody:      body,
		MessageStream: s.messageStream,
	}

	resp, err := s.client.SendEmail(ctx, email)
	if err != nil {
		// mrz1836/postmark wraps non-zero ErrorCode as ErrEmailFailed,
		// so we classify it below after also checking ErrorCode directly.
		if errors.Is(err, postmark.ErrEmailFailed) {
			return classifyPostmarkErrorCode(resp.ErrorCode, err)
		}
		// Network or HTTP-level error — retryable.
		return fmt.Errorf("%w: postmark request: %v", ErrTransient, err)
	}

	// Belt-and-suspenders: check ErrorCode even when err == nil.
	if resp.ErrorCode != 0 {
		return classifyPostmarkErrorCode(resp.ErrorCode, fmt.Errorf("postmark error %d: %s", resp.ErrorCode, resp.Message))
	}
	return nil
}

// classifyPostmarkErrorCode maps Postmark ErrorCode values to sentinels.
// Code 0 means success. See references/error-tables.md for the full table.
func classifyPostmarkErrorCode(code int64, cause error) error {
	if code == 0 {
		return nil
	}
	switch code {
	case 10, 400, 405:
		// 10=bad API token, 400=sender signature not confirmed, 405=account disabled.
		return fmt.Errorf("%w: postmark auth (code %d): %v", ErrAuth, code, cause)
	case 100, 429, 500:
		// 100=maintenance, 429=rate limited, 500=internal server error.
		return fmt.Errorf("%w: postmark transient (code %d): %v", ErrTransient, code, cause)
	case 300, 406, 412, 422:
		// 300=invalid email request, 406=inactive recipient,
		// 412=incompatible JSON, 422=bad message stream.
		return fmt.Errorf("%w: postmark permanent (code %d): %v", ErrPermanent, code, cause)
	default:
		if code >= 400 && code < 500 {
			return fmt.Errorf("%w: postmark permanent (code %d): %v", ErrPermanent, code, cause)
		}
		if code >= 500 {
			return fmt.Errorf("%w: postmark transient (code %d): %v", ErrTransient, code, cause)
		}
		return fmt.Errorf("%w: postmark unknown (code %d): %v", ErrTransient, code, cause)
	}
}
