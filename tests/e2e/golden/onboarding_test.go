// Invariant: a brand-new user can sign in via magic link, create an org,
// invite a second user, have that user sign in via their own magic link,
// accept the org invite, and then appear as a member of that org when
// calling GET /me. The flow exercises the real REST surface end-to-end:
// no test doubles of the portal, no shortcut DB writes, and no contact
// with real github.com or real SMTP servers.
package golden_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// tokenPair mirrors the TokenPair schema from openapi.yaml.
type tokenPair struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	AccessExpiresAt  string `json:"access_expires_at"`
	RefreshExpiresAt string `json:"refresh_expires_at"`
}

// orgRef mirrors the OrgRef schema.
type orgRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// inviteRef mirrors the InviteRef schema.
type inviteRef struct {
	ID             string `json:"id"`
	RecipientEmail string `json:"recipient_email"`
	ExpiresAt      string `json:"expires_at"`
}

// meResponse mirrors the MeResponse schema.
type meResponse struct {
	ID          string        `json:"id"`
	Email       string        `json:"email"`
	DisplayName string        `json:"display_name"`
	Orgs        []meOrgEntry  `json:"orgs"`
}

type meOrgEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Role string `json:"role"`
}

// magicLinkTokenRE extracts the raw token from the URL in a magic-link email
// body. The server writes the link as:
//
//	{portalURL}/auth/magic-link?token={raw}
var magicLinkTokenRE = regexp.MustCompile(`token=([A-Za-z0-9]+)`)

// inviteTokenRE extracts the raw invite token from the accept URL in an org
// invite email. The server writes:
//
//	{portalURL}/orgs/{orgID}/invites/{inviteID}/accept?token={raw}
var inviteTokenRE = regexp.MustCompile(`token=([A-Za-z0-9]+)`)

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

	// Step 1: Alice signs in via magic link.
	alice := signInViaMagicLink(ctx, t, p, mh, "alice@example.com")

	// Step 2: Alice creates an org.
	orgID := createOrg(ctx, t, p, alice.AccessToken, "Test Org")

	// Step 3: Alice invites Bob.
	inviteID := inviteToOrg(ctx, t, p, alice.AccessToken, orgID, "bob@example.com")

	// Step 4: Capture the invite token from Bob's email BEFORE Bob's magic-link
	// email arrives, so LatestMessageTo reliably returns the invite (not the
	// magic-link email sent in the next step).
	inviteToken := extractInviteToken(ctx, t, mh, "bob@example.com")

	// Step 5: Bob signs in via his own magic link.
	bob := signInViaMagicLink(ctx, t, p, mh, "bob@example.com")

	// Step 6: Bob accepts Alice's invite.
	acceptInvite(ctx, t, p, bob.AccessToken, orgID, inviteID, inviteToken)

	// Step 7: GET /me confirms Bob is a member of the org.
	requireOrgMembership(ctx, t, p, bob.AccessToken, orgID)
}

// signInViaMagicLink performs the full magic-link sign-in flow:
// POST /api/auth/magic-link/request, polls MailHog for the email, extracts
// the token, and POSTs /api/auth/magic-link/exchange. Returns the issued token
// pair.
func signInViaMagicLink(ctx context.Context, t *testing.T, p *portal.Portal, mh *mailhog.MailHog, email string) tokenPair {
	t.Helper()

	// Request the magic link.
	postJSON(ctx, t, p.URL+"/api/auth/magic-link/request",
		map[string]string{"email": email}, "", http.StatusNoContent)

	// Fetch the email from MailHog and extract the raw token.
	msg := mh.LatestMessageTo(ctx, t, email, 5*time.Second)
	matches := magicLinkTokenRE.FindStringSubmatch(msg.Body)
	if len(matches) < 2 {
		t.Fatalf("signInViaMagicLink(%s): could not find token in email body:\n%s", email, msg.Body)
	}
	rawToken := matches[1]

	// Exchange the token for a portal token pair.
	var pair tokenPair
	postJSONInto(ctx, t, p.URL+"/api/auth/magic-link/exchange",
		map[string]string{"token": rawToken}, "", http.StatusOK, &pair)
	if pair.AccessToken == "" {
		t.Fatalf("signInViaMagicLink(%s): empty access_token in exchange response", email)
	}
	return pair
}

