// Invariant: when the full stack is healthy (Postgres up, email configured),
// /readyz returns 200 with Content-Type application/json; charset=utf-8 and a
// JSON body where status is "ready" and every named check reports ok: true.
// This is the K8s/Cloud Run readiness contract — a lying test here means
// production traffic gets routed to a broken pod.
package golden_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

func TestReadyzHealthy(t *testing.T) {
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)

	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		SMTPHost:  mh.ContainerSMTPHost,
		SMTPPort:  mh.ContainerSMTPPort,
	})

	resp, err := http.Get(p.URL + "/readyz") //nolint:noctx
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode,
		"/readyz on a healthy portal must return 200")
	require.Equal(t, "application/json; charset=utf-8", resp.Header.Get("Content-Type"),
		"/readyz must declare application/json; charset=utf-8 content-type")

	var body struct {
		Status string `json:"status"`
		Checks []struct {
			Name  string  `json:"name"`
			OK    bool    `json:"ok"`
			Error *string `json:"error,omitempty"`
		} `json:"checks"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body),
		"/readyz body must be valid JSON matching the probe envelope")

	require.Equal(t, "ready", body.Status,
		"/readyz body.status must be \"ready\" when all checks pass")
	require.NotEmpty(t, body.Checks,
		"/readyz must declare at least one named check")
	for _, c := range body.Checks {
		require.Truef(t, c.OK,
			"check %q must report ok:true on a healthy stack; error: %v", c.Name, c.Error)
	}
}
