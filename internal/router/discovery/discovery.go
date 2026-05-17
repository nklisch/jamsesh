// Package discovery provides pod-set discovery implementations that feed the
// consistent-hash ring. Two modes are available:
//
//   - Static: reads a configured list of addresses, probes each via /readyz,
//     and publishes the healthy subset on every probe interval. Suitable for
//     VM, Docker Compose, and bare-metal clusters.
//
//   - Kubernetes: watches pods backing a named Service via the k8s client-go
//     informer, probes /readyz, and publishes the healthy subset. Suitable
//     for Kubernetes clusters.
//
// The Discoverer interface is the contract between these implementations and
// the ring.Ring that manages pod membership.
package discovery

import (
	"context"
	"time"

	"jamsesh/internal/router/readyz"
	"jamsesh/internal/router/ring"
)

// Discoverer publishes the current set of healthy pods to a sink at intervals.
type Discoverer interface {
	// Run blocks until ctx is cancelled. On each discovery + probe pass it
	// calls publish with the current healthy pod subset. Run returns
	// ctx.Err() on cancellation and may return a non-nil error for fatal
	// initialisation failures (e.g. unable to create a k8s informer).
	Run(ctx context.Context, publish func([]ring.Pod)) error
}

// Static returns a Discoverer that polls the configured addresses, probes
// /readyz on each, and publishes the healthy subset. The addr strings must be
// in "host:port" form. Pod IDs are set to the address itself (stable and
// deterministic for static config).
//
// The first probe pass runs immediately when Run is called; subsequent passes
// run every interval. publish is called only when the healthy set changes to
// avoid spurious ring rebalances.
func Static(addrs []string, probe *readyz.Probe, interval time.Duration) Discoverer {
	return &staticDiscoverer{
		addrs:    addrs,
		probe:    probe,
		interval: interval,
	}
}

// Kubernetes returns a Discoverer that watches pods backing serviceName in
// namespace via the k8s client-go informer, probes /readyz, and publishes the
// healthy subset. Pod ID is set to the pod name; pod address is "<podIP>:8443".
//
// The interval controls how often probe results are re-evaluated after
// informer events stabilise; the informer itself is event-driven. Run blocks
// until ctx is cancelled.
func Kubernetes(namespace, serviceName string, probe *readyz.Probe, interval time.Duration) Discoverer {
	return &k8sDiscoverer{
		namespace:   namespace,
		serviceName: serviceName,
		probe:       probe,
		interval:    interval,
	}
}
