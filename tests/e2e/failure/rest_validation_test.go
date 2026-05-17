// Invariant: the portal's REST surface enforces input validation, value
// boundaries, and permission checks at the documented HTTP error codes
// for every category surfaced in the OpenAPI spec. Each subtest asserts
// the status code AND — where the response is a JSON error envelope —
// the envelope's `error` field. Prose `message` fields are never asserted
// (they are localizable / formattable).
//
// Error envelope shape (docs/PROTOCOL.md > HTTP error contract):
//
//	{"error": "<machine-readable code>", "message": "<human-readable>"}
//
// Note: when the oapi-codegen strict handler rejects a request before it
// reaches business logic (e.g. malformed JSON body), it calls http.Error
// with a plain-text body rather than the JSON envelope. Those subtests
// assert only the status code.
package failure_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// errorEnvelope mirrors the ErrorEnvelope schema from openapi.yaml.
type errorEnvelope struct {
	Error   string         `json:"error"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// rawPostExpect sends a POST with a raw (pre-encoded) body, asserts the HTTP
// status, and — if wantError is non-empty — decodes the response as an
// errorEnvelope and asserts the `error` field matches wantError.
func rawPostExpect(ctx context.Context, t *testing.T, url string, body []byte, bearer string, wantStatus int, wantError string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("rawPostExpect: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rawPostExpect: POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("rawPostExpect: POST %s: status %d (want %d): %s", url, resp.StatusCode, wantStatus, respBody)
	}
	if wantError != "" {
		var env errorEnvelope
		if err := json.Unmarshal(respBody, &env); err != nil {
			t.Fatalf("rawPostExpect: decode error envelope from POST %s: %v\nbody: %s", url, err, respBody)
		}
		if env.Error != wantError {
			t.Fatalf("rawPostExpect: POST %s: error code %q (want %q)\nbody: %s", url, env.Error, wantError, respBody)
		}
	}
}

// getExpect sends a GET and asserts the HTTP status. If wantError is non-empty
// it also decodes the response as an errorEnvelope and asserts the `error` field.
func getExpect(ctx context.Context, t *testing.T, url string, bearer string, wantStatus int, wantError string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("getExpect: build request: %v", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("getExpect: GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("getExpect: GET %s: status %d (want %d): %s", url, resp.StatusCode, wantStatus, respBody)
	}
	if wantError != "" {
		var env errorEnvelope
		if err := json.Unmarshal(respBody, &env); err != nil {
			t.Fatalf("getExpect: decode error envelope from GET %s: %v\nbody: %s", url, err, respBody)
		}
		if env.Error != wantError {
			t.Fatalf("getExpect: GET %s: error code %q (want %q)\nbody: %s", url, env.Error, wantError, respBody)
		}
	}
}

func TestRestValidation(t *testing.T) {
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

	// ---------------------------------------------------------------------------
	// Invalid input
	// ---------------------------------------------------------------------------

	t.Run("invalid_input", func(t *testing.T) {
		t.Run("magic_link_exchange_malformed_json", func(t *testing.T) {
			// Invariant: sending a non-JSON body to /api/auth/magic-link/exchange
			// triggers the oapi-codegen strict-handler's JSON decode error path,
			// which returns 400. The response is plain text (not the JSON error
			// envelope) because the strict handler calls http.Error before
			// business logic runs.
			rawPostExpect(ctx, t,
				p.URL+"/api/auth/magic-link/exchange",
				[]byte("not json"),
				"", http.StatusBadRequest, "")
		})

		t.Run("magic_link_request_malformed_json", func(t *testing.T) {
			// Invariant: malformed JSON to /api/auth/magic-link/request → 400
			// before the handler runs (same strict-handler path as exchange).
			rawPostExpect(ctx, t,
				p.URL+"/api/auth/magic-link/request",
				[]byte("{bad json"),
				"", http.StatusBadRequest, "")
		})

		t.Run("create_org_malformed_json", func(t *testing.T) {
			// Invariant: malformed JSON to POST /api/orgs → 400 from the strict
			// handler's JSON decode error path. A valid bearer is still needed
			// because the route is behind BearerMiddleware; bearer validation runs
			// before body parsing.
			alice := authflow.SignInViaMagicLink(ctx, t, p, mh, "alice-invalid-org@example.com")
			rawPostExpect(ctx, t,
				p.URL+"/api/orgs",
				[]byte("not json"),
				alice.AccessToken, http.StatusBadRequest, "")
		})

		t.Run("magic_link_exchange_invalid_token_format", func(t *testing.T) {
			// Invariant: POST /api/auth/magic-link/exchange with a structurally
			// valid JSON body but a token value that does not match any stored
			// hash returns 401 auth.invalid_token.
			rawPostExpect(ctx, t,
				p.URL+"/api/auth/magic-link/exchange",
				[]byte(`{"token":"thisisnotavalidtoken"}`),
				"", http.StatusUnauthorized, "auth.invalid_token")
		})
	})

	// ---------------------------------------------------------------------------
	// Boundary values
	// ---------------------------------------------------------------------------

	t.Run("boundary_values", func(t *testing.T) {
		t.Run("magic_link_exchange_empty_token", func(t *testing.T) {
			// Invariant: exchanging an empty token string returns 401
			// auth.invalid_token — the empty string does not match any stored hash.
			rawPostExpect(ctx, t,
				p.URL+"/api/auth/magic-link/exchange",
				[]byte(`{"token":""}`),
				"", http.StatusUnauthorized, "auth.invalid_token")
		})

		t.Run("magic_link_token_reuse", func(t *testing.T) {
			// Invariant: a magic-link token is single-use. After a successful
			// exchange the token is consumed; attempting to exchange it a second
			// time returns 401 auth.invalid_token.
			const email = "reuse-test@example.com"

			// First sign-in: consume the token.
			pair := authflow.SignInViaMagicLink(ctx, t, p, mh, email)
			if pair.AccessToken == "" {
				t.Fatal("first sign-in: empty access_token")
			}

			// Re-request to get a fresh token in the inbox.
			authflow.PostJSON(ctx, t,
				p.URL+"/api/auth/magic-link/request",
				map[string]string{"email": email}, "", http.StatusNoContent)

			msg := mh.LatestMessageTo(ctx, t, email, 0)
			matches := authflow.MagicLinkTokenRE.FindStringSubmatch(msg.Body)
			if len(matches) < 2 {
				t.Fatal("could not extract token from second magic-link email")
			}
			freshToken := matches[1]

			// Exchange the fresh token once — should succeed.
			rawPostExpect(ctx, t,
				p.URL+"/api/auth/magic-link/exchange",
				[]byte(fmt.Sprintf(`{"token":%q}`, freshToken)),
				"", http.StatusOK, "")

			// Exchange the same token a second time — must be rejected.
			rawPostExpect(ctx, t,
				p.URL+"/api/auth/magic-link/exchange",
				[]byte(fmt.Sprintf(`{"token":%q}`, freshToken)),
				"", http.StatusUnauthorized, "auth.invalid_token")
		})
	})

	// ---------------------------------------------------------------------------
	// Permission failures
	// ---------------------------------------------------------------------------

	t.Run("permission_failures", func(t *testing.T) {
		t.Run("get_me_no_bearer", func(t *testing.T) {
			// Invariant: GET /me without an Authorization header returns 401
			// auth.invalid_token — BearerMiddleware rejects the request before
			// the handler runs.
			getExpect(ctx, t, p.URL+"/api/me", "", http.StatusUnauthorized, "auth.invalid_token")
		})

		t.Run("get_me_invalid_bearer", func(t *testing.T) {
			// Invariant: GET /me with an Authorization header that carries a
			// token not present in the store returns 401 auth.invalid_token.
			getExpect(ctx, t, p.URL+"/api/me", "invalid-token-xyz", http.StatusUnauthorized, "auth.invalid_token")
		})

		t.Run("create_org_no_bearer", func(t *testing.T) {
			// Invariant: POST /api/orgs without an Authorization header returns
			// 401 auth.invalid_token — BearerMiddleware blocks the request.
			rawPostExpect(ctx, t,
				p.URL+"/api/orgs",
				[]byte(`{"name":"ghost-org"}`),
				"", http.StatusUnauthorized, "auth.invalid_token")
		})

		t.Run("list_sessions_non_member", func(t *testing.T) {
			// Invariant: Alice cannot list sessions in Bob's org. She is
			// authenticated but not a member of that org; the portal returns
			// 403 auth.insufficient_permission.
			aliceTokens := authflow.SignInViaMagicLink(ctx, t, p, mh, "alice-perm@example.com")
			bobTokens := authflow.SignInViaMagicLink(ctx, t, p, mh, "bob-perm@example.com")

			// Bob creates his own org.
			bobOrgID := authflow.CreateOrg(ctx, t, p, bobTokens.AccessToken, "Bob Perm Org")

			// Alice tries to list sessions in Bob's org — must be forbidden.
			getExpect(ctx, t,
				fmt.Sprintf("%s/api/orgs/%s/sessions", p.URL, bobOrgID),
				aliceTokens.AccessToken,
				http.StatusForbidden, "auth.insufficient_permission")
		})

		t.Run("invite_accept_wrong_user", func(t *testing.T) {
			// Invariant: an invite token issued for charlie@example.com cannot be
			// accepted by dave@example.com. The portal verifies that the
			// authenticated user's email matches the invite's recipient_email and
			// returns 403 auth.insufficient_permission.
			charlieTokens := authflow.SignInViaMagicLink(ctx, t, p, mh, "charlie-invite@example.com")
			daveTokens := authflow.SignInViaMagicLink(ctx, t, p, mh, "dave-invite@example.com")

			// Charlie creates an org and invites an external address.
			charlieOrgID := authflow.CreateOrg(ctx, t, p, charlieTokens.AccessToken, "Charlie Invite Org")
			inviteID := authflow.InviteToOrg(ctx, t, p, charlieTokens.AccessToken, charlieOrgID, "extern-invite@example.com")

			// Capture the invite token before extern signs in.
			inviteToken := authflow.ExtractInviteToken(ctx, t, mh, "extern-invite@example.com")

			// Dave tries to accept an invite meant for extern — must be forbidden.
			acceptURL := fmt.Sprintf("%s/api/orgs/%s/invites/%s/accept", p.URL, charlieOrgID, inviteID)
			rawPostExpect(ctx, t,
				acceptURL,
				[]byte(fmt.Sprintf(`{"token":%q}`, inviteToken)),
				daveTokens.AccessToken, http.StatusForbidden, "auth.insufficient_permission")
		})

		t.Run("invite_accept_reuse", func(t *testing.T) {
			// Invariant: an invite token can only be accepted once. After a
			// successful acceptance, a second attempt returns 409 (conflict).
			eveTokens := authflow.SignInViaMagicLink(ctx, t, p, mh, "eve-reuse@example.com")
			frankTokens := authflow.SignInViaMagicLink(ctx, t, p, mh, "frank-reuse@example.com")

			// Eve creates an org and invites Frank.
			eveOrgID := authflow.CreateOrg(ctx, t, p, eveTokens.AccessToken, "Eve Reuse Org")
			inviteID := authflow.InviteToOrg(ctx, t, p, eveTokens.AccessToken, eveOrgID, "frank-reuse@example.com")
			inviteToken := authflow.ExtractInviteToken(ctx, t, mh, "frank-reuse@example.com")

			// First acceptance — should succeed.
			authflow.AcceptInvite(ctx, t, p, frankTokens.AccessToken, eveOrgID, inviteID, inviteToken)

			// Second acceptance of the same token — must return 409 invite.already_accepted.
			acceptURL := fmt.Sprintf("%s/api/orgs/%s/invites/%s/accept", p.URL, eveOrgID, inviteID)
			rawPostExpect(ctx, t,
				acceptURL,
				[]byte(fmt.Sprintf(`{"token":%q}`, inviteToken)),
				frankTokens.AccessToken, http.StatusConflict, "invite.already_accepted")
		})

		t.Run("list_org_members_non_member", func(t *testing.T) {
			// Invariant: an authenticated user who is not a member of an org
			// cannot list that org's members — returns 403.
			gracieTokens := authflow.SignInViaMagicLink(ctx, t, p, mh, "gracie-members@example.com")
			helenTokens := authflow.SignInViaMagicLink(ctx, t, p, mh, "helen-members@example.com")

			// Helen creates her own org; Gracie is not a member.
			helenOrgID := authflow.CreateOrg(ctx, t, p, helenTokens.AccessToken, "Helen Members Org")

			// Gracie tries to list Helen's org members — must be forbidden.
			getExpect(ctx, t,
				fmt.Sprintf("%s/api/orgs/%s/members", p.URL, helenOrgID),
				gracieTokens.AccessToken,
				http.StatusForbidden, "auth.insufficient_permission")
		})
	})
}

