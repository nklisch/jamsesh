// Invariant: a brand-new user can sign in via magic link, create an org,
// invite a second user, have that user sign in via their own magic link,
// accept the org invite, and then appear as a member of that org when
// calling GET /me. The flow exercises the real REST surface end-to-end:
// no test doubles of the portal, no shortcut DB writes, and no contact
// with real github.com or real SMTP servers.
package golden_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// randEmail returns a unique-per-run email of the form prefix-<hex>@example.com,
// keeping each test process's mail-inbox state isolated even when multiple
// suites or processes share a MailHog instance.
func randEmail(t *testing.T, prefix string) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("randEmail: rand.Read: %v", err)
	}
	return prefix + "-" + hex.EncodeToString(b) + "@example.com"
}

// TestOnboardingMagicLink exercises the full golden-path onboarding journey:
//
//  1. Alice signs in via magic link (request → mailhog → exchange).
//  2. Alice creates an org.
//  3. Alice invites bob@example.com (invite email lands in MailHog).
//  4. The invite token is captured from Bob's inbox before his magic-link email
//     arrives, so LatestMessageTo reliably identifies the invite.
//  5. Bob signs in via his own magic link (request → mailhog → exchange).
//  6. Bob accepts the invite using the token from step 4.
//  7. GET /me confirms Bob is now a member of Alice's org.
func TestOnboardingMagicLink(t *testing.T) {
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

	aliceEmail := randEmail(t, "alice")
	bobEmail := randEmail(t, "bob")

	// Step 1: Alice signs in via magic link.
	alice := authflow.SignInViaMagicLink(ctx, t, p, mh, aliceEmail)

	// Step 2: Alice creates an org.
	orgID := authflow.CreateOrg(ctx, t, p, alice.AccessToken, "Test Org")

	// Step 3: Alice invites Bob.
	inviteID := authflow.InviteToOrg(ctx, t, p, alice.AccessToken, orgID, bobEmail)

	// Step 4: Capture the invite token from Bob's email BEFORE Bob's magic-link
	// email arrives, so LatestMessageTo reliably returns the invite (not the
	// magic-link email sent in the next step).
	inviteToken := authflow.ExtractInviteToken(ctx, t, mh, bobEmail)

	// Step 5: Bob signs in via his own magic link.
	bob := authflow.SignInViaMagicLink(ctx, t, p, mh, bobEmail)

	// Step 6: Bob accepts Alice's invite.
	authflow.AcceptInvite(ctx, t, p, bob.AccessToken, orgID, inviteID, inviteToken)

	// Step 7: GET /me confirms Bob is a member of the org.
	authflow.RequireOrgMembership(ctx, t, p, bob.AccessToken, orgID)
}
