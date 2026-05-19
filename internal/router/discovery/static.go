package discovery

import (
	"context"
	"sort"
	"strings"
	"time"

	"jamsesh/internal/router/readyz"
	"jamsesh/internal/router/ring"
)

// staticDiscoverer implements Discoverer for a fixed list of pod addresses.
type staticDiscoverer struct {
	addrs    []string
	probe    *readyz.Probe
	interval time.Duration
}

// neverPublished is a sentinel value for prev that cannot match any real key.
// Using a string that cannot be produced by joinSorted (which joins with ",")
// avoids a separate boolean and keeps the comparison simple.
const neverPublished = "\x00"

// Run blocks until ctx is cancelled. It waits for the first tick interval
// before probing, then probes again on every subsequent tick. publish is called
// only when the healthy set changes from the previously published set.
//
// The pre-tick delay is intentional: at startup the ring is already seeded with
// all configured pod addresses by the caller (cmd/jamsesh-router/main.go). If
// we probed immediately, a readyz probe arriving before portals finish their
// Postgres-ping + os.Stat readiness checks would return zero healthy pods and
// publish([]) — evicting the seeded ring and causing all in-flight requests to
// 503. Waiting one full interval gives portals time to become genuinely ready
// before any probe can clear the ring.
func (d *staticDiscoverer) Run(ctx context.Context, publish func([]ring.Pod)) error {
	prev := neverPublished // sentinel: "no previous publish"

	doProbe := func() {
		healthy := d.probe.Check(ctx, d.addrs)
		key := joinSorted(healthy)
		if key == prev {
			return // no change — skip spurious ring rebalance
		}
		prev = key
		pods := addrsToPods(healthy)
		publish(pods)
	}

	// Wait for the first tick before probing. The ring is already seeded at
	// startup; an immediate probe risks clearing it before portals are ready.
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			doProbe()
		}
	}
}

// addrsToPods converts a slice of "host:port" addresses to ring.Pod values.
// The Pod ID is set to the address itself, which is stable and deterministic
// for static configuration.
func addrsToPods(addrs []string) []ring.Pod {
	pods := make([]ring.Pod, len(addrs))
	for i, a := range addrs {
		pods[i] = ring.Pod{
			ID:      a,
			Address: a,
		}
	}
	return pods
}

// joinSorted returns a canonical string representation of an address set for
// change detection. It sorts the slice in-place (a copy) and joins with ",".
func joinSorted(addrs []string) string {
	if len(addrs) == 0 {
		return ""
	}
	cp := make([]string, len(addrs))
	copy(cp, addrs)
	sort.Strings(cp)
	return strings.Join(cp, ",")
}
