// Shared container-startup concurrency limiter for the fuzz e2e suite.
//
// Why this exists: the manifest and fencing fuzz harnesses run their seeds as
// parallel sub-tests (t.Parallel). Each seed cold-starts TWO full portal
// containers (a bootstrap cluster then a hot cluster) on top of the shared
// Postgres/MinIO/MailHog stack. Under `go test`'s default parallelism
// (GOMAXPROCS, e.g. 16 on CI hardware) this fans out enough simultaneous
// container boots to saturate the Docker daemon / host: a subset of portals
// log "portal starting" but never finish booting within the 30s /healthz
// readiness deadline, and the cluster fixture reports "pod 0 is nil after
// startup". Healthy boots complete in ~1s, so the timeouts are pure resource
// contention, not slow startup.
//
// The fix is to bound how many portal cold-starts run at once. We gate the
// per-seed work behind a counting semaphore so the suite never has more than
// fuzzStartupConcurrency seeds racing for container resources simultaneously.
// This keeps the harnesses deterministic without serialising them entirely.
//
// Override the cap with FUZZ_STARTUP_CONCURRENCY (e.g. on a beefy host) — a
// value <= 0 falls back to the default.
package fuzz_test

import (
	"os"
	"strconv"
	"sync"
	"testing"
)

// fuzzStartupConcurrencyDefault is the default number of seeds permitted to
// cold-start portal containers concurrently. Each seed boots up to two portal
// containers against a shared Postgres/MinIO/MailHog stack and runs DB
// migrations at boot. These heavyweight clustered cold-starts are highly
// contention-sensitive: empirically a cap of 4 produced frequent >90s readiness
// stalls and a cap of 2 still produced sporadic ones, while serialised
// cold-starts complete reliably in ~1s. The default is therefore 1 — the seeds
// still run as parallel sub-tests (their setup/assertion work overlaps), but the
// container cold-start critical section is serialised across the whole package.
//
// On a host with ample Docker headroom, raise this via FUZZ_STARTUP_CONCURRENCY
// to trade reliability for wall-clock throughput.
const fuzzStartupConcurrencyDefault = 1

var (
	startupSemOnce sync.Once
	startupSem     chan struct{}
)

// startupSemaphore returns the process-wide container-startup semaphore, sized
// from FUZZ_STARTUP_CONCURRENCY or the default. It is shared across every fuzz
// harness in this package so their parallel sub-tests cooperate on one budget.
func startupSemaphore() chan struct{} {
	startupSemOnce.Do(func() {
		n := fuzzStartupConcurrencyDefault
		if v := os.Getenv("FUZZ_STARTUP_CONCURRENCY"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
				n = parsed
			}
		}
		startupSem = make(chan struct{}, n)
	})
	return startupSem
}

// acquireStartupSlot blocks until a container-startup slot is free, then holds
// it until the test (and all its registered cleanups) finish. Call it once at
// the top of a seed body, before starting any cluster:
//
//	func runSeed(t *testing.T, ...) {
//	    acquireStartupSlot(t)
//	    cluster := portalcluster.Start(ctx, t, ...)
//	    ...
//	}
//
// The slot is released via t.Cleanup, registered here BEFORE the cluster
// fixtures register their own teardown cleanups. Because t.Cleanup runs in LIFO
// order, this release runs LAST — after every container this seed started has
// been torn down. That serialises the expensive work (cold-start boot AND
// teardown) across the package: a new seed never boots a portal while a previous
// seed's containers are still being removed, which is the I/O contention that
// otherwise stalls boots past the readiness deadline.
func acquireStartupSlot(t *testing.T) {
	t.Helper()
	sem := startupSemaphore()
	sem <- struct{}{}
	t.Cleanup(func() { <-sem })
}
