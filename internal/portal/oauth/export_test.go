package oauth

import "net/http"

// HTTPClientForTest exposes the unexported httpClient method for white-box
// testing of the default-timeout path. Only compiled into test binaries.
func (g *GitHub) HTTPClientForTest() *http.Client {
	return g.httpClient()
}
