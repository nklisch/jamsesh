// Invariant: A playground session that sees no activity for the idle-timeout
// window (advanced via p.AdvanceClock) is swept by the destruction worker,
// its bare repo is deleted from disk, and the tombstone is served at
// GET /api/playground/sessions/{id}/tombstone with end_reason="idle".
// Uses JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S=1 so the ticker fires
// within 1-2s of the clock advance without requiring real idle-timeout wait time.
package golden_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestPlayground_AbandonmentDestructionSweep verifies the idle-abandonment
// destruction journey end-to-end:
//
//  1. Boot stack: postgres + portal with playground enabled, short idle timeout
//     (30s), long hard cap (600s), and 1s sweep interval.
//  2. POST /api/playground/sessions → capture bearer + session ID.
//  3. Confirm bare repo exists on portal container filesystem (anti-tautology).
//  4. Advance clock 60s (2× idle timeout) so the session becomes eligible.
//  5. Poll GET /api/playground/sessions/{id}/tombstone until 200 (~10s deadline).
//  6. Assert tombstone fields: end_reason="idle", members_count=1, duration_seconds>0,
//     expires_at in the future.
//  7. Assert GET /api/playground/sessions/{id} is no longer accessible (401 since
//     bearer is revoked by the destruction cascade before the session row is deleted).
//  8. Assert bare repo is gone from the portal container filesystem.
func TestPlayground_AbandonmentDestructionSweep(t *testing.T) {
	// Blocked on idea-playground-clock-not-wired-e2etest: the playground
	// Handler and destruction Worker in cmd/portal/main.go are wired with
	// playground.RealClock() instead of the testClockProvider's
	// AdvanceableClock, so POST /test/clock-advance has zero effect on
	// playground session expiry checks or destruction-worker sweep decisions.
	// This test relies on AdvanceClock to trigger idle-timeout destruction
	// and will silently never see the sweep fire until the wiring is fixed.
	t.Skip("blocked on idea-playground-clock-not-wired-e2etest")

	ctx := context.Background()

	// ── Stack boot ─────────────────────────────────────────────────────────────
	// JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S=30: session idle-expires after 30s of
	// clock time. Set short so AdvanceClock(60s) reliably blows past the threshold.
	//
	// JAMSESH_PLAYGROUND_HARD_CAP_S=600: long enough that the hard-cap path does
	// NOT fire during the test — the clock advance of 60s is well within the cap.
	// This ensures the worker picks "idle" rather than "hard_cap" as the reason.
	//
	// JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S=1: the ticker fires every
	// 1s in real time. After the clock advance the next tick queries the DB with
	// the injected Now() and sees the session as expired. Without this short
	// interval the test would need to wait the default 60s sweep cadence.
	pg := postgres.Start(ctx, t, postgres.Options{})
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		ExtraEnv: map[string]string{
			"JAMSESH_PLAYGROUND_ENABLED":                    "true",
			"JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S":             "30",
			"JAMSESH_PLAYGROUND_HARD_CAP_S":                 "600",
			"JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S": "1",
		},
	})

	// ── Step 1: Create a playground session ────────────────────────────────────
	created := createPlaygroundSession(ctx, t, p.URL)
	sessionID := created.Session.Id
	bearer := created.Bearer
	t.Logf("playground session created: id=%s nickname=%s", sessionID, created.Nickname)

	// ── Step 2: Confirm bare repo exists (anti-tautology Unit 5 discipline) ────
	// The bare repo should be at /tmp/jamsesh-repos/orgs/org_playground/sessions/<id>.git
	// inside the portal container. Confirming its presence before the clock advance
	// guards against a false-positive absence assertion at step 8 — if the repo
	// was never created we'd trivially pass "it's gone" without proving anything.
	repoPath := "/tmp/jamsesh-repos/orgs/org_playground/sessions/" + sessionID + ".git"
	exitCode, execOut, execErr := p.Exec(ctx, []string{"ls", repoPath})
	require.NoErrorf(t, execErr, "portal docker exec ls %s: API error", repoPath)
	require.Equalf(t, 0, exitCode,
		"bare repo not found at %s immediately after session create (exit %d): %s\n"+
			"— did CreatePlaygroundSession succeed in provisioning the repo?",
		repoPath, exitCode, strings.TrimSpace(execOut))
	t.Logf("bare repo confirmed present at %s before clock advance", repoPath)

	// ── Step 3: Advance clock past idle timeout ────────────────────────────────
	// Advance by 60s (2× the 30s idle timeout) so the session is unambiguously
	// in the expired zone. The cumulative offset is forward-only; AdvanceClock
	// logs the resulting server-now timestamp for diagnostics.
	p.AdvanceClock(ctx, t, 60*time.Second)
	t.Logf("clock advanced 60s past idle timeout; polling tombstone endpoint")

	// ── Step 4: Poll tombstone until 200 ──────────────────────────────────────
	// The destruction worker ticks every 1s (real time). After the clock advance
	// the next tick queries ListExpiredPlaygroundSessions with the injected Now()
	// and sees the session as expired. Destruction cascade completes in <1s.
	// We poll up to 10s with 200ms intervals; that is 5–10 full sweep cycles,
	// giving generous margin for scheduling jitter in CI.
	//
	// GetPlaygroundTombstone requires NO bearer (it is on the public router branch
	// in the handler). It returns 404 while the session is active and 200 after
	// the tombstone is recorded.
	const pollDeadline = 10 * time.Second
	const pollInterval = 200 * time.Millisecond

	var tombstone playgroundTombstone
	tombstoneFound := false
	deadline := time.Now().Add(pollDeadline)
	for time.Now().Before(deadline) {
		resp, body, err := doGET(ctx, p.URL+"/api/playground/sessions/"+sessionID+"/tombstone", "")
		if err != nil {
			t.Logf("tombstone poll: request error (will retry): %v", err)
			time.Sleep(pollInterval)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			if err := json.Unmarshal(body, &tombstone); err != nil {
				t.Fatalf("tombstone poll: decode 200 body: %v\nbody: %s", err, body)
			}
			tombstoneFound = true
			break
		}
		if resp.StatusCode != http.StatusNotFound {
			// Unexpected status — log and continue; don't fatal yet.
			t.Logf("tombstone poll: unexpected status %d (expected 200 or 404): %s",
				resp.StatusCode, strings.TrimSpace(string(body)))
		}
		time.Sleep(pollInterval)
	}

	if !tombstoneFound {
		// The destruction worker did not sweep within 10s after a clock advance.
		// This is a production-observable failure, not a test defect. Per
		// test-integrity discipline, we surface it as an assertion failure with a
		// clear message so the story's design-flaw escape hatch can be activated.
		t.Fatalf(
			"DESTRUCTION WORKER DID NOT SWEEP within %s after 60s clock advance.\n"+
				"Session %s was created with IDLE_TIMEOUT_S=30 and SWEEP_INTERVAL_S=1.\n"+
				"Expected: GET /tombstone returns 200 within ~2 ticks after clock advance.\n"+
				"Actual:   tombstone never appeared.\n\n"+
				"If this is consistently reproducible, this is a production bug in the\n"+
				"clock-inject → sweep interaction. Park via /agile-workflow:park with\n"+
				"severity=High and tag=playground before disabling this assertion.",
			pollDeadline, sessionID,
		)
	}

	t.Logf("tombstone appeared: end_reason=%s members_count=%d duration_seconds=%d expires_at=%s",
		tombstone.EndReason, tombstone.MembersCount, tombstone.DurationSeconds, tombstone.ExpiresAt)

	// ── Step 5: Assert tombstone payload ──────────────────────────────────────
	// end_reason must be "idle" (not "hard_cap") because we set HARD_CAP_S=600
	// and only advanced the clock by 60s — the hard cap has not elapsed.
	// The worker's reasonFor() prefers "hard_cap" when both thresholds are past,
	// so "idle" here proves the idle-timeout path fires specifically.
	require.Equalf(t, "idle", tombstone.EndReason,
		"tombstone end_reason: expected 'idle' (idle-timeout path), got %q.\n"+
			"If 'hard_cap': the clock advance exceeded HARD_CAP_S=600 (impossible at 60s advance) or the\n"+
			"reasonFor() logic has a bug. If 'manual': ListExpiredPlaygroundSessions returned an\n"+
			"unexpected result. Do NOT change this assertion — the mismatch is the bug.",
		tombstone.EndReason,
	)

	require.Equalf(t, 1, tombstone.MembersCount,
		"tombstone members_count: expected 1 (solo creator), got %d", tombstone.MembersCount)

	require.Greaterf(t, tombstone.DurationSeconds, int64(0),
		"tombstone duration_seconds: expected >0, got %d", tombstone.DurationSeconds)

	// expires_at must be in the future (tombstone TTL window is 30 days by default).
	expiresAt, err := time.Parse(time.RFC3339, tombstone.ExpiresAt)
	require.NoErrorf(t, err, "tombstone expires_at: parse %q: %v", tombstone.ExpiresAt, err)
	require.Truef(t, expiresAt.After(time.Now()),
		"tombstone expires_at must be in the future; got %s", tombstone.ExpiresAt)

	// ── Step 6: Assert session is inaccessible ─────────────────────────────────
	// After destruction, the bearer is revoked (step 4 of the cascade), so
	// GET /api/playground/sessions/{id} returns 401 (invalid/revoked token) rather
	// than 404. The BearerMiddleware validates the token BEFORE the handler looks
	// up the session row, so a revoked bearer halts the chain at the middleware
	// layer with "invalid_token". This is the correct observable behavior.
	//
	// We accept any non-200 status as "session no longer accessible" because the
	// exact code (401 vs 404) depends on whether the middleware or the handler
	// is the first to reject. Both prove the session is destroyed.
	resp, body, err := doGET(ctx, p.URL+"/api/playground/sessions/"+sessionID, bearer)
	require.NoErrorf(t, err, "GET /api/playground/sessions/%s after destruction: request error", sessionID)
	require.NotEqualf(t, http.StatusOK, resp.StatusCode,
		"GET /api/playground/sessions/%s returned 200 after destruction — session was NOT destroyed.\n"+
			"body: %s", sessionID, body)
	t.Logf("GET /api/playground/sessions/%s after destruction: status=%d (expected non-200) body=%s",
		sessionID, resp.StatusCode, strings.TrimSpace(string(body)))

	// ── Step 7: Assert bare repo is gone from portal filesystem ───────────────
	// The destruction cascade step 8 calls Storage.RemoveRepo which runs
	// os.RemoveAll on the bare repo directory. A non-zero exit code from
	// `ls <path>` inside the container confirms the directory is absent.
	exitCode, execOut, execErr = p.Exec(ctx, []string{"ls", repoPath})
	require.NoErrorf(t, execErr, "portal docker exec ls %s (post-destruction): API error", repoPath)
	require.NotEqualf(t, 0, exitCode,
		"BARE REPO STILL PRESENT at %s after destruction (exit 0).\n"+
			"ls output: %s\n"+
			"Storage.RemoveRepo should have deleted this directory in destruction step 8.\n"+
			"If consistently reproducible, park as High: 'destruction cascade does not\n"+
			"remove bare repo from disk'.",
		repoPath, strings.TrimSpace(execOut))
	t.Logf("bare repo confirmed absent at %s after destruction (exit %d)", repoPath, exitCode)
}

