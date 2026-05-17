package portalclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/sync/singleflight"

	"jamsesh/cmd/jamsesh/state"
)

// tokenPair matches the portal's TokenPair response schema returned by
// POST /api/auth/refresh.
type tokenPair struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

// Refresher fetches new access and refresh tokens from the portal and writes
// them to local state. Concurrent Refresh calls are collapsed into a single
// in-flight HTTP POST via singleflight so at most one network round-trip is
// made regardless of how many goroutines encountered a 401 simultaneously.
type Refresher struct {
	// BaseURL is the portal origin, e.g. "https://jamsesh.example.com".
	BaseURL string
	// HTTP is the underlying transport. If nil, http.DefaultClient is used.
	HTTP *http.Client

	group singleflight.Group
}

func (r *Refresher) httpClient() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return http.DefaultClient
}

// Refresh fetches fresh tokens from the portal using the locally stored
// refresh token, then writes the new access and refresh tokens to local state
// atomically.
//
// Concurrent callers share the same in-flight POST; all waiters receive the
// same error (nil on success). This is safe to call from multiple goroutines.
func (r *Refresher) Refresh(ctx context.Context) error {
	_, err, _ := r.group.Do("refresh", func() (any, error) {
		return nil, r.doRefresh(ctx)
	})
	return err
}

// doRefresh is the actual single-instance implementation called by the
// singleflight group. It must not be called directly from concurrent code.
func (r *Refresher) doRefresh(ctx context.Context) error {
	refreshToken, err := state.ReadRefreshToken()
	if err != nil {
		return fmt.Errorf("portalclient: reading refresh token: %w", err)
	}

	payload, err := json.Marshal(map[string]string{"refresh_token": refreshToken})
	if err != nil {
		return fmt.Errorf("portalclient: encoding refresh request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.BaseURL+"/api/auth/refresh", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("portalclient: building refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("portalclient: refresh POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("portalclient: refresh returned %d: %s", resp.StatusCode, body)
	}

	var pair tokenPair
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		return fmt.Errorf("portalclient: decoding refresh response: %w", err)
	}

	if pair.AccessToken == "" {
		return fmt.Errorf("portalclient: refresh response missing access_token")
	}
	if pair.RefreshToken == "" {
		return fmt.Errorf("portalclient: refresh response missing refresh_token")
	}

	// Write new tokens atomically. Write the refresh token first so that if
	// the process is interrupted between the two writes, the access token file
	// (which is read by the headers helper) still contains the old value rather
	// than an orphaned new token with no matching refresh token.
	if err := state.WriteRefreshToken(pair.RefreshToken); err != nil {
		return fmt.Errorf("portalclient: writing new refresh token: %w", err)
	}
	if err := state.WriteToken(pair.AccessToken); err != nil {
		return fmt.Errorf("portalclient: writing new access token: %w", err)
	}

	return nil
}
