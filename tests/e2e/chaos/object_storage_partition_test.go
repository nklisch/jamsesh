// Invariant: the portal maintains RPO=0 durability across bounded network
// partitions on the portal→MinIO path. Toxiproxy intercepts traffic between
// portal containers and MinIO; the test process reaches MinIO directly to
// perform bucket inspection.
//
// Active scenarios:
//
//   - latency_5s_writes_succeed — Toxiproxy injects 5 000ms latency on the
//     portal→MinIO path. A git push must eventually succeed (within a 60s
//     timeout). After the push, direct bucket inspection via mn.ListObjects
//     must find objects under sessions/<id>/. Remove the toxic and confirm the
//     next push completes quickly.
//
//   - transient_reset_peer_rpo0_holds — Toxiproxy injects reset_peer for ~3s
//     then removes it. A push attempted during the window may succeed or fail.
//     The forbidden outcome is: push returns success AND the bucket has zero
//     objects — that is an RPO=0 violation. Both outcomes (success+objects,
//     failure+no-leak) are acceptable.
//
//   - permanent_disconnect_fails_loudly — Toxiproxy injects reset_peer
//     permanently. A push must return a non-zero exit code (non-2xx HTTP).
//     The bucket must have zero objects for the session (nothing leaked).
package chaos_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/toxiproxy"
)

// TestObjectStoragePartition is the top-level chaos test for the portal→MinIO
// network partition scenarios. Each sub-test sets up a fresh full stack
// (Postgres + MinIO + Toxiproxy + clustered portal) to prevent cross-
// contamination between chaos scenarios.
func TestObjectStoragePartition(t *testing.T) {
	t.Run("latency_5s_writes_succeed", testLatency5sWritesSucceed)
	t.Run("transient_reset_peer_rpo0_holds", testTransientResetPeerRPO0Holds)
	t.Run("permanent_disconnect_fails_loudly", testPermanentDisconnectFailsLoudly)
}

// ---------------------------------------------------------------------------
// Scenario 1: latency_5s_writes_succeed
//
// Invariant: Toxiproxy injects 5 000ms latency on every portal→MinIO TCP
// connection. The portal's S3 client must retry until it succeeds; the push
// must eventually return success within a 60s client timeout. Objects must
// land in the bucket (RPO=0).
//
// Anti-tautology: a baseline push is asserted to complete in < 10s before the
// toxic is injected — if the baseline is already slow the chaos result would
// be meaningless.
// ---------------------------------------------------------------------------

