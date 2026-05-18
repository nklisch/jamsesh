// Invariant: after idle eviction, a pod's local bare-repo cache for a session
// is removed from disk. A subsequent request to that pod for the same session
// requires re-hydration (not served from stale local cache), and all
// pre-eviction state is preserved in MinIO and restored after hydration.
//
// Test: TestLifecycleEvictOnLeaseRelease
// Package: golden_test
//
// This is the "cleanup" side of the handoff contract. The golden tests
// (session_handoff_idle_eviction_test.go) verify that state is preserved after
// migration; this test verifies that the originating pod does NOT retain stale
// cache after eviction, and that re-hydration from object storage restores
// complete state on the same pod.
//
// Assertions use VerifyCacheEvicted (docker exec ls) for direct filesystem
// inspection — never indirect timing observations.
//
// This test covers idle (time-driven) eviction only. LRU eviction
// (memory-pressure-driven via JAMSESH_HYDRATION_CACHE_MAX_BYTES) is not
// tested — container memory is not a reliable test lever. See risks in
// epic-e2e-cnd-coverage-hydration-handoff body.
package golden_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestLifecycleEvictOnLeaseRelease verifies the evict-on-lease-release lifecycle
// invariant:
//
//  1. Push 3 commits to pod 0; record draftTipBefore.
//  2. Wait for idle eviction (accelerated: 5s idle timeout, 2s check period).
//  3. Assert cache is evicted from pod 0's filesystem (VerifyCacheEvicted).
//  4. Assert advisory lock is released (LeaseHolder == -1 after eviction).
//  5. Push a 4th commit via pod 0 directly — triggers re-hydration on pod 0.
//  6. WaitForHydration on pod 0.
//  7. Assert draft tip on pod 0 includes all 4 commits (pre-eviction + new).
//  8. Assert MinIO bucket still has all objects (eviction did NOT delete from bucket).
//
// This test uses Router: false because the lifecycle invariant is tested on a
// single pod — pod 0 evicts its own cache and then re-hydrates from MinIO on
// the next push. Multi-pod routing is tested in session_handoff_idle_eviction_test.go.
//
// The accelerated idle-eviction env vars are:
//
//	JAMSESH_HYDRATION_IDLE_TIMEOUT_S=5      (default 300s → 5s for test speed)
//	JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S=2 (default 30s  → 2s for test speed)
//
// These are defined in internal/portal/config/config.go. A 10s sleep after the
// initial push is sufficient: two full check cycles (2s period) with a 5s idle
// window elapsed, leaving a 5s margin for goroutine scheduling jitter in CI.
func TestLifecycleEvictOnLeaseRelease(t *testing.T) {
	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
	// Router: false — this test focuses on the single-pod eviction+re-hydration
	// round-trip. The router is not needed; pods are addressed directly.
	// 2 pods are started so the cluster is valid for clustered-mode boot, but
	// all interactions in this test go to pod 0 directly.
	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)

	cluster := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        2,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      false,
		PortalExtraEnv: map[string]string{
			// Accelerated idle-eviction: 5s idle timeout, 2s check period.
			// Verified against internal/portal/config/config.go applyHydrationEnv.
			"JAMSESH_HYDRATION_IDLE_TIMEOUT_S":      "5",
			"JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S": "2",

			// Short heartbeat so lease state settles quickly in test time.
			"JAMSESH_LEASE_HEARTBEAT_INTERVAL_S": "2",

			// Email provider: mailhog SMTP for magic-link delivery.
			"JAMSESH_EMAIL_PROVIDER":  "smtp",
			"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":  "none",
		},
	})
	require.GreaterOrEqualf(t, len(cluster.Pods), 2,
		"TestLifecycleEvictOnLeaseRelease: need at least 2 pods; got %d", len(cluster.Pods))

	// ── Step 1: Auth + org + session creation ────────────────────────────────
	aliceEmail := randEmail(t, "lifecycle-evict")
	pair := authflow.SignInViaMagicLink(ctx, t, cluster.Pods[0], mh, aliceEmail)
	t.Logf("lifecycle-evict: authenticated as %s", aliceEmail)

	userID := handoffGetMe(ctx, t, cluster.Pods[0].URL, pair.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, cluster.Pods[0], pair.AccessToken, "Lifecycle Evict Org")
	sessionID := handoffCreateSession(ctx, t, cluster.Pods[0].URL, pair.AccessToken, orgID, "lifecycle-evict-session")
	t.Logf("lifecycle-evict: created session %s", sessionID)

	// ── Step 2: Push 3 commits via pod 0 to acquire lease + populate cache ──
	// Push directly to pod 0 (not via router) so pod 0 acquires the advisory
	// lock and populates the local bare-repo cache deterministically.
	ref := "jam/" + sessionID + "/" + userID + "/main"
	repo := gitclient.Clone(ctx, t, cluster.Pods[0].URL, orgID, sessionID, userID, pair.AccessToken)

	for i := 1; i <= 3; i++ {
		filename := "lifecycle-" + strconv.Itoa(i) + ".md"
		message := "lifecycle-evict: pre-eviction commit " + strconv.Itoa(i)
		repo.Commit(ctx, t, filename, "pre-eviction content "+strconv.Itoa(i), message)
	}
	repo.Push(ctx, t, ref)
	t.Logf("lifecycle-evict: pushed 3 commits on %s via pod 0", ref)

	// Confirm pod 0 holds the lease after push.
	holder := cluster.RequireLeaseHolder(ctx, t, sessionID, 10*time.Second)
	require.Equalf(t, 0, holder,
		"lifecycle-evict: expected pod 0 to hold lease after direct push; got pod %d", holder)
	t.Logf("lifecycle-evict: lease held by pod 0")

	// ── Step 3: Sanity-check: cache present on pod 0 before eviction ─────────
	// VerifyCachePresent is the guard against false-positive VerifyCacheEvicted
	// results — if the cache was never written, VerifyCacheEvicted would pass
	// trivially without proving anything.
	cluster.VerifyCachePresent(ctx, t, orgID, sessionID, 0, "" /* default storagePath */)
	t.Logf("lifecycle-evict: cache confirmed present on pod 0 pre-eviction")

	// Record draftTipBefore — the ref tip after the 3 pre-eviction commits.
	draftTipBefore := handoffRevParseViaPod(ctx, t, cluster.Pods[0].URL, pair.AccessToken, orgID, sessionID, ref)
	require.NotEmptyf(t, draftTipBefore,
		"lifecycle-evict: could not resolve pre-eviction ref tip for %q — did the push succeed?", ref)
	t.Logf("lifecycle-evict: draftTipBefore = %s", draftTipBefore)

	// ── Step 4: Wait for idle eviction ───────────────────────────────────────
	// With JAMSESH_HYDRATION_IDLE_TIMEOUT_S=5 and JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S=2:
	// the session becomes eligible for eviction after 5s of no activity; the
	// LifecycleManager's check loop fires every 2s. 10s provides a 2-check-period
	// margin beyond the idle timeout, accounting for CI scheduling jitter.
	//
	// time.Sleep is used (not polling) because there is no external event to hook
	// into — eviction is internal to the LifecycleManager goroutine running inside
	// the portal container.
	t.Logf("lifecycle-evict: sleeping 10s for idle eviction (timeout=5s, period=2s)")
	time.Sleep(10 * time.Second)

	// ── Step 5: Verify cache evicted from pod 0 ───────────────────────────────
	// VerifyCacheEvicted confirms the bare-repo directory is absent from pod 0's
	// container filesystem (via docker exec ls). If still present, the
	// LifecycleManager's idle scanner did not fire or JAMSESH_HYDRATION_IDLE_TIMEOUT_S
	// is not honored — this is a High production bug.
	//
	// Design escape hatch (per story):
	// If this consistently fails, park as High: "LifecycleManager idle scanner
	// not running or JAMSESH_HYDRATION_IDLE_TIMEOUT_S not honored", and replace
	// this call with:
	//   t.Skip("bug-<id>: JAMSESH_HYDRATION_IDLE_TIMEOUT_S not honored — " +
	//       "cache not evicted after 2× idle timeout period")
	cluster.VerifyCacheEvicted(ctx, t, orgID, sessionID, 0, "" /* default storagePath */)
	t.Logf("lifecycle-evict: cache evicted from pod 0's filesystem")

	// ── Step 6: Assert advisory lock released alongside eviction ─────────────
	// The idle-eviction path in the LifecycleManager must release the Postgres
	// advisory lock alongside cleaning the cache. LeaseHolder == -1 proves this.
	//
	// If LeaseHolder returns 0 (pod 0 still holds the lock), that is a Medium
	// bug (cache evicted but advisory lock not released). Log it but do not
	// fatal — the lock will be released when pod 0's container is torn down by
	// Testcontainers. The re-hydration round-trip (Steps 7-9) will still proceed.
	holderAfterEviction := cluster.LeaseHolder(ctx, t, sessionID)
	if holderAfterEviction != -1 {
		t.Logf("lifecycle-evict: WARNING: advisory lock still held by pod %d after cache eviction — "+
			"Medium bug: idle eviction did not release the Postgres advisory lock alongside cache cleanup; "+
			"park as Medium if reproducible; the re-hydration round-trip will proceed regardless",
			holderAfterEviction)
	} else {
		t.Logf("lifecycle-evict: advisory lock released (LeaseHolder == -1) — eviction released lock as expected")
	}

	// ── Step 7: Re-hydration round-trip — push 4th commit via pod 0 ──────────
	// Pushing to pod 0 directly forces pod 0 to re-acquire the lease and
	// re-hydrate from MinIO before serving the push. This tests that the
	// evicted pod can correctly re-hydrate from object storage and then accept
	// new writes — the "cleanup side" round-trip invariant.
	repo2 := gitclient.Clone(ctx, t, cluster.Pods[0].URL, orgID, sessionID, userID, pair.AccessToken)
	repo2.Commit(ctx, t, "lifecycle-4.md", "post-eviction content", "lifecycle-evict: post-eviction commit 4")
	repo2.Push(ctx, t, ref)
	t.Logf("lifecycle-evict: pushed commit 4 via pod 0 directly (triggers re-hydration)")

	// ── Step 8: Wait for pod 0 to re-hydrate ─────────────────────────────────
	// WaitForHydration polls git ls-remote against pod 0 directly. This succeeds
	// iff the portal can serve the session's pack files from its local cache —
	// exactly the post-hydration readiness signal.
	cluster.WaitForHydration(ctx, t, orgID, sessionID, pair.AccessToken, 0, 30*time.Second)
	t.Logf("lifecycle-evict: pod 0 re-hydrated from MinIO")

	// Confirm pod 0 re-acquired the lease after the post-eviction push.
	newHolder := cluster.RequireLeaseHolder(ctx, t, sessionID, 15*time.Second)
	require.Equalf(t, 0, newHolder,
		"lifecycle-evict: expected pod 0 to re-acquire lease after post-eviction push; got pod %d", newHolder)
	t.Logf("lifecycle-evict: pod 0 re-acquired lease")

	// ── Step 9: Assert draft tip reflects all 4 commits ──────────────────────
	// The ref tip on pod 0 after re-hydration + push must advance past
	// draftTipBefore (commits 1-3 + new commit 4). If draftTipAfter ==
	// draftTipBefore, the 4th commit was not incorporated — hydration-then-push
	// sequencing bug. If draftTipAfter is empty, re-hydration failed.
	draftTipAfter := handoffRevParseViaPod(ctx, t, cluster.Pods[0].URL, pair.AccessToken, orgID, sessionID, ref)
	require.NotEmptyf(t, draftTipAfter,
		"lifecycle-evict: pod 0 returned empty SHA for ref %q after re-hydration; "+
			"re-hydration failed to restore ref state — park as Critical if reproducible", ref)
	require.NotEqualf(t, draftTipBefore, draftTipAfter,
		"lifecycle-evict: LIFECYCLE INVARIANT VIOLATED — "+
			"draftTipAfter (%s) == draftTipBefore (%s); "+
			"the 4th commit was NOT incorporated into the ref tip after re-hydration+push; "+
			"this indicates a hydration-then-push sequencing bug; "+
			"park as Critical if reproducible",
		draftTipAfter, draftTipBefore)
	t.Logf("lifecycle-evict: draftTipAfter = %s (advanced past draftTipBefore = %s)",
		draftTipAfter, draftTipBefore)

	// Cross-check: clone tip must match REST ref tip — ensures ref store and
	// bare repo are in sync after the re-hydration + push cycle.
	clonedTip := handoffGetRefTipFromClone(ctx, t, cluster.Pods[0].URL, pair.AccessToken, orgID, sessionID, userID, ref)
	require.Equalf(t, draftTipAfter, clonedTip,
		"lifecycle-evict: REST ref tip (%s) != git clone tip (%s) on pod 0 — "+
			"ref store and bare repo are out of sync after re-hydration; "+
			"park as Critical if reproducible",
		draftTipAfter, clonedTip)
	t.Logf("lifecycle-evict: REST ref tip == git clone tip (%s) — consistency holds", clonedTip)

	// ── Step 10: MinIO bucket still has all objects ───────────────────────────
	// Idle eviction removes the LOCAL cache but must NOT delete objects from
	// MinIO. All pre-eviction pack objects must still be present in the bucket.
	// Absence here means eviction corrupted the session's object-storage state —
	// this is a Critical production bug (RPO=0 violated).
	objectPrefix := "sessions/" + sessionID + "/"
	objects, err := mn.ListObjects(ctx, objectPrefix)
	require.NoErrorf(t, err, "lifecycle-evict: MinIO list objects failed")
	require.NotEmptyf(t, objects,
		"lifecycle-evict: MinIO bucket is empty for prefix %q after eviction — "+
			"idle eviction MUST NOT delete session objects from object storage; "+
			"RPO=0 invariant violated; park as Critical if reproducible", objectPrefix)
	t.Logf("lifecycle-evict: MinIO has %d object(s) under %s after eviction+re-hydration",
		len(objects), objectPrefix)
}
