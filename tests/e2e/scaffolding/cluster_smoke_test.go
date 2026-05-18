package scaffolding_test

// TestClusteredSmoke is the keystone acceptance test for the cnd-coverage
// cluster-fixture feature. If this passes, downstream cnd-coverage features
// (lease-fencing, object-storage-sync, routing-layer, hydration-handoff)
// are unblocked to start.
//
// Invariant: a session created on pod A is visible on pod B after a graceful
// drain, with all committed state preserved and the object backed by MinIO.
//
// Auth approach: magic-link sign-in via MailHog, reusing the same pattern as
// TestSessionLifecycleJoinAndPush in golden/. MailHog is passed to the cluster
// via PortalExtraEnv so the portal containers can deliver sign-in emails.
//
// Scope: full (all 7 invariant steps).
//   1. Start Postgres + MinIO + MailHog + 3-pod clustered portal cluster + router.
//   2. Authenticate via magic link; create org + session.
//   3. Push a commit through the router; capture HEAD SHA.
//   4. Inspect MinIO: assert session objects landed (RPO=0 invariant).
//   5. Find lease holder pod.
//   6. Gracefully drain the lease-holding pod.
//   7. Fetch the pushed ref through the router from a fresh repo; assert SHA matches.
//   8. Assert lease migrated to a different pod.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// smokeSessionRef is the minimal subset of the session-creation response.
type smokeSessionRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TestClusteredSmoke is the keystone acceptance test for the cnd-coverage
// cluster-fixture feature.
func TestClusteredSmoke(t *testing.T) {
	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})
	mh := mailhog.Start(ctx, t)

	// Pass MailHog SMTP settings to each portal pod so magic-link emails are
	// delivered. The cluster fixture exposes PortalExtraEnv for exactly this.
	cluster := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        3,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      true,
		PortalExtraEnv: map[string]string{
			"JAMSESH_EMAIL_PROVIDER":   "smtp",
			"JAMSESH_EMAIL_SMTP_HOST":  mh.ContainerSMTPHost,
			"JAMSESH_EMAIL_SMTP_PORT":  strconv.Itoa(mh.ContainerSMTPPort),
			"JAMSESH_EMAIL_SMTP_TLS":   "none",
		},
	})
	require.NotEmpty(t, cluster.RouterURL, "smoke test requires router")

	// Auth flows go to pod 0 directly — all pods share Postgres, so tokens
	// and session data created here are visible across the cluster.
	pod0 := cluster.Pods[0]

	// ── Authentication ───────────────────────────────────────────────────────
	// 1. Sign in via magic link (request → MailHog → exchange).
	userEmail := "smoke-" + fmt.Sprintf("%d", time.Now().UnixNano()) + "@example.com"
	pair := authflow.SignInViaMagicLink(ctx, t, pod0, mh, userEmail)
	t.Logf("smoke: authenticated as %s", userEmail)

	// 2. Capture user ID (needed for git ref namespace).
	userID := smokeGetMe(ctx, t, pod0, pair.AccessToken)
	t.Logf("smoke: user ID = %s", userID)

	// 3. Create an org, then a session.
	orgID := authflow.CreateOrg(ctx, t, pod0, pair.AccessToken, "Smoke Org")
	sessionID := createSessionViaRouter(ctx, t, cluster.RouterURL, pair.AccessToken, orgID)
	t.Logf("smoke: created session %s", sessionID)

	// ── Push a commit through the router ─────────────────────────────────────
	// 4. Clone from the router URL and push one commit.
	pushRef, headSHA := pushCommitViaRouter(ctx, t, cluster.RouterURL, pair.AccessToken, orgID, sessionID, userID)
	t.Logf("smoke: pushed commit %s on ref %s", headSHA, pushRef)

	// ── RPO=0 assertion — bucket must have session objects ────────────────────
	// 5. The objectstore sync layer uploads to "sessions/<sessionID>/…" after
	//    every push. List all objects under that prefix and assert non-empty.
	//    (The push already returned 2xx; this proves the objects *actually landed*
	//    in MinIO — the RPO=0 contract.)
	//
	//    We poll for up to 10 s to allow for any post-receive async window,
	//    though SyncPush is synchronous before the push ack (RPO=0 design).
	var objects []string
	var listErr error
	objectPrefix := "sessions/" + sessionID + "/"
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		objects, listErr = mn.ListObjects(ctx, objectPrefix)
		if listErr == nil && len(objects) > 0 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	require.NoError(t, listErr)
	require.NotEmptyf(t, objects,
		"session push MUST be mirrored to MinIO bucket — RPO=0 invariant violated; bucket=%q prefix=%q",
		mn.BucketName, objectPrefix)
	t.Logf("smoke: MinIO has %d object(s) under %s", len(objects), objectPrefix)

	// ── Lease holder ─────────────────────────────────────────────────────────
	// 6. Identify which pod holds the advisory lock for this session.
	holderIndex := cluster.LeaseHolder(ctx, t, sessionID)
	require.GreaterOrEqualf(t, holderIndex, 0,
		"a pod must hold the lease for session %s after push", sessionID)
	t.Logf("smoke: lease held by pod %d", holderIndex)

	// ── Graceful drain ───────────────────────────────────────────────────────
	// 7. Send SIGTERM to the lease-holding pod and wait for clean shutdown.
	cluster.GracefulDrain(ctx, t, holderIndex, 30*time.Second)
	t.Logf("smoke: drained pod %d", holderIndex)

	// ── Handoff — state must be preserved on the new pod ─────────────────────
	// 8. Fetch the pushed ref through the router. The router routes to a
	//    surviving pod, which hydrates from MinIO if it does not hold the local
	//    repo yet. The fetched SHA must match what we pushed.
	headFromNewPod := getSessionHeadViaRouter(ctx, t, cluster.RouterURL, pair.AccessToken, orgID, sessionID, userID, pushRef)
	require.Equalf(t, headSHA, headFromNewPod,
		"handoff must preserve committed state: pushed=%s after-handoff=%s",
		headSHA, headFromNewPod)
	t.Logf("smoke: handoff preserved commit state (%s)", headFromNewPod)

	// ── Lease migration ──────────────────────────────────────────────────────
	// 9. Wait for the lease to migrate to a different (surviving) pod. A 10 s
	//    window is generous for the happy-path lease-takeover.
	newHolder := cluster.WaitForLeaseMigration(ctx, t, sessionID, holderIndex, 10*time.Second)
	require.NotEqualf(t, holderIndex, newHolder,
		"lease must have migrated away from drained pod %d; got holder=%d", holderIndex, newHolder)
	require.GreaterOrEqualf(t, newHolder, 0,
		"a pod must hold the lease post-handoff")
	t.Logf("smoke: lease migrated from pod %d to pod %d", holderIndex, newHolder)
}

