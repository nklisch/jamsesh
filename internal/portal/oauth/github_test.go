package oauth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/portal/oauth"
)

// fakeGitHub wires up a httptest.Server that mocks the three GitHub
// endpoints used by Exchange: /login/oauth/access_token, /user, /user/emails.
type fakeGitHub struct {
	srv *httptest.Server
}

func newFakeGitHub(t *testing.T, opts fakeGitHubOpts) *fakeGitHub {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Always return a fixed access token.
		w.Header().Set("Content-Type", "application/json")
		if opts.tokenError != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":             opts.tokenError,
				"error_description": "test error",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"access_token": "ghs_test_token",
			"token_type":   "bearer",
		})
	})

	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		if opts.emailsError {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		emails := opts.emails
		if emails == nil {
			emails = []map[string]interface{}{
				{"email": "user@example.com", "primary": true, "verified": true},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(emails)
	})

	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		if opts.userError {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		profile := opts.user
		if profile == nil {
			profile = map[string]interface{}{
				"id":    int64(12345),
				"login": "testuser",
				"name":  "Test User",
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(profile)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &fakeGitHub{srv: srv}
}

type fakeGitHubOpts struct {
	tokenError  string
	userError   bool
	emailsError bool
	user        map[string]interface{}
	emails      []map[string]interface{}
}

func (f *fakeGitHub) provider(t *testing.T) *oauth.GitHub {
	t.Helper()
	return oauth.NewGitHub(oauth.GitHubOptions{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		BaseURL:      f.srv.URL,
		HTTPClient:   f.srv.Client(),
	})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestGitHub_Name(t *testing.T) {
	g := oauth.NewGitHub(oauth.GitHubOptions{ClientID: "id", ClientSecret: "secret"})
	if got := g.Name(); got != "github" {
		t.Errorf("Name() = %q, want %q", got, "github")
	}
}

func TestGitHub_AuthorizeURL(t *testing.T) {
	g := oauth.NewGitHub(oauth.GitHubOptions{ClientID: "myclient", ClientSecret: "s"})
	raw := g.AuthorizeURL("mynonce", "https://portal.example.com/auth/oauth/callback")

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	q := u.Query()

	if got := q.Get("client_id"); got != "myclient" {
		t.Errorf("client_id = %q, want %q", got, "myclient")
	}
	if got := q.Get("state"); got != "mynonce" {
		t.Errorf("state = %q, want %q", got, "mynonce")
	}
	if got := q.Get("redirect_uri"); got != "https://portal.example.com/auth/oauth/callback" {
		t.Errorf("redirect_uri = %q, want different", got)
	}
	scope := q.Get("scope")
	if !strings.Contains(scope, "read:user") || !strings.Contains(scope, "user:email") {
		t.Errorf("scope %q does not contain expected scopes", scope)
	}
}

func TestGitHub_AuthorizeURL_WithBaseURL(t *testing.T) {
	g := oauth.NewGitHub(oauth.GitHubOptions{
		ClientID:     "id",
		ClientSecret: "secret",
		BaseURL:      "http://localhost:9999",
	})
	raw := g.AuthorizeURL("nonce", "https://redirect.example.com/cb")
	if !strings.HasPrefix(raw, "http://localhost:9999/login/oauth/authorize") {
		t.Errorf("AuthorizeURL = %q, want base URL prefix", raw)
	}
}

func TestGitHub_Exchange_Success(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{
		user: map[string]interface{}{
			"id":    int64(99),
			"login": "octocat",
			"name":  "The Octocat",
		},
		emails: []map[string]interface{}{
			{"email": "octocat@github.com", "primary": true, "verified": true},
			{"email": "old@example.com", "primary": false, "verified": true},
		},
	})
	g := fake.provider(t)

	id, err := g.Exchange(context.Background(), "authcode", "https://redirect.example.com/cb")
	if err != nil {
		t.Fatalf("Exchange() error: %v", err)
	}

	if id.Provider != "github" {
		t.Errorf("Provider = %q, want %q", id.Provider, "github")
	}
	if id.ProviderID != "99" {
		t.Errorf("ProviderID = %q, want %q", id.ProviderID, "99")
	}
	if id.Email != "octocat@github.com" {
		t.Errorf("Email = %q, want %q", id.Email, "octocat@github.com")
	}
	if id.DisplayName != "The Octocat" {
		t.Errorf("DisplayName = %q, want %q", id.DisplayName, "The Octocat")
	}
}

func TestGitHub_Exchange_FallsBackToLogin_WhenNameEmpty(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{
		user: map[string]interface{}{
			"id":    int64(42),
			"login": "ghostuser",
			"name":  "", // empty name
		},
	})
	id, err := fake.provider(t).Exchange(context.Background(), "code", "https://x.example.com/cb")
	if err != nil {
		t.Fatalf("Exchange() error: %v", err)
	}
	if id.DisplayName != "ghostuser" {
		t.Errorf("DisplayName = %q, want %q", id.DisplayName, "ghostuser")
	}
}

// TestGitHub_Exchange_PicksVerifiedPrimaryEmailFromList asserts the only path
// that returns a non-error response: an email list that contains a
// primary+verified entry among mixed entries. The fixture includes one
// unverified primary and one non-primary-verified entry to actively guard
// against re-introducing a fallback chain — if any fallback were added back,
// the wrong email would be returned and this test would fail.
func TestGitHub_Exchange_PicksVerifiedPrimaryEmailFromList(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{
		emails: []map[string]interface{}{
			{"email": "secondary-verified@example.com", "primary": false, "verified": true},
			{"email": "primary-unverified@example.com", "primary": true, "verified": false},
			{"email": "primary-verified@example.com", "primary": true, "verified": true},
		},
	})
	id, err := fake.provider(t).Exchange(context.Background(), "code", "https://x.example.com/cb")
	if err != nil {
		t.Fatalf("Exchange() error: %v", err)
	}
	// Only the primary+verified entry is valid. Returning secondary-verified
	// would indicate a fallback to non-primary rows; returning
	// primary-unverified would indicate a fallback to unverified rows.
	if id.Email != "primary-verified@example.com" {
		t.Errorf("Email = %q, want primary-verified@example.com (not secondary-verified or primary-unverified)", id.Email)
	}
}

// TestGitHub_Exchange_NoVerifiedEmail_RejectsWithUnverifiedEmail verifies that
// when the email list has a primary entry that is NOT verified, Exchange rejects
// the flow with ErrUnverifiedEmail. Accepting an unverified email would enable
// account-confusion attacks; the provider must enforce verification.
func TestGitHub_Exchange_NoVerifiedEmail_RejectsWithUnverifiedEmail(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{
		emails: []map[string]interface{}{
			{"email": "x@y.example.com", "primary": true, "verified": false},
		},
	})
	_, err := fake.provider(t).Exchange(context.Background(), "code", "https://x.example.com/cb")
	if err == nil {
		t.Fatal("Exchange() returned nil error; want ErrUnverifiedEmail rejection for unverified primary email")
	}
	if !errors.Is(err, oauth.ErrUnverifiedEmail) {
		t.Errorf("errors.Is(err, oauth.ErrUnverifiedEmail) = false, want true; err = %v", err)
	}
}

