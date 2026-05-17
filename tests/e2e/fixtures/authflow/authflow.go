// Package authflow provides shared helpers for e2e specs that need to drive
// the jamsesh portal's auth + onboarding REST flows.
//
// All helpers are extracted from tests/e2e/golden/onboarding_test.go so that
// failure-mode and other specs can share them without duplication.
package authflow

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
)

// TokenPair mirrors the TokenPair schema from openapi.yaml.
type TokenPair struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	AccessExpiresAt  string `json:"access_expires_at"`
	RefreshExpiresAt string `json:"refresh_expires_at"`
}

// OrgRef mirrors the OrgRef schema.
type OrgRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// InviteRef mirrors the InviteRef schema.
type InviteRef struct {
	ID             string `json:"id"`
	RecipientEmail string `json:"recipient_email"`
	ExpiresAt      string `json:"expires_at"`
}

// MeResponse mirrors the MeResponse schema.
type MeResponse struct {
	ID          string       `json:"id"`
	Email       string       `json:"email"`
	DisplayName string       `json:"display_name"`
	Orgs        []MeOrgEntry `json:"orgs"`
}

// MeOrgEntry mirrors a single entry in MeResponse.Orgs.
type MeOrgEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Role string `json:"role"`
}

// MagicLinkTokenRE extracts the raw token from the URL in a magic-link email
// body. The server writes the link as:
//
//	{portalURL}/auth/magic-link?token={raw}
var MagicLinkTokenRE = regexp.MustCompile(`token=([A-Za-z0-9]+)`)

// InviteTokenRE extracts the raw invite token from the accept URL in an org
// invite email. The server writes:
//
//	{portalURL}/orgs/{orgID}/invites/{inviteID}/accept?token={raw}
var InviteTokenRE = regexp.MustCompile(`token=([A-Za-z0-9]+)`)

// SignInViaMagicLink performs the full magic-link sign-in flow:
// POST /api/auth/magic-link/request, polls MailHog for the email, extracts
// the token, and POSTs /api/auth/magic-link/exchange. Returns the issued token
// pair.
func SignInViaMagicLink(ctx context.Context, t *testing.T, p *portal.Portal, mh *mailhog.MailHog, email string) TokenPair {
	t.Helper()

	// Request the magic link.
	PostJSON(ctx, t, p.URL+"/api/auth/magic-link/request",
		map[string]string{"email": email}, "", http.StatusNoContent)

	// Fetch the email from MailHog and extract the raw token.
	msg := mh.LatestMessageTo(ctx, t, email, 5*time.Second)
	matches := MagicLinkTokenRE.FindStringSubmatch(msg.Body)
	if len(matches) < 2 {
		t.Fatalf("SignInViaMagicLink(%s): could not find token in email body:\n%s", email, msg.Body)
	}
	rawToken := matches[1]

	// Exchange the token for a portal token pair.
	var pair TokenPair
	PostJSONInto(ctx, t, p.URL+"/api/auth/magic-link/exchange",
		map[string]string{"token": rawToken}, "", http.StatusOK, &pair)
	if pair.AccessToken == "" {
		t.Fatalf("SignInViaMagicLink(%s): empty access_token in exchange response", email)
	}
	return pair
}

// CreateOrg calls POST /api/orgs and returns the new org's ID.
func CreateOrg(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, name string) string {
	t.Helper()
	var org OrgRef
	PostJSONInto(ctx, t, p.URL+"/api/orgs",
		map[string]string{"name": name}, accessToken, http.StatusCreated, &org)
	if org.ID == "" {
		t.Fatalf("CreateOrg: empty id in response")
	}
	return org.ID
}

// InviteToOrg calls POST /api/orgs/{orgID}/invites and returns the invite ID.
func InviteToOrg(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, email string) string {
	t.Helper()
	var invite InviteRef
	PostJSONInto(ctx, t, fmt.Sprintf("%s/api/orgs/%s/invites", p.URL, orgID),
		map[string]string{"email": email}, accessToken, http.StatusCreated, &invite)
	if invite.ID == "" {
		t.Fatalf("InviteToOrg: empty id in response")
	}
	return invite.ID
}

// ExtractInviteToken polls MailHog for the org invite email sent to email and
// returns the raw token from the accept URL.
func ExtractInviteToken(ctx context.Context, t *testing.T, mh *mailhog.MailHog, email string) string {
	t.Helper()
	msg := mh.LatestMessageTo(ctx, t, email, 5*time.Second)
	matches := InviteTokenRE.FindStringSubmatch(msg.Body)
	if len(matches) < 2 {
		t.Fatalf("ExtractInviteToken(%s): could not find token in invite email body:\n%s", email, msg.Body)
	}
	return matches[1]
}

// AcceptInvite calls POST /api/orgs/{orgID}/invites/{inviteID}/accept.
func AcceptInvite(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, inviteID, inviteToken string) {
	t.Helper()
	url := fmt.Sprintf("%s/api/orgs/%s/invites/%s/accept", p.URL, orgID, inviteID)
	var org OrgRef
	PostJSONInto(ctx, t, url,
		map[string]string{"token": inviteToken}, accessToken, http.StatusOK, &org)
	if org.ID == "" {
		t.Fatalf("AcceptInvite: empty org id in response")
	}
}

// RequireOrgMembership calls GET /me and asserts that orgID appears in the
// caller's org list.
func RequireOrgMembership(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/api/me", nil)
	if err != nil {
		t.Fatalf("RequireOrgMembership: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("RequireOrgMembership: GET /me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("RequireOrgMembership: GET /me: status %d: %s", resp.StatusCode, body)
	}
	var me MeResponse
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("RequireOrgMembership: decode response: %v\nbody: %s", err, body)
	}
	for _, org := range me.Orgs {
		if org.ID == orgID {
			return
		}
	}
	t.Fatalf("RequireOrgMembership: org %q not found in /me response; orgs=%v", orgID, me.Orgs)
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// PostJSON sends a POST with a JSON body and asserts the expected status code.
// Set bearer to "" to omit the Authorization header.
func PostJSON(ctx context.Context, t *testing.T, url string, body any, bearer string, wantStatus int) {
	t.Helper()
	PostJSONInto(ctx, t, url, body, bearer, wantStatus, nil)
}

// PostJSONInto sends a POST with a JSON body, asserts the expected status, and
// if dest is non-nil decodes the response body into it.
func PostJSONInto(ctx context.Context, t *testing.T, url string, body any, bearer string, wantStatus int, dest any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("PostJSONInto: marshal body: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("PostJSONInto: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PostJSONInto: POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("PostJSONInto: POST %s: status %d (want %d): %s", url, resp.StatusCode, wantStatus, respBody)
	}
	if dest != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, dest); err != nil {
			t.Fatalf("PostJSONInto: decode response from POST %s: %v\nbody: %s", url, err, respBody)
		}
	}
}
