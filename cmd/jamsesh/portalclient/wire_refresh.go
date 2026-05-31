package portalclient

// WireRefresh attaches a Refresher to client.Refresh so that 401 responses
// from the portal trigger the singleflight token-refresh path. The Refresher
// reads the refresh token from local state and writes the new tokens back; it
// inherits the client's SessionID so the refreshed access token is persisted to
// the SAME session the client's requests use (sessions/<id>/token), rather than
// a session inferred from the ambient CC-instance binding.
//
// Call this immediately after constructing a Client at any production call
// site that interacts with authenticated portal endpoints.
func WireRefresh(client *Client) {
	r := &Refresher{BaseURL: client.BaseURL, HTTP: client.HTTP, SessionID: client.SessionID}
	client.Refresh = r.Refresh
}
