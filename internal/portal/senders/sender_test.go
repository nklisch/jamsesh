package senders_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"jamsesh/internal/portal/config"
	"jamsesh/internal/portal/senders"
)

// ---------------------------------------------------------------------------
// Factory tests
// ---------------------------------------------------------------------------

func TestNew_UnknownProvider_ReturnsError(t *testing.T) {
	_, err := senders.New(config.EmailConfig{Provider: "noop", From: "a@example.com"})
	if err == nil {
		t.Fatal("want error for unknown provider, got nil")
	}
}

func TestNew_EmptyFrom_ReturnsError(t *testing.T) {
	_, err := senders.New(config.EmailConfig{Provider: "smtp"})
	if err == nil {
		t.Fatal("want error for empty from, got nil")
	}
}

func TestNew_SMTP_MissingHost_ReturnsError(t *testing.T) {
	_, err := senders.New(config.EmailConfig{
		Provider: "smtp",
		From:     "noreply@example.com",
		SMTP:     config.SMTPConfig{Port: 587},
	})
	if err == nil {
		t.Fatal("want error for missing SMTP host, got nil")
	}
}

func TestNew_SMTP_MissingPort_ReturnsError(t *testing.T) {
	_, err := senders.New(config.EmailConfig{
		Provider: "smtp",
		From:     "noreply@example.com",
		SMTP:     config.SMTPConfig{Host: "localhost"},
	})
	if err == nil {
		t.Fatal("want error for missing SMTP port, got nil")
	}
}

func TestNew_SendGrid_MissingAPIKey_ReturnsError(t *testing.T) {
	_, err := senders.New(config.EmailConfig{Provider: "sendgrid", From: "a@example.com"})
	if err == nil {
		t.Fatal("want error for missing SendGrid API key, got nil")
	}
}

func TestNew_Postmark_MissingServerToken_ReturnsError(t *testing.T) {
	_, err := senders.New(config.EmailConfig{Provider: "postmark", From: "a@example.com"})
	if err == nil {
		t.Fatal("want error for missing Postmark server token, got nil")
	}
}

func TestNew_Resend_MissingAPIKey_ReturnsError(t *testing.T) {
	_, err := senders.New(config.EmailConfig{Provider: "resend", From: "a@example.com"})
	if err == nil {
		t.Fatal("want error for missing Resend API key, got nil")
	}
}

func TestNew_SMTP_ValidConfig_ReturnsSender(t *testing.T) {
	s, err := senders.New(config.EmailConfig{
		Provider: "smtp",
		From:     "noreply@example.com",
		SMTP: config.SMTPConfig{
			Host: "localhost",
			Port: 587,
		},
	})
	if err != nil {
		t.Fatalf("want sender, got error: %v", err)
	}
	if s == nil {
		t.Fatal("want non-nil sender")
	}
}

func TestNew_SendGrid_ValidConfig_ReturnsSender(t *testing.T) {
	s, err := senders.New(config.EmailConfig{
		Provider: "sendgrid",
		From:     "noreply@example.com",
		SendGrid: config.SendGridConfig{APIKey: "SG.test"},
	})
	if err != nil {
		t.Fatalf("want sender, got error: %v", err)
	}
	if s == nil {
		t.Fatal("want non-nil sender")
	}
}

func TestNew_Postmark_ValidConfig_ReturnsSender(t *testing.T) {
	s, err := senders.New(config.EmailConfig{
		Provider: "postmark",
		From:     "noreply@example.com",
		Postmark: config.PostmarkConfig{ServerToken: "pm-test"},
	})
	if err != nil {
		t.Fatalf("want sender, got error: %v", err)
	}
	if s == nil {
		t.Fatal("want non-nil sender")
	}
}

func TestNew_Resend_ValidConfig_ReturnsSender(t *testing.T) {
	s, err := senders.New(config.EmailConfig{
		Provider: "resend",
		From:     "noreply@example.com",
		Resend:   config.ResendConfig{APIKey: "re_test"},
	})
	if err != nil {
		t.Fatalf("want sender, got error: %v", err)
	}
	if s == nil {
		t.Fatal("want non-nil sender")
	}
}

// ---------------------------------------------------------------------------
// SendGrid mock-HTTP tests
// ---------------------------------------------------------------------------

// sendGridRequest is the JSON payload sent to the SendGrid API.
type sendGridRequest struct {
	From struct {
		Email string `json:"email"`
	} `json:"from"`
	Personalizations []struct {
		To []struct {
			Email string `json:"email"`
		} `json:"to"`
	} `json:"personalizations"`
	Subject string `json:"subject"`
}

// newSendGridMock starts an httptest.Server that responds with statusCode.
// It returns the server and a function to retrieve the last-received request.
func newSendGridMock(t *testing.T, statusCode int) (*httptest.Server, func() *sendGridRequest) {
	t.Helper()
	var last sendGridRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &last)
		w.WriteHeader(statusCode)
	}))
	t.Cleanup(srv.Close)
	return srv, func() *sendGridRequest { return &last }
}