// ---------------------------------------------------------------------------
// Per-test helpers — these are specific to cluster_smoke_test.go and operate
// against the router URL rather than a direct portal.Portal.
// ---------------------------------------------------------------------------

// createSessionViaRouter posts to /api/orgs/{orgID}/sessions through the
// router URL and returns the new session ID.
func createSessionViaRouter(ctx context.Context, t *testing.T, routerURL, accessToken, orgID string) string {
	t.Helper()
	body := map[string]string{
		"name":         "smoke-session",
		"goal":         "clustered smoke test",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err, "createSessionViaRouter: marshal body")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/orgs/%s/sessions", routerURL, orgID),
		bytes.NewReader(b))
	require.NoError(t, err, "createSessionViaRouter: build request")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "createSessionViaRouter: POST")
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"createSessionViaRouter: want 201; body: %s", respBody)

	var s smokeSessionRef
	require.NoError(t, json.Unmarshal(respBody, &s), "createSessionViaRouter: decode response")
	require.NotEmpty(t, s.ID, "createSessionViaRouter: empty session ID")
	return s.ID
}

// pushCommitViaRouter clones the session repo via the router URL, commits a
// single file, pushes it to the user's personal ref namespace, and returns
// the ref name and the pushed HEAD SHA.
//
// The git-smart-HTTP endpoint is:
//   GET/POST {routerURL}/git/{orgID}/{sessionID}.git
//
// Authentication uses HTTP Basic auth with username "x-access-token" and the
// bearer token as password (identical to the single-pod tests in golden/).
func pushCommitViaRouter(
	ctx context.Context, t *testing.T,
	routerURL, accessToken, orgID, sessionID, userID string,
) (ref, headSHA string) {
	t.Helper()

	repo := gitclient.Clone(ctx, t, routerURL, orgID, sessionID, userID, accessToken)

	ref = "jam/" + sessionID + "/" + userID + "/main"
	headSHA = repo.Commit(ctx, t, "smoke.md", "clustered smoke test commit", "smoke: initial commit")
	repo.Push(ctx, t, ref)

	return ref, headSHA
}

// getSessionHeadViaRouter clones the session repo from the router URL into a
// fresh temporary directory, fetches all refs, and resolves the given ref to
// its tip SHA. This is the handoff invariant check: the ref's tip must survive
// a pod drain and be served from whichever pod the router now routes to.
func getSessionHeadViaRouter(
	ctx context.Context, t *testing.T,
	routerURL, accessToken, orgID, sessionID, userID, ref string,
) string {
	t.Helper()

	// Clone into a fresh directory — do not reuse the pusher's working tree,
	// since that tree has the commit locally and would not exercise hydration.
	repo := gitclient.Clone(ctx, t, routerURL, orgID, sessionID, userID, accessToken)
	repo.Fetch(ctx, t)

	sha := repo.RevParse(ctx, t, ref)
	require.NotEmpty(t, sha, "getSessionHeadViaRouter: RevParse returned empty SHA for ref %q", ref)
	return sha
}

// smokeGetMe calls GET /api/me against the given pod and returns the caller's
// user ID. The cluster version of getMe uses *portal.Portal, so we replicate
// the call against a pod0 URL here.
func smokeGetMe(ctx context.Context, t *testing.T, p *portal.Portal, accessToken string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/api/me", nil)
	require.NoError(t, err, "smokeGetMe: build request")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "smokeGetMe: GET /api/me")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "smokeGetMe: status; body=%s", body)

	var me struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	require.NoError(t, json.Unmarshal(body, &me), "smokeGetMe: decode")
	require.NotEmpty(t, me.ID, "smokeGetMe: empty user ID")
	return me.ID
}
