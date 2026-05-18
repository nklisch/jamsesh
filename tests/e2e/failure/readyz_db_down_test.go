// Invariant: when the Postgres connection is severed (simulated via a
// Toxiproxy reset_peer toxic), /readyz returns 503 with status "not_ready"
// and at least one check reporting ok: false within 3 seconds (2s per-check
// timeout + 1s margin). This is the K8s/Cloud Run failure contract — /readyz
// returning 200 when the DB is unreachable would keep broken pods in rotation.
package failure_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/toxiproxy"
)

func TestReadyzDBDown(t *testing.T) {
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})
	tp := toxiproxy.Start(ctx, t)

	// Create a Toxiproxy proxy: toxiproxy-container → postgres-container.
	// The portal will connect to Postgres through this proxy so we can inject
	// faults without disturbing other tests sharing the shared postgres container.
	const (
		proxyName   = "pg-readyz-test"
		proxyListen = "0.0.0.0:5433"
		toxicName   = "pg_reset"
	)

	// pg.ContainerDSN is postgres://test:test@<bridge-ip>:5432/<dbname>...
	// Extract the bridge IP so Toxiproxy (inside Docker) can reach Postgres.
	pgContainerHost := readyzExtractHost(pg.ContainerDSN)
	toxiproxyCreateProxy(ctx, t, tp.AdminURL, proxyName,
		proxyListen,
		fmt.Sprintf("%s:5432", pgContainerHost))

	// Wait briefly for the proxy port to be ready before starting the portal.
	time.Sleep(200 * time.Millisecond)

	// Portal connects to Postgres via the Toxiproxy container.
	// tp.ContainerIP is the Docker bridge IP reachable from the portal container.
	proxiedDSN := fmt.Sprintf("postgres://test:test@%s:5433/%s?sslmode=disable",
		tp.ContainerIP,
		readyzExtractDBName(pg.ContainerDSN))

	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     proxiedDSN,
		EmailFrom: "noreply@example.com",
		// No SMTP wiring: the readyz DB check does not depend on mail delivery,
		// and we are exercising only the DB probe path here.
	})

	// Sanity-check: /readyz must be healthy before injecting the fault.
	// The portal fixture already waits for /healthz, but assert /readyz
	// explicitly so a pre-existing probe regression surfaces here, not in the
	// 503-assertion below.
	preResp, err := http.Get(p.URL + "/readyz") //nolint:noctx
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, preResp.StatusCode,
		"/readyz must be healthy before toxiproxy fault is injected")
	preResp.Body.Close()

	// Inject reset_peer toxic: all new TCP connections to the proxy are reset
	// immediately. Existing pgxpool connections fail on next query attempt.
	toxiproxyAddToxic(ctx, t, tp.AdminURL, proxyName, toxicName, "reset_peer",
		map[string]any{"timeout": 0})

	// Within ~3s (/readyz 2s per-check timeout + 1s margin) the probe must
	// report 503. Don't extend this to mask flakiness: the contract is that
	// the probe detects DB failure within its documented timeout.
	require.Eventually(t, func() bool {
		r, err := http.Get(p.URL + "/readyz") //nolint:noctx
		if err != nil {
			return false
		}
		defer r.Body.Close()
		return r.StatusCode == http.StatusServiceUnavailable
	}, 3*time.Second, 200*time.Millisecond,
		"/readyz must return 503 within 3s when Postgres is unreachable via toxiproxy reset_peer")

	// Assert on body shape — status code alone is not sufficient. The body is
	// what Prometheus, alerting, and ops dashboards parse; a 503 with a
	// malformed or incorrect body is a silent ops-tooling failure.
	r, err := http.Get(p.URL + "/readyz") //nolint:noctx
	require.NoError(t, err)
	defer r.Body.Close()

	require.Equal(t, http.StatusServiceUnavailable, r.StatusCode,
		"/readyz must return 503 when DB is unreachable")

	var body struct {
		Status string `json:"status"`
		Checks []struct {
			Name  string  `json:"name"`
			OK    bool    `json:"ok"`
			Error *string `json:"error,omitempty"`
		} `json:"checks"`
	}
	require.NoError(t, json.NewDecoder(r.Body).Decode(&body),
		"/readyz body must be valid JSON even when unhealthy")

	require.Equal(t, "not_ready", body.Status,
		"/readyz body.status must be \"not_ready\" when at least one check fails")
	require.NotEmpty(t, body.Checks,
		"/readyz must declare at least one named check even when unhealthy")

	anyFailed := false
	for _, c := range body.Checks {
		if !c.OK {
			anyFailed = true
			break
		}
	}
	require.True(t, anyFailed,
		"at least one check must report ok:false when Postgres is unreachable")
}

// ---------------------------------------------------------------------------
// DSN parsing helpers — prefixed to avoid collision with config_and_deps_test.go
// which defines extractHostFromDSN/extractDBName in the same package.
// ---------------------------------------------------------------------------

// readyzExtractHost extracts the hostname (without port) from a postgres DSN.
// e.g. "postgres://test:test@172.17.0.3:5432/testdb?sslmode=disable" → "172.17.0.3"
func readyzExtractHost(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "localhost"
	}
	return u.Hostname()
}

// readyzExtractDBName extracts the database name from a postgres DSN.
// e.g. "postgres://test:test@host:5432/testdb?sslmode=disable" → "testdb"
func readyzExtractDBName(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "test"
	}
	return strings.TrimPrefix(u.Path, "/")
}