// sendGridSenderForTest creates a SendGrid sender wired to the test server.
// We do this by constructing the sender normally and monkey-patching via env
// is not possible; instead we override the host on the client indirectly.
// Since sendgrid-go doesn't expose the host after construction, we test
// the error classification path by constructing a sender that points to a
// custom endpoint — this requires using the low-level factory directly.
// For now we test via the exported factory and verify the error contract.
func TestSendGrid_202_NoError(t *testing.T) {
	srv, _ := newSendGridMock(t, 202)
	_ = srv // used for reference; actual test uses patched client below
	// We can only test the classification logic directly via a wrapper sender.
	// See TestClassifySendGridStatus below.
	t.Log("SendGrid 202 classification: covered by TestClassifySendGridStatus")
}

func TestClassifySendGridStatus_202_NoError(t *testing.T) {
	if err := sendGridStatusToError(202); err != nil {
		t.Errorf("202: want nil, got %v", err)
	}
}

func TestClassifySendGridStatus_400_PermanentError(t *testing.T) {
	err := sendGridStatusToError(400)
	assertErrorIs(t, err, senders.ErrPermanent, "400")
}

func TestClassifySendGridStatus_401_AuthError(t *testing.T) {
	err := sendGridStatusToError(401)
	assertErrorIs(t, err, senders.ErrAuth, "401")
}

func TestClassifySendGridStatus_403_AuthError(t *testing.T) {
	err := sendGridStatusToError(403)
	assertErrorIs(t, err, senders.ErrAuth, "403")
}

func TestClassifySendGridStatus_429_TransientError(t *testing.T) {
	err := sendGridStatusToError(429)
	assertErrorIs(t, err, senders.ErrTransient, "429")
}

func TestClassifySendGridStatus_500_TransientError(t *testing.T) {
	err := sendGridStatusToError(500)
	assertErrorIs(t, err, senders.ErrTransient, "500")
}

// sendGridStatusToError is an internal helper exposed via a test-only shim.
// We can't call classifySendGridStatus directly (unexported), so we exercise
// it via a patched sender that calls a mock HTTP server.
func sendGridStatusToError(code int) error {
	// We use a mock server that returns the given status code.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
	}))
	defer srv.Close()

	// Build the sender using the real factory to hit the mock.
	s, err := senders.NewSendGridSenderForTest(srv.URL+"/v3/mail/send", "noreply@example.com")
	if err != nil {
		return fmt.Errorf("sender construction: %w", err)
	}
	return s.Send(context.Background(), "to@example.com", "subj", "body")
}

// ---------------------------------------------------------------------------
// Postmark mock-HTTP tests
// ---------------------------------------------------------------------------

func TestClassifyPostmarkErrorCode_0_NoError(t *testing.T) {
	err := senders.ClassifyPostmarkErrorCode(0, nil)
	if err != nil {
		t.Errorf("code 0: want nil, got %v", err)
	}
}

func TestClassifyPostmarkErrorCode_10_AuthError(t *testing.T) {
	err := senders.ClassifyPostmarkErrorCode(10, fmt.Errorf("bad token"))
	assertErrorIs(t, err, senders.ErrAuth, "code 10")
}

func TestClassifyPostmarkErrorCode_100_TransientError(t *testing.T) {
	err := senders.ClassifyPostmarkErrorCode(100, fmt.Errorf("maintenance"))
	assertErrorIs(t, err, senders.ErrTransient, "code 100")
}

func TestClassifyPostmarkErrorCode_300_PermanentError(t *testing.T) {
	err := senders.ClassifyPostmarkErrorCode(300, fmt.Errorf("invalid email"))
	assertErrorIs(t, err, senders.ErrPermanent, "code 300")
}

func TestClassifyPostmarkErrorCode_406_PermanentError(t *testing.T) {
	err := senders.ClassifyPostmarkErrorCode(406, fmt.Errorf("inactive recipient"))
	assertErrorIs(t, err, senders.ErrPermanent, "code 406")
}

func TestClassifyPostmarkErrorCode_429_TransientError(t *testing.T) {
	err := senders.ClassifyPostmarkErrorCode(429, fmt.Errorf("rate limit"))
	assertErrorIs(t, err, senders.ErrTransient, "code 429")
}

// ---------------------------------------------------------------------------
// Error sentinel tests
// ---------------------------------------------------------------------------

func TestErrorSentinels_AreDistinct(t *testing.T) {
	if errors.Is(senders.ErrTransient, senders.ErrPermanent) {
		t.Error("ErrTransient should not be ErrPermanent")
	}
	if errors.Is(senders.ErrPermanent, senders.ErrAuth) {
		t.Error("ErrPermanent should not be ErrAuth")
	}
	if errors.Is(senders.ErrAuth, senders.ErrTransient) {
		t.Error("ErrAuth should not be ErrTransient")
	}
}

func TestWrappedErrors_PreserveSentinel(t *testing.T) {
	wrapped := fmt.Errorf("%w: some detail", senders.ErrTransient)
	if !errors.Is(wrapped, senders.ErrTransient) {
		t.Error("wrapped ErrTransient: errors.Is should return true")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func assertErrorIs(t *testing.T, err error, target error, label string) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: want error wrapping %v, got nil", label, target)
		return
	}
	if !errors.Is(err, target) {
		t.Errorf("%s: want errors.Is(err, %v) == true, got err=%v", label, target, err)
	}
}
