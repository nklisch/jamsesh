// Package authflow_test is a minimal self-test that verifies the exported
// helpers compose correctly against a live portal + MailHog stack.
//
// Running this test requires Docker and the jamsesh/portal:e2e image:
//
//	make test-portal-image
//	cd tests/e2e && go test ./fixtures/authflow/ -v
package authflow_test

import (
	"context"
	"testing"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestAuthflow_SignInAndCreateOrg verifies that SignInViaMagicLink and
// CreateOrg work end-to-end against the real portal stack. If these helpers
// are broken, all specs that depend on authflow will fail with confusing
// errors; catching it here surfaces the failure clearly.
func TestAuthflow_SignInAndCreateOrg(t *testing.T) {
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		SMTPHost:  mh.ContainerSMTPHost,
		SMTPPort:  mh.ContainerSMTPPort,
	})

	// Invariant: a user can sign in via magic link and receive a valid token pair.
	alice := authflow.SignInViaMagicLink(ctx, t, p, mh, "alice@example.com")
	if alice.AccessToken == "" {
		t.Fatal("SignInViaMagicLink: got empty access_token")
	}
	if alice.RefreshToken == "" {
		t.Fatal("SignInViaMagicLink: got empty refresh_token")
	}

	// Invariant: an authenticated user can create an org and receive its ID.
	orgID := authflow.CreateOrg(ctx, t, p, alice.AccessToken, "Self-Test Org")
	if orgID == "" {
		t.Fatal("CreateOrg: got empty org ID")
	}

	// Invariant: GET /me reflects the newly created org membership.
	authflow.RequireOrgMembership(ctx, t, p, alice.AccessToken, orgID)
}
