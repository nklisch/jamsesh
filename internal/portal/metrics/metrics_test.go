package metrics_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/portal/metrics"
)

// scrapeMetrics hits the /metrics handler and returns the response body.
func scrapeMetrics(t *testing.T, reg *metrics.Registry) string {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	reg.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics handler returned %d", w.Code)
	}
	return w.Body.String()
}

// assertContains fails the test if the scraped output does not contain all of
// the expected substrings.
func assertContains(t *testing.T, body string, fragments ...string) {
	t.Helper()
	for _, f := range fragments {
		if !strings.Contains(body, f) {
			t.Errorf("metrics output missing %q\n--- output ---\n%s", f, body)
		}
	}
}

func TestHandlerReturnsPrometheusFormat(t *testing.T) {
	reg := metrics.New()
	body := scrapeMetrics(t, reg)

	// Prometheus text format always starts with a HELP or TYPE comment, or a
	// metric line. The content-type must be text/plain.
	if body == "" {
		t.Fatal("expected non-empty metrics output")
	}
	if !strings.Contains(body, "# HELP") {
		t.Errorf("expected Prometheus text format (# HELP comments), got:\n%s", body)
	}
}

func TestStandardGoRuntimeMetricsPresent(t *testing.T) {
	reg := metrics.New()
	body := scrapeMetrics(t, reg)

	assertContains(t,
		body,
		"go_goroutines",
		"go_memstats_",
		"process_cpu_seconds_total",
	)
}

func TestHTTPRequestsCounterIncrements(t *testing.T) {
	reg := metrics.New()

	// Simulate a request by incrementing the counter directly as the middleware would.
	reg.HTTPRequestsTotal.WithLabelValues("GET", "/healthz", "200").Inc()
	reg.HTTPRequestsTotal.WithLabelValues("POST", "/api/auth/refresh", "401").Inc()
	reg.HTTPRequestsTotal.WithLabelValues("POST", "/api/auth/refresh", "401").Inc()

	body := scrapeMetrics(t, reg)
	assertContains(t,
		body,
		`http_requests_total{method="GET",route="/healthz",status="200"} 1`,
		`http_requests_total{method="POST",route="/api/auth/refresh",status="401"} 2`,
	)
}

func TestHTTPRequestDurationHistogramEmits(t *testing.T) {
	reg := metrics.New()

	reg.HTTPRequestDuration.WithLabelValues("GET", "/healthz").Observe(0.005)

	body := scrapeMetrics(t, reg)
	assertContains(t,
		body,
		`http_request_duration_seconds_bucket{method="GET",route="/healthz"`,
		`http_request_duration_seconds_count{method="GET",route="/healthz"} 1`,
	)
}

func TestRoutePatternLabelUsesChiPattern(t *testing.T) {
	// This test verifies the cardinality-safety guarantee: route labels MUST
	// be chi route patterns, not raw URLs. A parameterised route like
	// /api/orgs/{orgID}/sessions/{sessionID} must appear as that pattern in
	// the metric label, regardless of the actual URL values.
	reg := metrics.New()

	// Wire up a minimal chi router with the pattern we want to test.
	r := chi.NewRouter()
	r.Get("/api/orgs/{orgID}/sessions/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		// Simulate what the Access middleware does: read the route pattern after
		// routing is complete, then record the metric.
		rctx := chi.RouteContext(r.Context())
		pattern := rctx.RoutePattern()
		if pattern == "" {
			pattern = "unknown"
		}
		reg.HTTPRequestsTotal.WithLabelValues(r.Method, pattern, "200").Inc()
		w.WriteHeader(http.StatusOK)
	})

	// Issue a request with concrete URL parameters.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/orgs/org-abc/sessions/sess-xyz", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	body := scrapeMetrics(t, reg)

	// The label must contain the route pattern, NOT the raw URL values.
	wantPattern := `/api/orgs/{orgID}/sessions/{sessionID}`
	wantMetric := fmt.Sprintf(`http_requests_total{method="GET",route="%s",status="200"} 1`, wantPattern)
	if !strings.Contains(body, wantMetric) {
		t.Errorf("expected route pattern label %q in metric output\n--- output ---\n%s", wantPattern, body)
	}

	// Ensure raw URL values do NOT appear as route labels.
	badLabel := `route="org-abc"`
	if strings.Contains(body, badLabel) {
		t.Errorf("raw URL value appeared as route label (cardinality violation): %q", badLabel)
	}
}

func TestGitPushesTotalIncrements(t *testing.T) {
	reg := metrics.New()

	reg.GitPushesTotal.WithLabelValues("ok").Inc()
	reg.GitPushesTotal.WithLabelValues("ok").Inc()
	reg.GitPushesTotal.WithLabelValues("rejected").Inc()

	body := scrapeMetrics(t, reg)
	assertContains(t,
		body,
		`jamsesh_git_pushes_total{result="ok"} 2`,
		`jamsesh_git_pushes_total{result="rejected"} 1`,
	)
}

func TestAutoMergerOutcomesIncrements(t *testing.T) {
	reg := metrics.New()

	reg.AutoMergerOutcomes.WithLabelValues("succeeded").Inc()
	reg.AutoMergerOutcomes.WithLabelValues("conflict").Inc()
	reg.AutoMergerOutcomes.WithLabelValues("backpressure").Inc()

	body := scrapeMetrics(t, reg)
	assertContains(t,
		body,
		`jamsesh_automerger_outcomes_total{outcome="succeeded"} 1`,
		`jamsesh_automerger_outcomes_total{outcome="conflict"} 1`,
		`jamsesh_automerger_outcomes_total{outcome="backpressure"} 1`,
	)
}

func TestEventLogEmitTotalIncrements(t *testing.T) {
	reg := metrics.New()

	reg.EventLogEmitTotal.Add(5)

	body := scrapeMetrics(t, reg)
	assertContains(t,
		body,
		"jamsesh_event_log_emit_total 5",
	)
}

func TestMetricsHandlerRequiresNoAuthentication(t *testing.T) {
	// The /metrics handler must be reachable without any auth headers.
	// This test verifies the handler does not inspect Authorization headers.
	reg := metrics.New()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	// Deliberately omit Authorization header.
	reg.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("want 200 (no auth required), got %d", w.Code)
	}
}
