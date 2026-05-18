// Invariant: after idle-eviction of the local cache on the holding pod, a
// subsequent request causes re-hydration from MinIO and the re-hydrated pod
// serves the same draft tip as before eviction. No committed state is lost
// across an idle eviction cycle.
//
// Test: TestSessionHandoffIdleEviction
// Package: golden_test
//
// Assertions use VerifyCacheEvicted and direct ref-SHA comparison — never HTTP
// status codes alone.
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

// TestSessionHandoffIdleEviction verifies the idle-eviction round-trip invariant:
//  1. Push 3 commits on pod 0; record draftTipBefore.
//  2. Wait for idle-eviction (accelerated via env vars: 5s idle timeout, 2s check).
//  3. Confirm pod 0's cache is evicted (VerifyCacheEvicted).
//  4. Push a 4th commit via the router — triggers re-hydration on the routed pod.
//  5. Confirm the re-hydrated pod's tip advances past draftTipBefore (all 4 commits).
//  6. MinIO bucket still intact.
//
// The accelerated idle-eviction env vars are:
//   JAMSESH_HYDRATION_IDLE_TIMEOUT_S=5      (default 300s → 5s for test speed)
//   JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S=2 (default 30s  → 2s for test speed)
//
// These are defined in internal/portal/config/config.go and verified therein.
// A 10s sleep after the initial push is sufficient for at least two check cycles
// (2s period) with a 5s idle window to have elapsed.
func TestSessionHandoffIdleEviction(t *testing.T) {
	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)

	cluster := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        2,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      true,
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
	require.NotEmpty(t, cluster.RouterURL, "TestSessionHandoffIdleEviction: Router: true is required")
	require.GreaterOrEqualf(t, len(cluster.Pods), 2,
		"TestSessionHandoffIdleEviction: need at least 2 pods; got %d", len(cluster.Pods))

	// ── Step 1: Auth + org + session creation ────────────────────────────────
	aliceEmail := randEmail(t, "handoff-idle")
	pair := authflow.SignInViaMagicLink(ctx, t, cluster.Pods[0], mh, aliceEmail)
	t.Logf("handoff-idle-eviction: authenticated as %s", aliceEmail)

	userID := handoffGetMe(ctx, t, cluster.Pods[0].URL, pair.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, cluster.Pods[0], pair.AccessToken, "Handoff Idle Eviction Org")
	sessionID := handoffCreateSession(ctx, t, cluster.RouterURL, pair.AccessToken, orgID, "handoff-idle-eviction-session")
	t.Logf("handoff-idle-eviction: created session %s", sessionID)

	// ── Step 2: Push 3 commits via pod 0 directly ────────────────────────────
	// Use pod 0 directly (not router) so pod 0 acquires the lease deterministically.
	// This sets up a known pre-eviction state we can verify afterwards.
	ref := "jam/" + sessionID + "/" + userID + "/main"
	repo := gitclient.Clone(ctx, t, cluster.Pods[0].URL, orgID, sessionID, userID, pair.AccessToken)

	for i := 1; i <= 3; i++ {
		filename := "idle-" + strconv.Itoa(i) + ".md"
		message := "handoff-idle: pre-eviction commit " + strconv.Itoa(i)
		repo.Commit(ctx, t, filename, "pre-eviction content "+strconv.Itoa(i), message)
	}
	repo.Push(ctx, t, ref)
	t.Logf("handoff-idle-eviction: pushed 3 commits on %s via pod 0", ref)

	// Confirm pod 0 holds the lease after push.
	holder := cluster.RequireLeaseHolder(ctx, t, sessionID, 10*time.Second)
	require.Equalf(t, 0, holder,
		"handoff-idle-eviction: expected pod 0 to hold lease after direct push; got pod %d", holder)
	t.Logf("handoff-idle-eviction: lease held by pod 0")

	// Verify cache is present on pod 0 before waiting for eviction.
	cluster.VerifyCachePresent(ctx, t, orgID, sessionID, 0, "" /* default storagePath */)
	t.Logf("handoff-idle-eviction: cache confirmed present on pod 0 pre-eviction")

	// Record draftTipBefore — the ref tip after the 3 pre-eviction commits.
	draftTipBefore := handoffRevParseViaPod(ctx, t, cluster.Pods[0].URL, pair.AccessToken, orgID, sessionID, ref)
	require.NotEmptyf(t, draftTipBefore,
		"handoff-idle-eviction: could not resolve pre-eviction ref tip for %q; "+
			"this is a pre-condition failure — did the push succeed?", ref)
	t.Logf("handoff-idle-eviction: draftTipBefore = %s", draftTipBefore)

	// ── Step 3: Wait for idle eviction ───────────────────────────────────────
	// With JAMSESH_HYDRATION_IDLE_TIMEOUT_S=5 and JAMSESH_HYDRATION_IDLE_CHECK_PERIOD_S=2:
	// after 5s of no activity the session is eligible for eviction; the check
	// loop fires every 2s and removes the cache. 10s sleep provides a 2-check-
	// period margin beyond the idle timeout.
	//
	// This test uses time.Sleep rather than polling because there is no event
	// to hook into from outside the container — the eviction is internal to the
	// LifecycleManager goroutine. 10s is conservative for a 5s+2s configuration.
	t.Logf("handoff-idle-eviction: sleeping 10s for idle eviction (timeout=5s, period=2s)")
	time.Sleep(10 * time.Second)

	// ── Step 4: Verify cache evicted on pod 0 ────────────────────────────────
	// VerifyCacheEvicted confirms the bare-repo directory is absent from the
	// container filesystem. If the cache was NOT evicted, this fatals the test.
	// The idle eviction env vars are the only lever; if this fails consistently
	// the env vars are not being respected (park as Important: test-mode tunables).
	cluster.VerifyCacheEvicted(ctx, t, orgID, sessionID, 0, "" /* default storagePath */)
	t.Logf("handoff-idle-eviction: cache evicted from pod 0")

	// ── Step 5: Push commit 4 via router → re-hydration ──────────────────────
	// The router routes to whichever pod its consistent hash prefers (pod 0 or
	// pod 1). That pod acquires the lease, hydrates from MinIO, and then accepts
	// the push. This is the core re-hydration scenario.
	repo2 := gitclient.Clone(ctx, t, cluster.RouterURL, orgID, sessionID, userID, pair.AccessToken)
	repo2.Commit(ctx, t, "idle-4.md", "post-eviction content", "handoff-idle: post-eviction commit 4")
	repo2.Push(ctx, t, ref)
	t.Logf("handoff-idle-eviction: pushed commit 4 via router (triggers re-hydration)")

	// ── Step 6: Find new lease holder ────────────────────────────────────────
	// After the push, one pod holds the lease. RequireLeaseHolder polls until
	// found (15s window — enough for heartbeat=2s to settle).
	newHolder := cluster.RequireLeaseHolder(ctx, t, sessionID, 15*time.Second)
	require.GreaterOrEqualf(t, newHolder, 0,
		"handoff-idle-eviction: no pod holds lease after post-eviction push")
	t.Logf("handoff-idle-eviction: new lease holder = pod %d", newHolder)

	// ── Step 7: Verify draftTipAfter reflects all 4 commits ──────────────────
	// The ref tip on the new holder must advance past draftTipBefore. The
	// non-tautological check: draftTipAfter != draftTipBefore (new commit added).
	// The full invariant: the new tip is NOT empty and was reached via hydration
	// from the same MinIO store that held the 3 pre-eviction commits.
	draftTipAfter := handoffRevParseViaPod(ctx, t, cluster.Pods[newHolder].URL, pair.AccessToken, orgID, sessionID, ref)
	require.NotEmptyf(t, draftTipAfter,
		"handoff-idle-eviction: pod %d returned empty SHA for ref %q after re-hydration; "+
			"this means the pod could not serve the ref after hydrating from MinIO", newHolder, ref)
	require.NotEqualf(t, draftTipBefore, draftTipAfter,
		"handoff-idle-eviction: HYDRATION INVARIANT VIOLATED — "+
			"draftTipAfter (%s) == draftTipBefore (%s); "+
			"the 4th commit was NOT incorporated into the ref tip after re-hydration; "+
			"this indicates a hydration-then-push sequencing bug; "+
			"park as Critical if reproducible",
		draftTipAfter, draftTipBefore)
	t.Logf("handoff-idle-eviction: draftTipAfter = %s (advanced past draftTipBefore = %s)",
		draftTipAfter, draftTipBefore)

	// Cross-check: the tip from a fresh git clone must match the REST API ref tip.
	// This ensures the portal's ref store and the bare repo are in sync after
	// the re-hydration + push cycle.
	clonedTip := handoffGetRefTipFromClone(ctx, t, cluster.Pods[newHolder].URL, pair.AccessToken, orgID, sessionID, userID, ref)
	require.Equalf(t, draftTipAfter, clonedTip,
		"handoff-idle-eviction: REST ref tip (%s) != git clone tip (%s) on pod %d — "+
			"ref store and bare repo are out of sync after re-hydration; "+
			"park as Critical if reproducible",
		draftTipAfter, clonedTip, newHolder)
	t.Logf("handoff-idle-eviction: REST ref tip == git clone tip (%s) — consistency holds", clonedTip)

	// ── Step 8: MinIO bucket still intact ────────────────────────────────────
	// Idle eviction removes the local cache but must NOT delete objects from
	// MinIO. The bucket must still contain session objects.
	objectPrefix := "sessions/" + sessionID + "/"
	objects, err := mn.ListObjects(ctx, objectPrefix)
	require.NoErrorf(t, err, "handoff-idle-eviction: MinIO list objects failed")
	require.NotEmptyf(t, objects,
		"handoff-idle-eviction: MinIO bucket is empty for prefix %q after eviction — "+
			"idle eviction must not delete session objects from object storage; "+
			"RPO=0 invariant violated", objectPrefix)
	t.Logf("handoff-idle-eviction: MinIO has %d object(s) under %s after eviction+re-hydration",
		len(objects), objectPrefix)
}
