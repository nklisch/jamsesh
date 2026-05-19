// Invariant: when JAMSESH_METRICS_TOKEN is set, /metrics requires a valid
// "Authorization: Bearer <token>" header. Missing or incorrect tokens must
// receive 401. The correct token must receive 200 with valid Prometheus
// exposition format that parses cleanly via expfmt.TextParser and contains at
// least one well-known metric family.
//
// When JAMSESH_METRICS_TOKEN is not set, /metrics is not mounted at all (404).
// This test covers only the token-set path — the 404 case is exercised by the
// unit tests in internal/portal/router/metrics_auth_test.go.
package golden_test

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// prometheus/common v0.66+ requires NameValidationScheme to be set before
// expfmt.TextParser can run; an "unset" scheme panics with
// "Invalid name validation scheme requested: unset". Set the legacy scheme
// here so the parser accepts the standard metric/label names the portal emits.
func init() {
	model.NameValidationScheme = model.LegacyValidation
}

func TestMetricsEndpoint(t *testing.T) {
	const metricsToken = "e2e-metrics-test-token"
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)

	// Start the portal with JAMSESH_METRICS_TOKEN set so the /metrics route
	// is mounted and gated behind bearer-token auth.
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		SMTPHost:  mh.ContainerSMTPHost,
		SMTPPort:  mh.ContainerSMTPPort,
		ExtraEnv: map[string]string{
			"JAMSESH_METRICS_TOKEN": metricsToken,
		},
	})

	t.Run("no_auth_header_is_401", func(t *testing.T) {
		resp, err := http.Get(p.URL + "/metrics") //nolint:noctx
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"/metrics without auth must return 401; got %d", resp.StatusCode)
	})

	t.Run("wrong_bearer_is_401", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/metrics", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer wrong-token")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"/metrics with wrong bearer must return 401; got %d", resp.StatusCode)
	})

	t.Run("correct_bearer_is_200_with_prometheus_output", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/metrics", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+metricsToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode,
			"/metrics with correct bearer must return 200; got %d", resp.StatusCode)

		// Prometheus exposition format declares text/plain Content-Type.
		ct := resp.Header.Get("Content-Type")
		require.True(t, strings.HasPrefix(ct, "text/plain"),
			"Content-Type %q must start with text/plain (Prometheus exposition format)", ct)

		// Parse with the Prometheus text parser — this is the non-tautological
		// assertion. A 200 with a malformed body still means a broken exporter.
		// NewTextParser takes the validation scheme explicitly; the zero-value
		// TextParser has scheme=UnsetValidation which panics inside
		// setOrCreateCurrentMF (prometheus/common v0.66+). Use LegacyValidation
		// to accept the standard names the portal emits.
		parser := expfmt.NewTextParser(model.LegacyValidation)
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
	})
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
