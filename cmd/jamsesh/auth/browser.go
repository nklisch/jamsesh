package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"jamsesh/cmd/jamsesh/state"
)

// tokenResponse is the JSON shape returned by the portal's code-exchange
// endpoint. Fields are populated when the portal's auth-flows feature ships
// (epic-portal-foundation-auth-flows); the shape matches standard OAuth 2.0.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// browserFlow executes the local-listener OAuth 2.0 authorization code flow
// with PKCE (S256). It:
//
//  1. Generates a PKCE pair and state nonce.
//  2. Starts a one-shot HTTP listener on an ephemeral loopback port.
//  3. Builds the portal authorization URL and opens it via openURL.
//  4. Waits for the callback; validates the state parameter.
//  5. Exchanges the code at the portal's token endpoint.
//  6. Writes access + refresh tokens to local state.
//
// openURL is injectable so tests can avoid launching a real browser.
//
// NOTE: The portal endpoints called here (/api/auth/oauth/github/start and
// the token exchange at /api/auth/code) do not exist yet — they land in
// epic-portal-foundation-auth-flows. End-to-end use against a real portal
// will fail with 404 until that feature ships. Tests exercise the binary's
// HTTP call shapes using mock portals via httptest.NewServer.
func browserFlow(ctx context.Context, portalURL string, openURL func(string) error) error {
	pkce, err := GeneratePKCE()
	if err != nil {
		return err
	}
	stateVal, err := GenerateState()
	if err != nil {
		return err
	}

	// Bind to an ephemeral loopback port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting local OAuth listener: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/cb", port)

	// Build the authorization URL.
	authURL, err := buildAuthURL(portalURL, pkce, stateVal, redirectURI)
	if err != nil {
		ln.Close()
		return err
	}

	// callbackResult carries either the authorization code or an error from
	// the callback handler.
	type callbackResult struct {
		code string
		err  error
	}
	resultCh := make(chan callbackResult, 1)

	srv := &http.Server{
		ReadHeaderTimeout: 30 * time.Second,
	}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cb" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		gotState := q.Get("state")
		if gotState != stateVal {
			http.Error(w, "state mismatch — possible CSRF", http.StatusBadRequest)
			resultCh <- callbackResult{err: errors.New("OAuth callback: state parameter mismatch (possible CSRF)")}
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing code parameter", http.StatusBadRequest)
			resultCh <- callbackResult{err: errors.New("OAuth callback: missing code parameter")}
			return
		}
		fmt.Fprintln(w, "Authentication successful — you may close this tab.")
		resultCh <- callbackResult{code: code}
	})

	// Serve in the background; shut down once we have a result.
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	// Open the browser (or let the test intercept).
	fmt.Printf("Opening browser for authentication...\nURL: %s\n", authURL)
	if err := openURL(authURL); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}

	// Wait for the callback or context cancellation.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-resultCh:
		if res.err != nil {
			return res.err
		}
		return exchangeCode(ctx, portalURL, res.code, pkce.Verifier, redirectURI)
	}
}

// buildAuthURL constructs the portal authorization URL with all required
// OAuth + PKCE parameters.
func buildAuthURL(portalURL string, pkce PKCEPair, stateVal, redirectURI string) (string, error) {
	base, err := url.Parse(strings.TrimRight(portalURL, "/") + "/api/auth/oauth/github/start")
	if err != nil {
		return "", fmt.Errorf("parsing portal URL: %w", err)
	}
	q := base.Query()
	q.Set("response_type", "code")
	q.Set("code_challenge", pkce.Challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", stateVal)
	q.Set("redirect_uri", redirectURI)
	base.RawQuery = q.Encode()
	return base.String(), nil
}

// exchangeCode sends the authorization code to the portal's token endpoint
// and writes the resulting tokens to local state.
//
// Endpoint: POST /api/auth/code
// NOTE: This endpoint does not exist yet (lands in epic-portal-foundation-auth-flows).
func exchangeCode(ctx context.Context, portalURL, code, verifier, redirectURI string) error {
	endpoint := strings.TrimRight(portalURL, "/") + "/api/auth/code"
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("building token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("token exchange request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("reading token exchange response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token exchange: portal returned %d: %s", resp.StatusCode, body)
	}

	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return fmt.Errorf("parsing token exchange response: %w", err)
	}
	if tok.AccessToken == "" {
		return errors.New("token exchange: portal returned empty access_token")
	}

	if err := state.WriteToken(tok.AccessToken); err != nil {
		return fmt.Errorf("writing access token: %w", err)
	}
	if tok.RefreshToken != "" {
		if err := state.WriteRefreshToken(tok.RefreshToken); err != nil {
			return fmt.Errorf("writing refresh token: %w", err)
		}
	}

	fmt.Println("Authentication successful. Tokens written to local state.")
	return nil
}
