package senders

import (
	"context"
	"errors"
	"fmt"
	"net/textproto"

	gomail "github.com/wneessen/go-mail"
)

// smtpSender implements Sender using wneessen/go-mail.
type smtpSender struct {
	host string
	port int
	user string
	pass string
	tls  string // "mandatory" | "opportunistic" | "none"
	from string
}

// newSMTPSender constructs an SMTP sender. tls should be "mandatory",
// "opportunistic", or "none"; the zero value defaults to "mandatory".
func newSMTPSender(host string, port int, user, pass, from, tlsMode string) *smtpSender {
	if tlsMode == "" {
		tlsMode = "mandatory"
	}
	return &smtpSender{
		host: host,
		port: port,
		user: user,
		pass: pass,
		from: from,
		tls:  tlsMode,
	}
}

// Send dials the SMTP server, sends one message, and disconnects.
// Each call opens a new connection — acceptable for low-volume magic-link
// delivery; a persistent connection pool can replace this later if needed.
func (s *smtpSender) Send(ctx context.Context, recipient, subject, body string) error {
	m := gomail.NewMsg()
	if err := m.From(s.from); err != nil {
		return fmt.Errorf("%w: smtp from address: %v", ErrPermanent, err)
	}
	if err := m.To(recipient); err != nil {
		return fmt.Errorf("%w: smtp to address: %v", ErrPermanent, err)
	}
	m.Subject(subject)
	m.SetBodyString(gomail.TypeTextPlain, body)

	opts := []gomail.Option{
		gomail.WithPort(s.port),
	}

	// TLS policy selection.
	switch s.tls {
	case "mandatory":
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	case "opportunistic":
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSOpportunistic))
	case "none":
		opts = append(opts, gomail.WithTLSPolicy(gomail.NoTLS))
	default:
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	}

	if s.user != "" {
		opts = append(opts,
			gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
			gomail.WithUsername(s.user),
			gomail.WithPassword(s.pass),
		)
	}

	c, err := gomail.NewClient(s.host, opts...)
	if err != nil {
		// Client construction failure is typically a config issue.
		return fmt.Errorf("%w: smtp client init: %v", ErrAuth, err)
	}

	if err := c.DialAndSendWithContext(ctx, m); err != nil {
		return classifySMTPError(err)
	}
	return nil
}

// classifySMTPError maps SMTP protocol errors to package sentinels.
// See references/error-tables.md for the full classification table.
func classifySMTPError(err error) error {
	var te *textproto.Error
	if errors.As(err, &te) {
		switch {
		case te.Code == 535:
			return fmt.Errorf("%w: smtp auth failed: %v", ErrAuth, err)
		case te.Code >= 400 && te.Code < 500:
			return fmt.Errorf("%w: smtp 4xx: %v", ErrTransient, err)
		case te.Code >= 500:
			return fmt.Errorf("%w: smtp 5xx: %v", ErrPermanent, err)
		}
	}
	// Network errors, TLS failures, DNS — all retryable.
	return fmt.Errorf("%w: smtp network: %v", ErrTransient, err)
}