// TestGitHub_Exchange_OnlyNonPrimaryVerified_AlsoRejects verifies that a
// verified email that is NOT the primary is also rejected. There must be no
// silent fallback to the first-row or any non-primary entry — such a fallback
// would break the account model (primary is the canonical identity address).
func TestGitHub_Exchange_OnlyNonPrimaryVerified_AlsoRejects(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{
		emails: []map[string]interface{}{
			{"email": "x@y.example.com", "primary": false, "verified": true},
		},
	})
	_, err := fake.provider(t).Exchange(context.Background(), "code", "https://x.example.com/cb")
	if err == nil {
		t.Fatal("Exchange() returned nil error; want ErrUnverifiedEmail rejection for non-primary verified email")
	}
	if !errors.Is(err, oauth.ErrUnverifiedEmail) {
		t.Errorf("errors.Is(err, oauth.ErrUnverifiedEmail) = false, want true; err = %v", err)
	}
}

// TestGitHub_Exchange_EmptyEmailList_Rejects verifies that an empty email list
// is rejected with ErrUnverifiedEmail. A GitHub account without any listed
// email has no verified primary by definition.
func TestGitHub_Exchange_EmptyEmailList_Rejects(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{
		emails: []map[string]interface{}{}, // explicitly empty
	})
	_, err := fake.provider(t).Exchange(context.Background(), "code", "https://x.example.com/cb")
	if err == nil {
		t.Fatal("Exchange() returned nil error; want ErrUnverifiedEmail rejection for empty email list")
	}
	if !errors.Is(err, oauth.ErrUnverifiedEmail) {
		t.Errorf("errors.Is(err, oauth.ErrUnverifiedEmail) = false, want true; err = %v", err)
	}
}

