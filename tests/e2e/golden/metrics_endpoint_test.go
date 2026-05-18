// Invariant: the portal's /metrics endpoint emits valid Prometheus exposition
// format, parses cleanly via expfmt.TextParser, and contains at least one
// well-known metric family. This is the Prometheus scrape contract — a broken
// exporter means ops gets garbage scraping data, alerts misfire, and dashboards
// lie. The expfmt parse + family-presence check is the non-tautological core.
package golden_test

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

func TestMetricsEndpoint(t *testing.T) {
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

	resp, err := http.Get(p.URL + "/metrics") //nolint:noctx
	require.NoError(t, err)
	defer resp.Body.Close()

	// If /metrics is behind auth, fail loud so the implementer knows exactly
	// what happened — do NOT silently skip or weaken the assertion.
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("/metrics returned 401 Unauthorized (WWW-Authenticate: %q); "+
			"the endpoint must be unauthenticated for Prometheus scraping. "+
			"Add the required auth header or remove the auth guard.",
			resp.Header.Get("WWW-Authenticate"))
	}

	require.Equal(t, http.StatusOK, resp.StatusCode,
		"/metrics must return 200; got %d", resp.StatusCode)

	// Prometheus exposition format declares text/plain Content-Type.
	ct := resp.Header.Get("Content-Type")
	require.True(t, strings.HasPrefix(ct, "text/plain"),
		"Content-Type %q must start with text/plain (Prometheus exposition format)", ct)

	// Parse with the Prometheus text parser — this is the non-tautological
	// assertion. A 200 with a malformed body still means a broken exporter.
	var parser expfmt.TextParser
	families, err := parser.TextToMetricFamilies(resp.Body)
	require.NoError(t, err, "Prometheus exposition format must parse without error; "+
		"a parse error means the exporter emits malformed output that Prometheus "+
		"would silently discard")
	require.NotEmpty(t, families,
		"/metrics must expose at least one metric family; got an empty map")

	// Spot-check: the portal registers Go runtime collectors (NewGoCollector),
	// so go_goroutines is always present. If it's somehow absent, fall back to
	// checking for any well-known prefix — but we still fail if nothing
	// recognisable is present, because an empty or entirely-unrecognised map
	// means the exporter is misconfigured.
	if _, ok := families["go_goroutines"]; !ok {
		recognised := false
		for name := range families {
			if strings.HasPrefix(name, "go_") ||
				strings.HasPrefix(name, "process_") ||
				strings.HasPrefix(name, "http_") ||
				strings.HasPrefix(name, "jamsesh_") {
				recognised = true
				break
			}
		}
		require.True(t, recognised,
			"expected at least one well-known metric family "+
				"(go_*, process_*, http_*, or jamsesh_*); "+
				"got families: %v", familyNames(families))
	}
}

// familyNames returns the sorted metric family names from the parsed map.
// Used in failure messages so the reader sees a deterministic list rather
// than an arbitrary map iteration order.
func familyNames(m map[string]*dto.MetricFamily) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