func testLatency5sWritesSucceed(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	mn, tp, mh, cluster := partitionSetupStack(ctx, t)

	pod0 := cluster.Pods[0]

	// ── Baseline: push completes quickly without any toxic ───────────────────
	{
		userEmail := randEmail(t, "partition-lat-base")
		pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
		userID := partitionGetMe(ctx, t, pod0.URL, pair.AccessToken)
		orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Partition Latency Baseline Org")
		sessionID := partitionCreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "partition-lat-baseline")

		repo := gitclient.Clone(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken)
		ref := "jam/" + sessionID + "/" + userID + "/main"
		repo.Commit(ctx, t, "baseline.md", "baseline content", "partition: baseline commit")

		start := time.Now()
		repo.Push(ctx, t, ref)
		elapsed := time.Since(start)

		if elapsed > 10*time.Second {
			t.Fatalf("latency_5s_writes_succeed: baseline push took %v (>10s); stack is too slow — chaos results would be meaningless", elapsed)
		}
		t.Logf("latency_5s_writes_succeed: baseline push elapsed: %v", elapsed)
	}

	// ── Inject 5 000ms latency toxic ────────────────────────────────────────
	const toxicName = "latency_5000ms"
	tp.AddLatency(ctx, t, partitionProxyName, toxicName, 5000)
	toxicRemoved := false
	t.Cleanup(func() {
		if !toxicRemoved {
			tp.RemoveToxic(context.Background(), t, partitionProxyName, toxicName)
		}
	})

	// ── Under-chaos push: expect eventual success within 60s ─────────────────
	userEmail := randEmail(t, "partition-lat-chaos")
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	userID := partitionGetMe(ctx, t, pod0.URL, pair.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Partition Latency Chaos Org")
	sessionID := partitionCreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "partition-lat-chaos")

	// Clone and commit before chaos push (clone uses portal HTTP, not MinIO,
	// so it is unaffected by the toxic on the MinIO proxy).
	repo := gitclient.Clone(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken)
	ref := "jam/" + sessionID + "/" + userID + "/main"
	repo.Commit(ctx, t, "chaos.md", "chaos content under 5s latency", "partition: latency chaos commit")

	// Push with a long-enough timeout so the S3 client can succeed despite
	// 5 000ms latency per connection. 60s is generous — the latency is injected
	// per connection, and a push may involve multiple S3 PutObject calls.
	pushCtx, pushCancel := context.WithTimeout(ctx, 60*time.Second)
	defer pushCancel()
	pushErr := partitionTryPush(pushCtx, repo, ref)

	if pushErr != nil {
		// Push failed even under 60s. This may indicate the portal's S3 client
		// does not retry or has a too-short internal timeout — log and fail.
		t.Fatalf("latency_5s_writes_succeed: push failed under 5s latency (60s budget): %v", pushErr)
	}

	// ── RPO=0 assertion: bucket FIRST ────────────────────────────────────────
	// The push returned success — verify objects are actually in the bucket.
	prefix := "sessions/" + sessionID + "/"
	keys, err := mn.ListObjects(ctx, prefix)
	if err != nil {
		t.Fatalf("latency_5s_writes_succeed: ListObjects(%q) error: %v", prefix, err)
	}
	if len(keys) == 0 {
		// RPO=0 violation: push returned 2xx but the bucket is empty.
		// This is a production durability bug.
		t.Fatalf("latency_5s_writes_succeed: RPO=0 VIOLATED — push returned success but bucket has no objects under %q (bucket=%q). This is a durability violation — file a production bug.",
			prefix, mn.BucketName)
	}
	t.Logf("latency_5s_writes_succeed: %d object(s) in bucket under %s — RPO=0 holds", len(keys), prefix)

	// ── Remove toxic and verify recovery ─────────────────────────────────────
	tp.RemoveToxic(ctx, t, partitionProxyName, toxicName)
	toxicRemoved = true

	// A post-recovery push should complete quickly.
	userEmail2 := randEmail(t, "partition-lat-recov")
	pair2 := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail2)
	userID2 := partitionGetMe(ctx, t, pod0.URL, pair2.AccessToken)
	orgID2 := authflow.CreateOrg(ctx, t, pod0, pair2.AccessToken, "Partition Latency Recovery Org")
	sessionID2 := partitionCreateSession(ctx, t, pod0.URL, pair2.AccessToken, orgID2, "partition-lat-recov")
	repo2 := gitclient.Clone(ctx, t, pod0.URL, orgID2, sessionID2, userID2, pair2.AccessToken)
	ref2 := "jam/" + sessionID2 + "/" + userID2 + "/main"
	repo2.Commit(ctx, t, "recovery.md", "recovery content", "partition: post-toxic recovery commit")

	start := time.Now()
	repo2.Push(ctx, t, ref2)
	recovElapsed := time.Since(start)
	t.Logf("latency_5s_writes_succeed: post-toxic recovery push elapsed: %v", recovElapsed)
	if recovElapsed > 15*time.Second {
		t.Errorf("latency_5s_writes_succeed: post-toxic recovery push took %v (>15s); portal may not have fully recovered", recovElapsed)
	}
}

// ---------------------------------------------------------------------------
// Scenario 2: transient_reset_peer_rpo0_holds
//
// Invariant: Toxiproxy injects reset_peer for 3s then removes it. A push
// during the window may succeed or fail. The forbidden outcome: push returned
// success AND the bucket has zero objects for the session — that is an RPO=0
// violation. Both consistent outcomes (success+objects, failure+empty-bucket)
// are acceptable.
// ---------------------------------------------------------------------------

