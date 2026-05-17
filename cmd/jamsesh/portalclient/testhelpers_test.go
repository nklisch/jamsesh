package portalclient

import (
	"encoding/json"
	"net/http"
)

// parseJSON is a test-only helper that decodes r.Body into v.
func parseJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