// ---------------------------------------------------------------------------
// Helpers local to this test file
// ---------------------------------------------------------------------------

// playgroundCreated is a minimal decode target for the
// POST /api/playground/sessions 201 response (PlaygroundSessionCreated schema).
type playgroundCreated struct {
	Bearer   string `json:"bearer"`
	Nickname string `json:"nickname"`
	Session  struct {
		Id string `json:"id"`
	} `json:"session"`
}

// playgroundTombstone is a minimal decode target for
// GET /api/playground/sessions/{id}/tombstone (PlaygroundTombstone schema).
type playgroundTombstone struct {
	SessionID       string `json:"session_id"`
	OrgID           string `json:"org_id"`
	MembersCount    int    `json:"members_count"`
	CommitsCount    int    `json:"commits_count"`
	AutoMergesCount int    `json:"auto_merges_count"`
	DurationSeconds int64  `json:"duration_seconds"`
	EndReason       string `json:"end_reason"`
	EndedAt         string `json:"ended_at"`
	ExpiresAt       string `json:"expires_at"`
}

// createPlaygroundSession calls POST /api/playground/sessions and returns the
// decoded 201 body. No auth is required; the portal assigns an anonymous bearer.
func createPlaygroundSession(ctx context.Context, t *testing.T, baseURL string) playgroundCreated {
	t.Helper()
	url := strings.TrimRight(baseURL, "/") + "/api/playground/sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("createPlaygroundSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("createPlaygroundSession: POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("createPlaygroundSession: status %d (want 201): %s", resp.StatusCode, body)
	}

	var created playgroundCreated
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("createPlaygroundSession: decode: %v\nbody: %s", err, body)
	}
	if created.Session.Id == "" {
		t.Fatalf("createPlaygroundSession: empty session.id in response: %s", body)
	}
	if created.Bearer == "" {
		t.Fatalf("createPlaygroundSession: empty bearer in response: %s", body)
	}
	return created
}

// doGET performs a GET request with an optional bearer token and returns the
// response, body bytes, and any transport-level error. It does NOT fatal — the
// caller decides how to handle errors and status codes.
func doGET(ctx context.Context, url, bearer string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("doGET: build request: %w", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("doGET: do request: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp, nil, fmt.Errorf("doGET: read body: %w", readErr)
	}
	return resp, body, nil
}
