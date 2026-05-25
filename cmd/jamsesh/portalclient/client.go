// Package portalclient provides a thin HTTP client for the portal REST API.
// It attaches Bearer tokens automatically and handles 401 responses by
// invoking a single-flight refresh helper before retrying once.
package portalclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"jamsesh/cmd/jamsesh/state"
)

// maxErrBodyBytes caps the number of response bytes included in error
// messages on non-2xx responses. Portal error envelopes top out at ~200B
// (`{"error":"...","message":"..."}`); 512B covers them comfortably while
// preventing megabyte-scale provider blobs from ever appearing in local
// stderr/logs. (gate-security-refresh-error-body-leak)
const maxErrBodyBytes = 512

// truncatedBody reads at most maxErrBodyBytes from r and returns the content
// as a string. Used at every error-path body read to bound diagnostic memory
// and log exposure. The caller is responsible for closing the underlying
// reader (typically via `defer resp.Body.Close()`).
func truncatedBody(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, maxErrBodyBytes))
	return string(b)
}

// Client is a thin REST client for the portal API. It reads the current
// access token from local state on every request so token updates written
// by Refresher.Refresh are picked up automatically.
type Client struct {
	// BaseURL is the portal origin, e.g. "https://jamsesh.example.com".
	// Trailing slash is allowed; path segments are appended with "/".
	BaseURL string
	// SessionID is the jamsesh session this client is scoped to. When set,
	// attachBearer reads the per-session token first (via state.ReadCurrentBearer)
	// and falls back to the legacy account-wide token only if none is found.
	// Leave empty for pre-binding or account-level calls (legacy path only).
	SessionID string
	// HTTP is the underlying transport. If nil, http.DefaultClient is used.
	HTTP *http.Client
	// Refresh is called on a 401 response before the single retry. Typically
	// set to (*Refresher).Refresh. If nil, 401 errors are returned immediately
	// without a retry.
	Refresh func(ctx context.Context) error
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// Do executes req with an Authorization: Bearer header attached. On a 401
// response it calls c.Refresh (if set) and retries the request once with the
// freshly written token. If the retry also returns 401 the error is returned.
//
// The caller must not reuse req after Do returns because the body may have
// been consumed.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Attach the current token.
	if err := c.attachBearer(req); err != nil {
		return nil, err
	}

	resp, err := c.httpClient().Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized || c.Refresh == nil {
		return resp, nil
	}

	// 401 — drain and close the first response, then try to refresh.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if refreshErr := c.Refresh(ctx); refreshErr != nil {
		return nil, fmt.Errorf("portalclient: token refresh failed: %w", refreshErr)
	}

	// Clone the request so we can attach the new token and re-send.
	retryReq, err := cloneRequest(req)
	if err != nil {
		return nil, fmt.Errorf("portalclient: cloning request for retry: %w", err)
	}
	if err := c.attachBearer(retryReq); err != nil {
		return nil, err
	}

	resp2, err := c.httpClient().Do(retryReq.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if resp2.StatusCode == http.StatusUnauthorized {
		_, _ = io.Copy(io.Discard, resp2.Body)
		_ = resp2.Body.Close()
		return nil, fmt.Errorf("portalclient: still unauthorized after token refresh")
	}
	return resp2, nil
}

// attachBearer reads the current access token from local state and sets the
// Authorization header on req. This is called fresh on each request so that
// a token written by Refresher.Refresh is used for the retry.
//
// When c.SessionID is non-empty, state.ReadCurrentBearer checks the per-session
// token store first and falls back to the legacy account-wide file only if no
// per-session token is present. When c.SessionID is empty the legacy path is used
// directly (correct for pre-binding and account-level calls).
func (c *Client) attachBearer(req *http.Request) error {
	tok, err := state.ReadCurrentBearer(c.SessionID)
	if err != nil {
		return fmt.Errorf("portalclient: reading access token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return nil
}

// cloneRequest creates a shallow copy of req suitable for a single retry.
// If the original request carried a body it must have already been drained
// (as is the case in Do); the clone therefore carries no body.
func cloneRequest(orig *http.Request) (*http.Request, error) {
	clone := orig.Clone(orig.Context())
	clone.Body = nil
	clone.ContentLength = 0
	return clone, nil
}

// GetJSON issues a GET to <c.BaseURL><path>, runs it through Do, decodes
// the response body as JSON into T, and returns it. Non-2xx status codes
// after any refresh retry are returned as errors.
func GetJSON[T any](ctx context.Context, c *Client, path string) (T, error) {
	var zero T
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return zero, fmt.Errorf("portalclient: building GET request for %q: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.Do(ctx, req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("portalclient: GET %q returned %d: %s", path, resp.StatusCode, truncatedBody(resp.Body))
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return zero, fmt.Errorf("portalclient: decoding GET %q response: %w", path, err)
	}
	return result, nil
}

// GetJSONWithBearer issues a GET to baseURL+path with an explicit Authorization
// header (Authorization: Bearer <bearer>). Unlike GetJSON it does NOT go through
// the Client's refresh-retry machinery — the caller supplies the bearer directly.
// This is used for per-session status fetches where the token is already in hand
// and refresh may not be applicable (e.g., playground anonymous tokens).
func GetJSONWithBearer[T any](ctx context.Context, httpClient *http.Client, baseURL, path, bearer string) (T, error) {
	var zero T
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return zero, fmt.Errorf("portalclient: building GET request for %q: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)

	hc := httpClient
	if hc == nil {
		hc = http.DefaultClient
	}

	resp, err := hc.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("portalclient: GET %q returned %d: %s", path, resp.StatusCode, truncatedBody(resp.Body))
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return zero, fmt.Errorf("portalclient: decoding GET %q response: %w", path, err)
	}
	return result, nil
}

// PostJSONAnon marshals body to JSON, issues an unauthenticated POST to
// baseURL+path, decodes the response body as JSON into T, and returns it.
// Unlike PostJSON it does NOT attach any Authorization header — suitable
// for public/anonymous endpoints such as POST /api/playground/sessions.
// httpClient may be nil; http.DefaultClient is used in that case.
func PostJSONAnon[T any](ctx context.Context, httpClient *http.Client, baseURL, path string, body any) (T, error) {
	var zero T

	encoded, err := json.Marshal(body)
	if err != nil {
		return zero, fmt.Errorf("portalclient: encoding POST body for %q: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return zero, fmt.Errorf("portalclient: building POST request for %q: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	hc := httpClient
	if hc == nil {
		hc = http.DefaultClient
	}

	resp, err := hc.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("portalclient: POST %q returned %d: %s", path, resp.StatusCode, truncatedBody(resp.Body))
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return zero, fmt.Errorf("portalclient: decoding POST %q response: %w", path, err)
	}
	return result, nil
}

// PostJSON marshals body to JSON, issues a POST to <c.BaseURL><path> through
// Do, decodes the response body as JSON into T, and returns it.
func PostJSON[T any](ctx context.Context, c *Client, path string, body any) (T, error) {
	var zero T

	encoded, err := json.Marshal(body)
	if err != nil {
		return zero, fmt.Errorf("portalclient: encoding POST body for %q: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return zero, fmt.Errorf("portalclient: building POST request for %q: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.Do(ctx, req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("portalclient: POST %q returned %d: %s", path, resp.StatusCode, truncatedBody(resp.Body))
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return zero, fmt.Errorf("portalclient: decoding POST %q response: %w", path, err)
	}
	return result, nil
}
