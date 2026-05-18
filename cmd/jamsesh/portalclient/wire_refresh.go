package portalclient

// WireRefresh attaches a Refresher to client.Refresh so that 401 responses
// from the portal trigger the singleflight token-refresh path. The Refresher
// reads the refresh token from, and writes the new access/refresh tokens to,
// local state (via state.Read/WriteRefreshToken and state.WriteToken) — no
// additional parameters are required.
//
// Call this immediately after constructing a Client at any production call
// site that interacts with authenticated portal endpoints.
func WireRefresh(client *Client) {
	r := &Refresher{BaseURL: client.BaseURL, HTTP: client.HTTP}
	client.Refresh = r.Refresh
}
