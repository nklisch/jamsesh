// Package metrics provides a Prometheus metrics registry for the portal server.
// It exposes typed metric handles for HTTP traffic, git push outcomes, auto-merger
// results, and event-log throughput. Callers increment metrics directly via the
// exported handles; the Registry never inspects request/response internals itself.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds typed Prometheus metric handles for the portal server. Construct
// once via [New] and share across components. All exported fields are safe for
// concurrent use.
type Registry struct {
	// HTTPRequestsTotal counts completed HTTP requests with method, route pattern,
	// and HTTP status code labels. Route is a chi route pattern (e.g.
	// "/api/orgs/{orgID}/sessions/{sessionID}"), not a raw URL, to prevent
	// cardinality explosion.
	HTTPRequestsTotal *prometheus.CounterVec

	// HTTPRequestDuration measures HTTP request latency in seconds, labeled by
	// method and route pattern.
	HTTPRequestDuration *prometheus.HistogramVec

	// GitPushesTotal counts completed git push operations. Label "result" is
	// one of: "ok" (accepted by pre-receive and persisted) or "rejected"
	// (pre-receive policy rejection).
	GitPushesTotal *prometheus.CounterVec

	// AutoMergerOutcomes counts auto-merger apply outcomes. Label "outcome" is
	// one of: "succeeded" (clean merge or safe auto-resolve), "conflict" (hard
	// conflict written to DB), "backpressure" (worker dropped due to queue full).
	AutoMergerOutcomes *prometheus.CounterVec

	// EventLogEmitTotal counts every individual event emitted to the event log,
	// including both Emit and EmitBatch calls (batch counts each event separately).
	EventLogEmitTotal prometheus.Counter

	reg *prometheus.Registry
}

// New creates and registers all portal metrics into an isolated Prometheus
// registry. Standard Go runtime and process collectors are included so that
// go_goroutines, go_memstats_*, and process_cpu_seconds_total are present at
// /metrics without additional configuration.
func New() *Registry {
	reg := prometheus.NewRegistry()

	// Standard collectors: Go runtime + process metrics.
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	httpRequestsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests completed, labeled by method, route pattern, and status code.",
	}, []string{"method", "route", "status"})
	reg.MustRegister(httpRequestsTotal)

	// Histogram buckets spanning 5ms–10s to cover typical portal latency range.
	httpRequestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds, labeled by method and route pattern.",
		Buckets: prometheus.ExponentialBuckets(0.005, 2, 12), // 5ms → ~10s
	}, []string{"method", "route"})
	reg.MustRegister(httpRequestDuration)

	gitPushesTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "jamsesh_git_pushes_total",
		Help: "Total number of git push operations, labeled by result (ok or rejected).",
	}, []string{"result"})
	reg.MustRegister(gitPushesTotal)

	autoMergerOutcomes := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "jamsesh_automerger_outcomes_total",
		Help: "Total number of auto-merger apply outcomes, labeled by outcome (succeeded, conflict, backpressure).",
	}, []string{"outcome"})
	reg.MustRegister(autoMergerOutcomes)

	eventLogEmitTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "jamsesh_event_log_emit_total",
		Help: "Total number of individual events emitted to the event log.",
	})
	reg.MustRegister(eventLogEmitTotal)

	return &Registry{
		HTTPRequestsTotal:   httpRequestsTotal,
		HTTPRequestDuration: httpRequestDuration,
		GitPushesTotal:      gitPushesTotal,
		AutoMergerOutcomes:  autoMergerOutcomes,
		EventLogEmitTotal:   eventLogEmitTotal,
		reg:                 reg,
	}
}

// Handler returns an http.Handler that serves the Prometheus text exposition
// format at /metrics. The handler uses the registry's isolated collector set
// rather than the global default registry.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}
