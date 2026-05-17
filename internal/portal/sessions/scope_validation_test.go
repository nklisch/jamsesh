package sessions_test

import (
	"net/http"
	"testing"

	"jamsesh/internal/api/openapi"
)

// TestCreateSession_InvalidWritableScope covers the API-time validation
// added to close the gap where malformed globs were only caught at push
// time (story portal-validate-writable-scope-at-create-time).
func TestCreateSession_InvalidWritableScope(t *testing.T) {
	cases := []struct {
		name      string
		scope     string
		wantCode  int
		wantError string
	}{
		{
			name:      "unclosed brace",
			scope:     `["docs/{"]`,
			wantCode:  http.StatusBadRequest,
			wantError: "session.invalid_writable_scope",
		},
		{
			name:      "unclosed character class",
			scope:     `["[abc"]`,
			wantCode:  http.StatusBadRequest,
			wantError: "session.invalid_writable_scope",
		},
		{
			name:      "unclosed alternation",
			scope:     `["{a,b"]`,
			wantCode:  http.StatusBadRequest,
			wantError: "session.invalid_writable_scope",
		},
		{
			name:     "well-formed globs",
			scope:    `["docs/**", "src/*.go"]`,
			wantCode: http.StatusCreated,
		},
		{
			name:     "empty string is deny-all and allowed",
			scope:    ``,
			wantCode: http.StatusCreated,
		},
		{
			name:     "empty JSON array is deny-all and allowed",
			scope:    `[]`,
			wantCode: http.StatusCreated,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			acc := seedAccount(t, env.s, "creator@example.com")
			org := seedOrg(t, env.s, "Test Org", "scope-valid-"+t.Name())
			seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

			token := env.bearerToken(t, acc.ID)
			body := map[string]any{
				"name":         "Scope Validation",
				"goal":         "Test malformed scope handling",
				"scope":        tc.scope,
				"default_mode": "sync",
			}

			resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions", token, body)
			if resp.StatusCode != tc.wantCode {
				t.Fatalf("expected status %d, got %d", tc.wantCode, resp.StatusCode)
			}

			if tc.wantError != "" {
				var errEnv openapi.ErrorEnvelope
				decodeBody(t, resp, &errEnv)
				if errEnv.Error != tc.wantError {
					t.Errorf("expected error %q, got %q", tc.wantError, errEnv.Error)
				}
				if errEnv.Message == "" {
					t.Errorf("expected non-empty error message")
				}
			}
		})
	}
}

// TestPatchSession_InvalidWritableScope mirrors the create-time check on
// the patch path. The session is created with a known-good scope and then
// patched with the candidate scope; malformed candidates must be rejected
// with `session.invalid_writable_scope` and the row left unchanged.
func TestPatchSession_InvalidWritableScope(t *testing.T) {
	cases := []struct {
		name      string
		scope     string
		wantCode  int
		wantError string
	}{
		{
			name:      "unclosed brace",
			scope:     `["docs/{"]`,
			wantCode:  http.StatusBadRequest,
			wantError: "session.invalid_writable_scope",
		},
		{
			name:      "unclosed character class",
			scope:     `["[abc"]`,
			wantCode:  http.StatusBadRequest,
			wantError: "session.invalid_writable_scope",
		},
		{
			name:      "unclosed alternation",
			scope:     `["{a,b"]`,
			wantCode:  http.StatusBadRequest,
			wantError: "session.invalid_writable_scope",
		},
		{
			name:     "well-formed widening",
			scope:    `["src/**", "docs/**", "tests/**"]`,
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			acc := seedAccount(t, env.s, "creator@example.com")
			org := seedOrg(t, env.s, "Test Org", "scope-patch-"+t.Name())
			seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

			// createSession seeds scope as ["src/**"].
			sess := createSession(t, env, org.ID, acc.ID)
			token := env.bearerToken(t, acc.ID)

			path := "/api/orgs/" + org.ID + "/sessions/" + sess.Id
			body := map[string]any{"scope": tc.scope}
			resp := patchJSON(t, env.srv, path, token, body)
			if resp.StatusCode != tc.wantCode {
				t.Fatalf("expected status %d, got %d", tc.wantCode, resp.StatusCode)
			}

			if tc.wantError != "" {
				var errEnv openapi.ErrorEnvelope
				decodeBody(t, resp, &errEnv)
				if errEnv.Error != tc.wantError {
					t.Errorf("expected error %q, got %q", tc.wantError, errEnv.Error)
				}

				// Confirm the session row was not mutated.
				current, err := env.s.GetSession(t.Context(), org.ID, sess.Id)
				if err != nil {
					t.Fatalf("re-fetch session: %v", err)
				}
				if current.WritableScope != `["src/**"]` {
					t.Errorf("scope should be unchanged on rejection, got %q", current.WritableScope)
				}
			}
		})
	}
}