func TestGitHub_Exchange_TokenError(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{tokenError: "bad_verification_code"})
	_, err := fake.provider(t).Exchange(context.Background(), "bad", "https://x.example.com/cb")
	if err == nil {
		t.Fatal("expected error from Exchange when token endpoint returns error")
	}
	var exchErr *oauth.ErrExchange
	if !isExchangeErr(err, &exchErr) {
		t.Errorf("error should be *ErrExchange, got %T: %v", err, err)
	}
}

// TestGitHub_Exchange_BadVerificationCode_WrapsErrBadGrant verifies that
// when the upstream token endpoint returns HTTP 200 with a JSON body
// carrying `{"error": "bad_verification_code"}` (RFC 6749's business
// rejection), the returned error chain carries oauth.ErrBadGrant. This
// is the signal the OauthCallback handler uses to emit 400
// `oauth.invalid_grant` instead of the dep-class 503.
func TestGitHub_Exchange_BadVerificationCode_WrapsErrBadGrant(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{tokenError: "bad_verification_code"})
	_, err := fake.provider(t).Exchange(context.Background(), "expired-code", "https://x.example.com/cb")
	if err == nil {
		t.Fatal("expected error from Exchange when token endpoint returns bad_verification_code")
	}
	if !errors.Is(err, oauth.ErrBadGrant) {
		t.Errorf("errors.Is(err, oauth.ErrBadGrant) = false, want true; err = %v", err)
	}
	// The wrapped error should still mention the upstream error code
	// for operator debugging.
	if !strings.Contains(err.Error(), "bad_verification_code") {
		t.Errorf("error message should preserve upstream error code; got %q", err.Error())
	}
}

// TestGitHub_Exchange_InvalidGrant_WrapsErrBadGrant exercises the same
// branch with RFC 6749's canonical `invalid_grant` code (any non-empty
// `error` field in the 200-response body is a business rejection).
func TestGitHub_Exchange_InvalidGrant_WrapsErrBadGrant(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{tokenError: "invalid_grant"})
	_, err := fake.provider(t).Exchange(context.Background(), "code", "https://x.example.com/cb")
	if err == nil {
		t.Fatal("expected error from Exchange when token endpoint returns invalid_grant")
	}
	if !errors.Is(err, oauth.ErrBadGrant) {
		t.Errorf("errors.Is(err, oauth.ErrBadGrant) = false, want true; err = %v", err)
	}
}

// TestGitHub_Exchange_TransportError_DoesNotWrapErrBadGrant guards the
// taxonomy boundary: a real transport failure (e.g. /user 5xx) must not
// be misclassified as a business rejection.
func TestGitHub_Exchange_TransportError_DoesNotWrapErrBadGrant(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{userError: true})
	_, err := fake.provider(t).Exchange(context.Background(), "code", "https://x.example.com/cb")
	if err == nil {
		t.Fatal("expected error from Exchange when /user fails")
	}
	if errors.Is(err, oauth.ErrBadGrant) {
		t.Errorf("transport failure incorrectly matched oauth.ErrBadGrant: %v", err)
	}
}

func TestGitHub_Exchange_UserError(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{userError: true})
	_, err := fake.provider(t).Exchange(context.Background(), "code", "https://x.example.com/cb")
	if err == nil {
		t.Fatal("expected error when /user endpoint fails")
	}
}

func TestGitHub_Exchange_EmailsError(t *testing.T) {
	fake := newFakeGitHub(t, fakeGitHubOpts{emailsError: true})
	_, err := fake.provider(t).Exchange(context.Background(), "code", "https://x.example.com/cb")
	if err == nil {
		t.Fatal("expected error when /user/emails endpoint fails")
	}
}

// TestGitHub_BaseURL_SubstitutesAllEndpoints verifies that when BaseURL is set,
// the provider routes all three GitHub endpoints (token exchange, /user,
// /user/emails) through the substituted base URL and no requests escape to
// real github.com or api.github.com.
func TestGitHub_BaseURL_SubstitutesAllEndpoints(t *testing.T) {
	// Track which paths were hit on the fake server.
	hitPaths := map[string]int{}

	// Wrap the fake with a recording transport so we can assert request hosts.
	fake := newFakeGitHub(t, fakeGitHubOpts{
		user: map[string]interface{}{
			"id":    int64(7),
			"login": "testuser",
			"name":  "Test User",
		},
		emails: []map[string]interface{}{
			{"email": "test@example.com", "primary": true, "verified": true},
		},
	})

	// Install a custom transport that records the host of every outgoing
	// request. Any host other than the fake server's host is a test failure.
	fakeHost := fake.srv.URL // e.g. "http://127.0.0.1:PORT"
	recorder := &recordingTransport{
		wrapped:  fake.srv.Client().Transport,
		fakeHost: fakeHost,
		hitPaths: hitPaths,
		t:        t,
	}

	g := oauth.NewGitHub(oauth.GitHubOptions{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		BaseURL:      fakeHost,
		HTTPClient:   &http.Client{Transport: recorder},
	})

	id, err := g.Exchange(context.Background(), "code", "https://redirect.example.com/cb")
	if err != nil {
		t.Fatalf("Exchange() error: %v", err)
	}
	if id.Email != "test@example.com" {
		t.Errorf("Email = %q, want test@example.com", id.Email)
	}

	// Verify all three endpoint paths were hit on the fake server.
	for _, path := range []string{"/login/oauth/access_token", "/user", "/user/emails"} {
		if hitPaths[path] == 0 {
			t.Errorf("expected path %q to be hit on fake server, but it was not", path)
		}
	}

	if recorder.escapedToReal {
		t.Error("at least one request escaped to a non-fake host (real github.com or api.github.com)")
	}
}