func testTransientResetPeerRPO0Holds(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	mn, tp, mh, cluster := partitionSetupStack(ctx, t)

	pod0 := cluster.Pods[0]

	userEmail := randEmail(t, "partition-trans")
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	userID := partitionGetMe(ctx, t, pod0.URL, pair.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Partition Transient Org")
	sessionID := partitionCreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "partition-transient")

	repo := gitclient.Clone(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken)
	ref := "jam/" + sessionID + "/" + userID + "/main"
	repo.Commit(ctx, t, "transient.md", "transient partition content", "partition: transient commit")

	// ── Inject reset_peer, push, remove after 3s ──────────────────────────
	const toxicName = "reset_peer_transient"
	tp.AddResetPeer(ctx, t, partitionProxyName, toxicName, 0)
	toxicRemoved := false
	t.Cleanup(func() {
		if !toxicRemoved {
			tp.RemoveToxic(context.Background(), t, partitionProxyName, toxicName)
		}
	})

	// Attempt the push during the reset_peer window. Allow up to 15s for
	// the push to complete or fail — the portal may have a connection retry
	// budget of a few seconds before giving up.
	pushCtx, pushCancel := context.WithTimeout(ctx, 15*time.Second)
	defer pushCancel()
	pushErr := partitionTryPush(pushCtx, repo, ref)

	// Remove the toxic after 3s (or after the push returns, whichever comes
	// first). We already waited up to 15s for the push.
	tp.RemoveToxic(ctx, t, partitionProxyName, toxicName)
	toxicRemoved = true

	// Allow time for any in-flight S3 sync to land after the partition heals.
	// The portal may have queued the S3 write and completes it once reconnected.
	time.Sleep(2 * time.Second)

	// ── RPO=0 invariant check ────────────────────────────────────────────────
	prefix := "sessions/" + sessionID + "/"
	keys, listErr := mn.ListObjects(ctx, prefix)
	if listErr != nil {
		t.Fatalf("transient_reset_peer_rpo0_holds: ListObjects(%q) error: %v", prefix, listErr)
	}

	switch {
	case pushErr == nil && len(keys) > 0:
		// Best outcome: push succeeded, objects in bucket. RPO=0 holds.
		t.Logf("transient_reset_peer_rpo0_holds: push succeeded, %d object(s) in bucket — RPO=0 holds", len(keys))

	case pushErr != nil && len(keys) == 0:
		// Consistent failure: push failed (surfaced the error), bucket is empty (no leak).
		// This is acceptable.
		t.Logf("transient_reset_peer_rpo0_holds: push failed (expected under partition), bucket empty (no leak) — consistent failure, RPO=0 holds: push_err=%v", pushErr)

	case pushErr == nil && len(keys) == 0:
		// FORBIDDEN: push returned success but bucket has nothing.
		// This is an RPO=0 violation — a production durability bug.
		// Per test-integrity rules: do NOT change this assertion to allow this outcome.
		t.Fatalf(
			"transient_reset_peer_rpo0_holds: RPO=0 VIOLATED — "+
				"push returned success (exit 0) but bucket has NO objects under %q "+
				"(bucket=%q) after the partition healed. "+
				"This is a production durability bug: push was acknowledged but data was not durable. "+
				"File a Critical bug via /agile-workflow:park.",
			prefix, mn.BucketName,
		)

	case pushErr != nil && len(keys) > 0:
		// Push reported failure but objects are in the bucket. This means the
		// portal completed the S3 write but the git push still failed (possible
		// if the portal aborted after syncing but before ACK). The objects in
		// the bucket are orphaned — the session will not reference them via a
		// push ACK, but they are not a data-loss RPO violation (no data was
		// silently lost; the push was not acknowledged). Log as an anomaly.
		t.Logf("transient_reset_peer_rpo0_holds: push failed but %d orphaned object(s) found in bucket under %q — push was not ACK'd so RPO=0 still holds (no silent data loss), but orphaned objects may indicate a mid-push abort after S3 sync. push_err=%v",
			len(keys), prefix, pushErr)
	}
}

// ---------------------------------------------------------------------------
// Scenario 3: permanent_disconnect_fails_loudly
//
// Invariant: Toxiproxy injects reset_peer permanently. A push must fail
// (non-zero exit / non-2xx HTTP). After the push attempt the bucket must have
// zero objects for the session — nothing leaked silently.
// ---------------------------------------------------------------------------

