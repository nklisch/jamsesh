// Package metrics provides a Prometheus metrics registry shared between the
// portal server and the jamsesh-router binary. It exposes typed metric handles
// for HTTP traffic, git push outcomes, auto-merger results, event-log
// throughput, and router routing decisions. Callers increment metrics directly
// via the exported handles; the Registry never inspects request/response
// internals itself.
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

	// ── Router metrics ────────────────────────────────────────────────────────
	// These fields are populated only when the Registry is used by the
	// jamsesh-router binary. Portal instances leave them nil (they are not
	// registered in a portal-only New call). Use nil checks before incrementing,
	// or prefer the nil-safe handler fields on proxy.Handler and readyz.Probe.

	// RouterDecisionsTotal counts routing decisions per request. Label "result"
	// is one of: hit_cache, hit_ring, fallback, empty_ring, retry, error_503.
	RouterDecisionsTotal *prometheus.CounterVec

	// RouterRingSize tracks the current number of pods in the consistent-hash
	// ring. Updated on every ring rebalance via the discovery publish callback.
	RouterRingSize prometheus.Gauge

	// RouterRingRebalancesTotal counts how many times ring.SetPods has been
	// called (i.e., the healthy pod set has changed). Increments even when the
	// new set is empty.
	RouterRingRebalancesTotal prometheus.Counter

	// RouterProbeFailuresTotal counts readiness-probe failures, labeled by the
	// pod address that failed. Cardinality is bounded by the pod count (≤ 20).
	RouterProbeFailuresTotal *prometheus.CounterVec

	// ── Lease metrics ─────────────────────────────────────────────────────────
	// These fields track distributed session lease acquisition, hold duration,
	// and loss events. They are populated only in clustered mode; portal
	// instances running in single mode (NoopManager) do not increment them.

	// LeaseAcquiresTotal counts lease acquisition attempts. Label "result" is
	// one of: ok (acquired), conflict (ErrAlreadyHeld), error (unexpected error).
	LeaseAcquiresTotal *prometheus.CounterVec

	// LeaseHoldsCurrently tracks the current number of active leases held by
	// this pod. Incremented on acquire; decremented on release.
	LeaseHoldsCurrently prometheus.Gauge

	// LeaseHoldDurationSeconds measures the duration a lease was held (observed
	// at Release). Uses default buckets to cover short (sub-second) through
	// long (minutes) hold times.
	LeaseHoldDurationSeconds prometheus.Histogram

	// LeaseLostTotal counts leases lost due to a heartbeat failure or PG
	// session drop. A non-zero value indicates a transient connectivity issue
	// or an overloaded PG server.
	LeaseLostTotal prometheus.Counter

	// LeaseFencingTokensIssuedTotal counts successfully issued fencing tokens
	// (one per successful Acquire). Equals LeaseAcquiresTotal{result="ok"};
	// provided as a standalone counter for simpler alerting rules.
	LeaseFencingTokensIssuedTotal prometheus.Counter

	reg *prometheus.Registry
}

// New creates and registers all portal and router metrics into an isolated
// Prometheus registry. Standard Go runtime and process collectors are included
// so that go_goroutines, go_memstats_*, and process_cpu_seconds_total are
// present at /metrics without additional configuration.
//
// Both the portal server and the jamsesh-router binary call New() and obtain
// the same Registry type. Router-specific fields (RouterDecisionsTotal, etc.)
// are registered by New() unconditionally; the portal simply never increments
// them, so they appear at zero in portal /metrics output.
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

	// ── Router metrics ────────────────────────────────────────────────────────

	routerDecisionsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "jamsesh_router_decisions_total",
		Help: "Total number of routing decisions made by the router, labeled by result " +
			"(hit_cache, hit_ring, fallback, empty_ring, retry, error_503).",
	}, []string{"result"})
	reg.MustRegister(routerDecisionsTotal)

	routerRingSize := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "jamsesh_router_ring_size",
		Help: "Current number of pods in the consistent-hash ring.",
	})
	reg.MustRegister(routerRingSize)

	routerRingRebalancesTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "jamsesh_router_ring_rebalances_total",
		Help: "Total number of ring rebalances (SetPods calls) triggered by discovery.",
	})
	reg.MustRegister(routerRingRebalancesTotal)

	routerProbeFailuresTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "jamsesh_router_probe_failures_total",
		Help: "Total number of readiness-probe failures, labeled by pod address. " +
			"Cardinality is bounded by the pod count (typically ≤ 20).",
	}, []string{"addr"})
	reg.MustRegister(routerProbeFailuresTotal)

	// ── Lease metrics ─────────────────────────────────────────────────────────

	leaseAcquiresTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "jamsesh_lease_acquires_total",
		Help: "Total number of session lease acquisition attempts, labeled by result " +
			"(ok, conflict, error).",
	}, []string{"result"})
	reg.MustRegister(leaseAcquiresTotal)

	leaseHoldsCurrently := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "jamsesh_lease_holds_currently",
		Help: "Current number of active session leases held by this pod.",
	})
	reg.MustRegister(leaseHoldsCurrently)

	leaseHoldDurationSeconds := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "jamsesh_lease_hold_duration_seconds",
		Help:    "Duration (in seconds) a session lease was held, observed at Release.",
		Buckets: prometheus.DefBuckets,
	})
	reg.MustRegister(leaseHoldDurationSeconds)

	leaseLostTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "jamsesh_lease_lost_total",
		Help: "Total number of session leases lost due to a heartbeat failure or PG session drop.",
	})
	reg.MustRegister(leaseLostTotal)

	leaseFencingTokensIssuedTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "jamsesh_lease_fencing_tokens_issued_total",
		Help: "Total number of fencing tokens successfully issued (one per successful Acquire).",
	})
	reg.MustRegister(leaseFencingTokensIssuedTotal)

	return &Registry{
		HTTPRequestsTotal:             httpRequestsTotal,
		HTTPRequestDuration:           httpRequestDuration,
		GitPushesTotal:                gitPushesTotal,
		AutoMergerOutcomes:            autoMergerOutcomes,
		EventLogEmitTotal:             eventLogEmitTotal,
		RouterDecisionsTotal:          routerDecisionsTotal,
		RouterRingSize:                routerRingSize,
		RouterRingRebalancesTotal:     routerRingRebalancesTotal,
		RouterProbeFailuresTotal:      routerProbeFailuresTotal,
		LeaseAcquiresTotal:            leaseAcquiresTotal,
		LeaseHoldsCurrently:           leaseHoldsCurrently,
		LeaseHoldDurationSeconds:      leaseHoldDurationSeconds,
		LeaseLostTotal:                leaseLostTotal,
		LeaseFencingTokensIssuedTotal: leaseFencingTokensIssuedTotal,
		reg:                           reg,
	}
}

// Handler returns an http.Handler that serves the Prometheus text exposition
// format at /metrics. The handler uses the registry's isolated collector set
// rather than the global default registry.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}
