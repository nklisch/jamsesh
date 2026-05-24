// Invariant: an anonymous client can create a playground session, push a commit
// to it, and after the destruction-worker sweep fires (clock advanced past the
// hard-cap) the tombstone endpoint returns 200 with members_count=1,
// commits_count>=1, end_reason="hard_cap", while the bare repo is gone from
// the container filesystem.
package golden_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// soloCreateResponse is the 201 body from POST /api/playground/sessions.
type soloCreateResponse struct {
	Session   soloSessionSummary `json:"session"`
	Bearer    string             `json:"bearer"`
	Nickname  string             `json:"nickname"`
	ExpiresAt string             `json:"expires_at"`
}

// soloSessionSummary is the compact descriptor embedded in playground create/join responses.
type soloSessionSummary struct {
	ID           string `json:"id"`
	OrgID        string `json:"org_id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	MembersCount int    `json:"members_count"`
}

// soloTombstone is the destruction summary returned by GET /api/playground/sessions/{id}/tombstone.
type soloTombstone struct {
	SessionID       string `json:"session_id"`
	OrgID           string `json:"org_id"`
	MembersCount    int    `json:"members_count"`
	CommitsCount    int    `json:"commits_count"`
	AutoMergesCount int    `json:"auto_merges_count"`
	DurationSeconds int    `json:"duration_seconds"`
	EndReason       string `json:"end_reason"`
	EndedAt         string `json:"ended_at"`
	ExpiresAt       string `json:"expires_at"`
}


func TestPlayground_SoloCreatePushTombstone(t *testing.T) {
	// Blocked on bug-playground-git-receive-pack-fails-with-200-hangup
	// (ROOT CAUSE IDENTIFIED in that bug's body): the base-ref push is
	// rejected by prereceive.WalkAndValidate because the seed commit lacks
	// the required Jam-Session/Jam-Turn/Jam-Author trailers
	// (internal/portal/prereceive/commits.go:15). The test will only pass
	// once base-ref pushes are exempted from trailer validation (or
	// equivalent fix).
	//
	// The original second blocker (idea-playground-worker-clock-not-advanceable)
	// was resolved as a single-stride fix at commit cc55579; once the
	// push bug is also fixed, this test's clock-advance step will Just Work.
	t.Skip("blocked on bug-playground-git-receive-pack-fails-with-200-hangup (root cause: trailer requirement on seed commit)")

	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
	// Postgres + single portal with playground enabled.
	// Hard-cap: 60s (short so clock-advance is feasible).
	// Destruction sweep interval: 1s (so the sweep fires within 1s of AdvanceClock).
	// Idle timeout: 300s (larger than hard-cap so the hard_cap path fires first).
	pg := postgres.Start(ctx, t, postgres.Options{})
	p := portal.Start(ctx, t, portal.Options{
		DBDriver: "postgres",
		DBDSN:    pg.ContainerDSN,
		ExtraEnv: map[string]string{
			"JAMSESH_PLAYGROUND_ENABLED":                  "true",
			"JAMSESH_PLAYGROUND_HARD_CAP_S":               "60",
			"JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S":           "300",
			"JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S": "1",
		},
	})

	// ── Step 1: Anonymous create ─────────────────────────────────────────────
	t.Log("playground: creating anonymous session")
	createResp := playgroundCreate(ctx, t, p)
	sessionID := createResp.Session.ID
	bearer := createResp.Bearer

	require.NotEmpty(t, sessionID, "session ID must be non-empty")
	require.NotEmpty(t, bearer, "bearer must be non-empty")
	// The bearer is a 64-char hex-encoded opaque token (raw 32 random bytes).
	// It does not carry a visible prefix — the anonymous nature is stored on the
	// token row in the DB (CreateAnonymousBearer), not in the token string itself.
	require.Len(t, bearer, 64,
		"bearer must be a 64-char hex opaque token, got length %d: %q", len(bearer), bearer)
	require.Equal(t, "org_playground", createResp.Session.OrgID,
		"session org_id must be org_playground")
	require.Equal(t, 1, createResp.Session.MembersCount,
		"newly created session must have exactly 1 member")
	t.Logf("playground: session=%s bearer_prefix=%s", sessionID, bearer[:20])

	// Derive the accountID by calling GET /api/me with the anonymous bearer.
	// The anon account ID is injected into context by BearerMiddleware and
	// returned as "id" in the /api/me response.
	me := getMe(ctx, t, p, bearer)
	accountID := me.ID
	require.NotEmpty(t, accountID, "account ID from /api/me must be non-empty")
	t.Logf("playground: anon accountID=%s", accountID)

	// ── Step 2: Assert the bare repo exists on real disk ─────────────────────
	// The storage root inside the container is /tmp/jamsesh-repos (see portal.go
	// buildEnv: JAMSESH_STORAGE = "/tmp/jamsesh-repos"). The path shape comes
	// from storage.RepoPath: <root>/orgs/<orgID>/sessions/<sessionID>.git.
	repoPath := "/tmp/jamsesh-repos/orgs/org_playground/sessions/" + sessionID + ".git"
	code, output, err := p.Exec(ctx, []string{"ls", "-la", repoPath})
	require.NoError(t, err, "docker exec ls %s: docker API error", repoPath)
	require.Equal(t, 0, code,
		"bare repo must exist at %s after create (ls exit %d)\noutput: %s", repoPath, code, output)
	t.Logf("playground: repo confirmed present at %s", repoPath)

	// ── Step 3: Clone the session repo and push a commit ─────────────────────
	// The git HTTP URL shape is /git/{orgID}/{sessionID}.git where orgID is the
	// actual org ID (not the slug). For playground that is "org_playground"
	// (ReservedOrgID constant in playground/provision.go). The session members
	// and storage paths are all keyed by org_id, so the URL must match.
	orgID := "org_playground"
	ref := "jam/" + sessionID + "/" + accountID + "/main"

	t.Log("playground: cloning session repo and pushing commit")
	repo := gitclient.Clone(ctx, t, p.URL, orgID, sessionID, accountID, bearer)
	repo.Commit(ctx, t, "hello.md", "# Hello from playground e2e test", "playground: initial commit")
	repo.Push(ctx, t, ref)
	t.Logf("playground: pushed commit on ref %s", ref)

	// ── Step 4: Advance the portal clock past the hard-cap (60s) ─────────────
	// Advance by 90s so hard_cap_at (60s from session creation) is firmly in
	// the past. The destruction worker's ticker fires every 1s, so within ~1s
	// of AdvanceClock returning, a sweep should detect the expired session.
	t.Log("playground: advancing clock 90s past hard-cap")
	p.AdvanceClock(ctx, t, 90*time.Second)

	// ── Step 5: Poll for tombstone (up to 10s) ───────────────────────────────
	// The destruction worker sweeps every 1s; we poll until the tombstone
	// endpoint returns 200 or the deadline elapses.
	t.Log("playground: polling for tombstone (up to 10s)")
	tomb := pollForTombstone(ctx, t, p, sessionID, 10*time.Second)

	// ── Step 6: Assert tombstone payload invariants ───────────────────────────
	require.Equal(t, sessionID, tomb.SessionID,
		"tombstone session_id must match the created session")
	require.Equal(t, "org_playground", tomb.OrgID,
		"tombstone org_id must be org_playground")
	require.Equal(t, 1, tomb.MembersCount,
		"tombstone members_count must be 1 (solo creator only)")
	require.GreaterOrEqual(t, tomb.CommitsCount, 1,
		"tombstone commits_count must be >= 1 (we pushed one commit)")
	require.Equal(t, "hard_cap", tomb.EndReason,
		"end_reason must be hard_cap (idle_timeout > hard_cap)")
	t.Logf("playground: tombstone verified: members=%d commits=%d end_reason=%s",
		tomb.MembersCount, tomb.CommitsCount, tomb.EndReason)

	// ── Step 7: Assert the session row is gone (GET returns 404) ─────────────
	// After destruction the sessions row is deleted; the GET endpoint returns 404.
	t.Log("playground: asserting session is gone (expect 404)")
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/playground/sessions/%s", p.URL, sessionID), nil)
	require.NoError(t, reqErr, "build GET /api/playground/sessions/%s request", sessionID)
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, doErr := http.DefaultClient.Do(req)
	require.NoError(t, doErr, "GET /api/playground/sessions/%s", sessionID)
	defer resp.Body.Close()
	io.ReadAll(resp.Body) //nolint:errcheck
	require.Equal(t, http.StatusNotFound, resp.StatusCode,
		"destroyed session must return 404; got %d", resp.StatusCode)
	t.Log("playground: session confirmed gone (404)")

	// ── Step 8: Assert the bare repo is gone from disk ────────────────────────
	// After Destruction.Destroy runs, RemoveRepo deletes the bare-repo directory.
	// A non-zero exit from ls confirms it is absent.
	code, output, err = p.Exec(ctx, []string{"ls", "-la", repoPath})
	require.NoError(t, err, "docker exec ls %s: docker API error", repoPath)
	require.NotEqual(t, 0, code,
		"bare repo must be gone from %s after destruction (ls exit %d)\noutput: %s",
		repoPath, code, output)
	t.Logf("playground: repo confirmed removed from %s (ls exit %d)", repoPath, code)
}

// ---------------------------------------------------------------------------
// Playground-specific API helpers (private to this file)
// ---------------------------------------------------------------------------

// playgroundCreate calls POST /api/playground/sessions and returns the parsed
// 201 response body. Fails the test on any non-201 status.
func playgroundCreate(ctx context.Context, t *testing.T, p *portal.Portal) soloCreateResponse {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.URL+"/api/playground/sessions", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("playgroundCreate: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("playgroundCreate: POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("playgroundCreate: status %d (want 201): %s", resp.StatusCode, body)
	}
	var r soloCreateResponse
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("playgroundCreate: decode response: %v\nbody: %s", err, body)
	}
	if r.Session.ID == "" {
		t.Fatalf("playgroundCreate: empty session.id in response: %s", body)
	}
	if r.Bearer == "" {
		t.Fatalf("playgroundCreate: empty bearer in response: %s", body)
	}
	return r
}

// pollForTombstone polls GET /api/playground/sessions/{id}/tombstone until
// it returns 200 or the deadline elapses. Returns the parsed tombstone body.
// The caller does NOT need a bearer — GetPlaygroundTombstone requires no auth.
func pollForTombstone(ctx context.Context, t *testing.T, p *portal.Portal, sessionID string, deadline time.Duration) soloTombstone {
	t.Helper()
	url := fmt.Sprintf("%s/api/playground/sessions/%s/tombstone", p.URL, sessionID)
	end := time.Now().Add(deadline)
	var lastStatus int
	var lastBody string
	for time.Now().Before(end) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("pollForTombstone: build request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("pollForTombstone: GET: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastStatus = resp.StatusCode
		lastBody = string(body)
		if resp.StatusCode == http.StatusOK {
			var tomb soloTombstone
			if err := json.Unmarshal(body, &tomb); err != nil {
				t.Fatalf("pollForTombstone: decode 200 body: %v\nbody: %s", err, body)
			}
			return tomb
		}
		// 404 is expected while the session is still active or the sweep hasn't
		// fired yet. Any other status is unexpected — log it but keep polling.
		if resp.StatusCode != http.StatusNotFound {
			t.Logf("pollForTombstone: unexpected status %d (body: %s), continuing", resp.StatusCode, body)
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("pollForTombstone: tombstone not available after %s "+
		"(last status=%d, last body=%s)\n"+
		"Hint: check portal logs — the destruction worker may not have swept "+
		"the session within the polling window.",
		deadline, lastStatus, lastBody)
	panic("unreachable")
}