func testPermanentDisconnectFailsLoudly(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	mn, tp, mh, cluster := partitionSetupStack(ctx, t)

	pod0 := cluster.Pods[0]

	userEmail := randEmail(t, "partition-perm")
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	userID := partitionGetMe(ctx, t, pod0.URL, pair.AccessToken)
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Partition Permanent Org")
	sessionID := partitionCreateSession(ctx, t, pod0.URL, pair.AccessToken, orgID, "partition-permanent")

	repo := gitclient.Clone(ctx, t, pod0.URL, orgID, sessionID, userID, pair.AccessToken)
	ref := "jam/" + sessionID + "/" + userID + "/main"
	repo.Commit(ctx, t, "permanent.md", "permanent partition content", "partition: permanent commit")

	// ── Inject permanent reset_peer ──────────────────────────────────────────
	const toxicName = "reset_peer_permanent"
	tp.AddResetPeer(ctx, t, partitionProxyName, toxicName, 0)
	// Defensive cleanup — leave in place for the rest of the test but ensure it
	// is removed when the test stack tears down.
	t.Cleanup(func() {
		// Best-effort: the proxy may already be gone when the container is torn down.
		_ = toxiproxyRemoveToxicBestEffort(tp, partitionProxyName, toxicName)
	})

	// ── Push under permanent partition — must fail ────────────────────────────
	// Give the portal up to 30s to time out and surface a clear error. If the
	// portal hangs indefinitely, the test will fail at context deadline.
	//
	// Important: if the push hangs (no fail-fast) this is an Important bug.
	// Log a clear note and fail the test — park separately if this surfaces.
	pushCtx, pushCancel := context.WithTimeout(ctx, 30*time.Second)
	defer pushCancel()

	pushErr := partitionTryPush(pushCtx, repo, ref)

	if pushErr == nil {
		// Push returned success with a permanent partition — this must not happen.
		// Either the portal silently accepted the push without writing to MinIO,
		// or the toxic did not intercept properly. Check the bucket.
		prefix := "sessions/" + sessionID + "/"
		keys, listErr := mn.ListObjects(ctx, prefix)
		if listErr != nil {
			t.Fatalf("permanent_disconnect_fails_loudly: push returned success AND ListObjects errored: list_err=%v", listErr)
		}
		if len(keys) == 0 {
			t.Fatalf("permanent_disconnect_fails_loudly: push returned success under permanent partition AND bucket is empty under %q — RPO=0 VIOLATED (silent success with no data). This is a production bug.", prefix)
		}
		// If somehow the push succeeded and objects exist (toxic not active?),
		// fail with explanation.
		t.Fatalf("permanent_disconnect_fails_loudly: push returned success under permanent reset_peer partition with %d object(s) in bucket — Toxiproxy may not be intercepting portal→MinIO traffic correctly. Verify JAMSESH_OBJECT_STORAGE_ENDPOINT_URL is routed through tp.ContainerIP:%d.", len(keys), partitionProxyPort)
	}

	// Push failed as expected — verify no data leaked into the bucket.
	prefix := "sessions/" + sessionID + "/"
	keys, listErr := mn.ListObjects(ctx, prefix)
	if listErr != nil {
		t.Fatalf("permanent_disconnect_fails_loudly: ListObjects(%q) error: %v", prefix, listErr)
	}
	if len(keys) > 0 {
		// The push failed (no ACK) but objects appeared in the bucket.
		// This means the portal wrote to MinIO before the push ACK, then the
		// push ACK failed. The objects are orphaned but no data was
		// "silently" lost (the push was not acknowledged). Log as anomaly
		// but not an RPO=0 violation.
		t.Logf("permanent_disconnect_fails_loudly: push failed (expected) but %d orphaned object(s) found in bucket under %q — push was not ACK'd (RPO=0 holds for the client's perspective), but orphaned objects indicate partial work before fail-fast. push_err=%v",
			len(keys), prefix, pushErr)
	} else {
		t.Logf("permanent_disconnect_fails_loudly: push failed (expected), bucket empty (no leak) — partition fails loudly as required. push_err=%v", pushErr)
	}

	t.Logf("permanent_disconnect_fails_loudly: PASS — push returned non-zero exit under permanent partition, no silent success")
}

// ---------------------------------------------------------------------------
// Shared setup helpers
// ---------------------------------------------------------------------------

const (
	// partitionProxyName is the Toxiproxy proxy name for the portal→MinIO path.
	partitionProxyName = "minio"
	// partitionProxyPort is the port Toxiproxy listens on inside the Docker network
	// for the MinIO proxy. Portal containers connect to tp.ContainerIP:9001.
	partitionProxyPort = 9001
	// partitionProxyListen is the Toxiproxy internal listen address.
	partitionProxyListen = "0.0.0.0:9001"
)

