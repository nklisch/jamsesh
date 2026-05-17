package mailhog_test

import (
	"context"
	"testing"

	"jamsesh/tests/e2e/fixtures/mailhog"
)

// TestStartMailHog verifies that Start brings up a MailHog container, that
// the SMTP port and HTTP API are populated, and that the HTTP API is reachable.
func TestStartMailHog(t *testing.T) {
	ctx := context.Background()
	mh := mailhog.Start(ctx, t)

	if mh.SMTPHost == "" {
		t.Fatal("expected non-empty SMTPHost")
	}
	if mh.SMTPPort == 0 {
		t.Fatal("expected non-zero SMTPPort")
	}
	if mh.HTTPURL == "" {
		t.Fatal("expected non-empty HTTPURL")
	}

	if err := mh.CheckReachable(); err != nil {
		t.Fatalf("mailhog HTTP API not reachable: %v", err)
	}
}
