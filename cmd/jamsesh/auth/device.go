package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"jamsesh/cmd/jamsesh/state"
)

// deviceAuthResponse is the JSON payload from the device authorization
// endpoint (RFC 8628 §3.2).
type deviceAuthResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"` // seconds; default 5 if absent
}

// deviceTokenResponse is the JSON payload returned by the token polling
// endpoint (RFC 8628 §3.5). On error the response carries an "error" field
// rather than access_token.
type deviceTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	// Error fields (RFC 6749 §5.2 / RFC 8628 §3.5).
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

const (
	errAuthorizationPending = "authorization_pending"
	errSlowDown             = "slow_down"
	errExpiredToken         = "expired_token"
	errAccessDenied         = "access_denied"

	defaultPollingInterval = 5 * time.Second
	slowDownIncrement      = 5 * time.Second
)

// deviceFlow executes the RFC 8628 device authorization grant. It:
//
//  1. POSTs to /api/auth/device/authorize to obtain a device_code and
//     user_code.
//  2. Prints instructions for the user to visit the verification URI.
//  3. Polls /api/auth/token at the prescribed interval until the user
//     completes authorization, the code expires, or ctx is cancelled.
//  4. On success, writes access + refresh tokens to local state.
//
// sleep is injectable so tests can use a fake clock without real delays.
//
// NOTE: The portal endpoints (/api/auth/device/authorize and
// /api/auth/token) do not exist yet — they land in
// epic-portal-foundation-auth-flows. End-to-end use will fail with 404
// until that feature ships. Tests use httptest.NewServer mock portals.
func deviceFlow(ctx context.Context, portalURL string, sleep func(time.Duration)) error {
	if sleep == nil {
		sleep = time.Sleep
	}

	authResp, err := requestDeviceAuthorization(ctx, portalURL)
	if err != nil {
		return err
	}

	interval := defaultPollingInterval
	if authResp.Interval > 0 {
		interval = time.Duration(authResp.Interval) * time.Second
	}
	expiresAt := time.Now().Add(time.Duration(authResp.ExpiresIn) * time.Second)

	fmt.Printf("\nDevice authorization required.\n")
	fmt.Printf("  Visit: %s\n", authResp.VerificationURI)
	fmt.Printf("  Enter code: %s\n\n", authResp.UserCode)
	fmt.Println("Waiting for authorization...")

	for {
		if time.Now().After(expiresAt) {
			return errors.New("device code expired before authorization was granted")
		}

		// Respect interval before each poll (including the first — RFC 8628 §3.5
		// says the client MUST wait at least `interval` seconds between requests).
		sleep(interval)

		tok, err := pollDeviceToken(ctx, portalURL, authResp.DeviceCode)
		if err != nil {
			return err
		}

		switch tok.Error {
		case "":
			// Success path.
			if tok.AccessToken == "" {
				return errors.New("device token: portal returned empty access_token")
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

		case errAuthorizationPending:
			// User hasn't completed the flow yet; keep polling.
			continue

		case errSlowDown:
			// Server requests slower polling; add 5 s per RFC 8628 §3.5.
			interval += slowDownIncrement
			continue

		case errExpiredToken:
			return errors.New("device code expired — please run `jamsesh auth --device-code` again")

		case errAccessDenied:
			return errors.New("device authorization denied by user or server")

		default:
			desc := tok.ErrorDescription
			if desc == "" {
				desc = tok.Error
			}
			return fmt.Errorf("device token error: %s", desc)
		}
	}
}

// requestDeviceAuthorization POSTs to the portal's device authorization
// endpoint and returns the parsed response.
func requestDeviceAuthorization(ctx context.Context, portalURL string) (deviceAuthResponse, error) {
	endpoint := strings.TrimRight(portalURL, "/") + "/api/auth/device/authorize"
	form := url.Values{}
	form.Set("client_id", "jamsesh-cli")
	form.Set("scope", "openid profile")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return deviceAuthResponse{}, fmt.Errorf("building device authorization request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return deviceAuthResponse{}, fmt.Errorf("device authorization request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return deviceAuthResponse{}, fmt.Errorf("reading device authorization response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return deviceAuthResponse{}, fmt.Errorf("device authorization: portal returned %d: %s", resp.StatusCode, body)
	}

	var authResp deviceAuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return deviceAuthResponse{}, fmt.Errorf("parsing device authorization response: %w", err)
	}
	if authResp.DeviceCode == "" {
		return deviceAuthResponse{}, errors.New("device authorization: portal returned empty device_code")
	}
	return authResp, nil
}

// pollDeviceToken POSTs to the portal's token endpoint with the device_code
// grant and returns the raw response (including error fields on pending/
// slow-down/expired states).
func pollDeviceToken(ctx context.Context, portalURL, deviceCode string) (deviceTokenResponse, error) {
	endpoint := strings.TrimRight(portalURL, "/") + "/api/auth/token"
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", "jamsesh-cli")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return deviceTokenResponse{}, fmt.Errorf("building device token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return deviceTokenResponse{}, fmt.Errorf("device token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return deviceTokenResponse{}, fmt.Errorf("reading device token response: %w", err)
	}

	// RFC 8628 §3.5: pending/slow-down/expired errors arrive as 4xx with a
	// JSON error body. We parse the body regardless of status code and let the
	// caller inspect the .Error field.
	var tok deviceTokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return deviceTokenResponse{}, fmt.Errorf("parsing device token response (HTTP %d): %w", resp.StatusCode, err)
	}
	return tok, nil
}