// createOrg calls POST /api/orgs and returns the new org's ID.
func createOrg(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, name string) string {
	t.Helper()
	var org orgRef
	postJSONInto(ctx, t, p.URL+"/api/orgs",
		map[string]string{"name": name}, accessToken, http.StatusCreated, &org)
	if org.ID == "" {
		t.Fatalf("createOrg: empty id in response")
	}
	return org.ID
}

// inviteToOrg calls POST /api/orgs/{orgID}/invites and returns the invite ID.
func inviteToOrg(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, email string) string {
	t.Helper()
	var invite inviteRef
	postJSONInto(ctx, t, fmt.Sprintf("%s/api/orgs/%s/invites", p.URL, orgID),
		map[string]string{"email": email}, accessToken, http.StatusCreated, &invite)
	if invite.ID == "" {
		t.Fatalf("inviteToOrg: empty id in response")
	}
	return invite.ID
}

// extractInviteToken polls MailHog for the org invite email sent to email and
// returns the raw token from the accept URL.
func extractInviteToken(ctx context.Context, t *testing.T, mh *mailhog.MailHog, email string) string {
	t.Helper()
	// The invite email arrives after the magic-link email, so poll with a
	// slightly longer timeout to account for both being in MailHog; we use
	// the most-recent message addressed to this recipient, which will be the
	// invite (sent after the magic-link).
	msg := mh.LatestMessageTo(ctx, t, email, 5*time.Second)
	matches := inviteTokenRE.FindStringSubmatch(msg.Body)
	if len(matches) < 2 {
		t.Fatalf("extractInviteToken(%s): could not find token in invite email body:\n%s", email, msg.Body)
	}
	return matches[1]
}

// acceptInvite calls POST /api/orgs/{orgID}/invites/{inviteID}/accept.
func acceptInvite(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, inviteID, inviteToken string) {
	t.Helper()
	url := fmt.Sprintf("%s/api/orgs/%s/invites/%s/accept", p.URL, orgID, inviteID)
	var org orgRef
	postJSONInto(ctx, t, url,
		map[string]string{"token": inviteToken}, accessToken, http.StatusOK, &org)
	if org.ID == "" {
		t.Fatalf("acceptInvite: empty org id in response")
	}
}

// requireOrgMembership calls GET /me and asserts that orgID appears in the
// caller's org list.
func requireOrgMembership(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/api/me", nil)
	if err != nil {
		t.Fatalf("requireOrgMembership: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("requireOrgMembership: GET /me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("requireOrgMembership: GET /me: status %d: %s", resp.StatusCode, body)
	}
	var me meResponse
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("requireOrgMembership: decode response: %v\nbody: %s", err, body)
	}
	for _, org := range me.Orgs {
		if org.ID == orgID {
			return
		}
	}
	t.Fatalf("requireOrgMembership: org %q not found in /me response; orgs=%v", orgID, me.Orgs)
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// postJSON sends a POST with a JSON body and asserts the expected status code.
// Set bearer to "" to omit the Authorization header.
func postJSON(ctx context.Context, t *testing.T, url string, body any, bearer string, wantStatus int) {
	t.Helper()
	postJSONInto(ctx, t, url, body, bearer, wantStatus, nil)
}

// postJSONInto sends a POST with a JSON body, asserts the expected status, and
// if dest is non-nil decodes the response body into it.
func postJSONInto(ctx context.Context, t *testing.T, url string, body any, bearer string, wantStatus int, dest any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("postJSONInto: marshal body: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("postJSONInto: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("postJSONInto: POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("postJSONInto: POST %s: status %d (want %d): %s", url, resp.StatusCode, wantStatus, respBody)
	}
	if dest != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, dest); err != nil {
			t.Fatalf("postJSONInto: decode response from POST %s: %v\nbody: %s", url, err, respBody)
		}
	}
}