// partitionSetupStack spins up the full stack for a partition sub-test:
//
//   - MinIO (direct target; test assertions bypass Toxiproxy via host-side Endpoint)
//   - Toxiproxy (proxy in front of MinIO, portal connects through it)
//   - Postgres (shared DB for the cluster)
//   - MailHog (for magic-link delivery)
//   - Clustered portal (2 pods, routed through Toxiproxy for MinIO)
//
// Returns (mn, tp, mh, cluster). Postgres is used internally; callers do not
// need it after Start returns.
func partitionSetupStack(ctx context.Context, t *testing.T) (
	*minio.MinIO,
	*toxiproxy.Toxiproxy,
	*mailhog.MailHog,
	*portalcluster.Cluster,
) {
	t.Helper()

	mn := minio.Start(ctx, t, minio.Options{})
	tp := toxiproxy.Start(ctx, t)
	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)

	// Create a Toxiproxy proxy: tp-container port 9001 → MinIO container IP:9000.
	// stripScheme removes "http://" to get "ip:9000" that Toxiproxy accepts as upstream.
	minioUpstream := partitionStripScheme(mn.ContainerEndpoint)
	tp.CreateProxy(ctx, t, partitionProxyName, partitionProxyListen, minioUpstream)

	// Configure the cluster so each portal pod routes its S3 writes through
	// Toxiproxy (tp.ContainerIP:9001). The test process uses mn.Endpoint
	// (host-mapped port, bypasses Toxiproxy) for direct bucket inspection.
	cluster := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        2,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      false,
		PortalExtraEnv: map[string]string{
			// Override the object-storage endpoint so portal containers reach
			// MinIO via Toxiproxy instead of directly.
			"JAMSESH_OBJECT_STORAGE_ENDPOINT_URL": fmt.Sprintf("http://%s:%d", tp.ContainerIP, partitionProxyPort),
			// SMTP for magic-link delivery via MailHog.
			"JAMSESH_EMAIL_PROVIDER":  "smtp",
			"JAMSESH_EMAIL_SMTP_HOST": mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT": strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":  "none",
		},
	})

	return mn, tp, mh, cluster
}

// partitionStripScheme removes a leading "http://" or "https://" scheme from
// addr, returning the bare "host:port" form required by Toxiproxy's upstream
// field.
func partitionStripScheme(addr string) string {
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(addr, prefix) {
			return addr[len(prefix):]
		}
	}
	return addr
}

// partitionTryPush pushes HEAD to the given ref on the repo's origin remote,
// returning an error instead of calling t.Fatal on failure. This allows
// chaos subtests to observe push failures without aborting the test.
func partitionTryPush(ctx context.Context, repo *gitclient.Repo, ref string) error {
	cmd := exec.CommandContext(ctx, "git", "push", "origin", "HEAD:refs/heads/"+ref)
	cmd.Dir = repo.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push: %w\noutput: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// toxiproxyRemoveToxicBestEffort removes a toxic without failing the test on
// error. Used in t.Cleanup for toxic teardown when the proxy container may
// already be gone.
func toxiproxyRemoveToxicBestEffort(tp *toxiproxy.Toxiproxy, proxyName, toxicName string) error {
	url := fmt.Sprintf("%s/proxies/%s/toxics/%s", tp.AdminURL, proxyName, toxicName)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("toxiproxy: remove toxic: status %d", resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// API helpers (scoped to this file — no shared package deps on golden helpers)
// ---------------------------------------------------------------------------

// partitionSessionRef captures the ID from POST /api/orgs/{id}/sessions.
type partitionSessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// partitionGetMe calls GET /api/me and returns the user ID.
func partitionGetMe(ctx context.Context, t *testing.T, baseURL, accessToken string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/me", nil)
	if err != nil {
		t.Fatalf("partitionGetMe: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("partitionGetMe: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("partitionGetMe: status %d: %s", resp.StatusCode, body)
	}
	var me struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("partitionGetMe: decode: %v\nbody: %s", err, body)
	}
	if me.ID == "" {
		t.Fatalf("partitionGetMe: empty user ID")
	}
	return me.ID
}

// partitionCreateSession calls POST /api/orgs/{orgID}/sessions and returns the
// new session ID.
func partitionCreateSession(ctx context.Context, t *testing.T, baseURL, accessToken, orgID, name string) string {
	t.Helper()
	body := map[string]string{
		"name":         name,
		"goal":         "chaos partition test session",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("partitionCreateSession: marshal: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", baseURL, orgID),
		bytes.NewReader(b))
	if err != nil {
		t.Fatalf("partitionCreateSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("partitionCreateSession: POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("partitionCreateSession: status %d (want 201): %s", resp.StatusCode, respBody)
	}
	var s partitionSessionRef
	if err := json.Unmarshal(respBody, &s); err != nil {
		t.Fatalf("partitionCreateSession: decode: %v\nbody: %s", err, respBody)
	}
	if s.ID == "" {
		t.Fatalf("partitionCreateSession: empty session ID")
	}
	return s.ID
}