// recordingTransport is an http.RoundTripper that records request paths and
// flags any request that doesn't target the expected fake host.
type recordingTransport struct {
	wrapped       http.RoundTripper
	fakeHost      string // expected base, e.g. "http://127.0.0.1:PORT"
	hitPaths      map[string]int
	escapedToReal bool
	t             *testing.T
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.t.Helper()
	actual := req.URL.Scheme + "://" + req.URL.Host
	if actual != r.fakeHost {
		r.t.Errorf("request escaped to non-fake host: %s%s (expected host %s)", actual, req.URL.Path, r.fakeHost)
		r.escapedToReal = true
	}
	r.hitPaths[req.URL.Path]++
	return r.wrapped.RoundTrip(req)
}

// isExchangeErr is a helper that checks for *oauth.ErrExchange in the error
// chain and sets *target if found.
func isExchangeErr(err error, target **oauth.ErrExchange) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*oauth.ErrExchange); ok {
		*target = e
		return true
	}
	return false
}

// TestGitHub_DefaultHTTPClientHasTimeout verifies that the GitHub provider
// injects a timeout when no HTTPClient is supplied. This guards against
// regressions where a future refactor silently re-introduces http.DefaultClient
// (which has Timeout == 0, enabling indefinite hangs on slow OAuth providers).
func TestGitHub_DefaultHTTPClientHasTimeout(t *testing.T) {
	g := oauth.NewGitHub(oauth.GitHubOptions{
		ClientID:     "id",
		ClientSecret: "secret",
		// Intentionally no HTTPClient override — exercises the default path.
	})

	client := g.HTTPClientForTest()
	if client.Timeout <= 0 {
		t.Errorf("default HTTPClient.Timeout = %v; want > 0 (no timeout means indefinite hang on slow OAuth providers)", client.Timeout)
	}
	const maxSensibleTimeout = 30 * time.Second
	if client.Timeout > maxSensibleTimeout {
		t.Errorf("default HTTPClient.Timeout = %v; want <= %v (too generous — goroutine pileup risk)", client.Timeout, maxSensibleTimeout)
	}
}

// TestGitHub_Exchange_TimesOutOnSlowProvider verifies that Exchange returns an
// error when the OAuth provider is slower than the configured HTTP client
// timeout. The test uses a short 100ms test-only timeout against a server that
// sleeps 500ms, so it completes well under a second of wall-clock time.
func TestGitHub_Exchange_TimesOutOnSlowProvider(t *testing.T) {
	// Slow token endpoint — sleeps longer than the client timeout below.
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "should-never-reach"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Use a custom HTTP client with a 100ms timeout — well under the 500ms
	// server sleep, so the timeout fires first.
	g := oauth.NewGitHub(oauth.GitHubOptions{
		ClientID:     "id",
		ClientSecret: "secret",
		BaseURL:      srv.URL,
		HTTPClient:   &http.Client{Timeout: 100 * time.Millisecond},
	})

	start := time.Now()
	_, err := g.Exchange(context.Background(), "chaos-code", "https://example.com/cb")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Exchange() should have returned an error due to client timeout, got nil")
	}
	// Verify it did not wait for the server sleep to finish (would be ~500ms).
	if elapsed > 400*time.Millisecond {
		t.Errorf("Exchange() took %v — expected to abort at ~100ms timeout, not wait for the 500ms server sleep", elapsed)
	}
	// The error should mention timeout or deadline.
	msg := err.Error()
	if !strings.Contains(msg, "timeout") && !strings.Contains(msg, "deadline") && !strings.Contains(msg, "Timeout") {
		t.Logf("Exchange() error = %v (does not mention timeout/deadline — acceptable if the transport wraps it)", err)
	}
}
