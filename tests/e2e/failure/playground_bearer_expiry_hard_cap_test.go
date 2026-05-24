// Invariant: the anonymous bearer issued by POST /api/playground/sessions is
// strictly scoped:
//
//  1. It is bound to exactly one session — using bearer B1 to access session S2
//     returns 401 (not a member), regardless of whether S2 is active.
//
//  2. It is revoked when the session is destroyed — after the clock advances
//     past the hard-cap and the destruction worker sweeps, the original bearer
//     must not return 200 with stale state. Both 401 (bearer revoked by the
//     cascade) and 404 (session row gone before the auth check) are acceptable;
//     200 is the bug.
//
//  3. It cannot perform OAuth-level organisation operations — the anon account
//     created by IssueAnonymousSessionBearer is not an org member, so any route
//     that requires org membership returns 403. Specifically: GET
//     /api/orgs/org_playground/members requires RequireOrgRole middleware; the
//     anon account holds session membership only, not org-level membership.
//     Response must be 403 (Forbidden), not 200.
//
// These properties are load-bearing because the anonymous bearer is the ONLY
// auth credential for playground users. A wiring bug (e.g. middleware ordering
// error, missing membership gate) could grant playground bearers unintended
// access to org data or let a destroyed session remain readable — neither of
// which is caught by unit tests against stubStorage.
package failure_test

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

// TestPlayground_Bearer_ScopeIsolation verifies the three scope properties of
// the anonymous playground bearer against the real portal binary.
func TestPlayground_Bearer_ScopeIsolation(t *testing.T) {
	ctx := context.Background()

	// ── Stack: cross-session isolation subtests ───────────────────────────────
	// A single portal with playground enabled. No clock manipulation here — the
	// cross-session and org-scope subtests don't require time travel.
	// HARD_CAP_S=600 is long enough that neither session expires during the test.
	// IDLE_TIMEOUT_S=300: same margin.
	// JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR=180 → perMinute=3, burst=3: allows
	// the 2 rapid-fire creates (S1, S2) without triggering the rate limiter.
	// The default of 3/hour (perMinute=1, burst=1) would block the second create.
	pg := postgres.Start(ctx, t, postgres.Options{})
	p := portal.Start(ctx, t, portal.Options{
		DBDriver: "postgres",
		DBDSN:    pg.ContainerDSN,
		ExtraEnv: map[string]string{
			"JAMSESH_PLAYGROUND_ENABLED":              "true",
			"JAMSESH_PLAYGROUND_HARD_CAP_S":           "600",
			"JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S":       "300",
			"JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR":   "180",
		},
	})

	// Create two independent playground sessions. Each call issues a fresh
	// anonymous bearer bound to that session only.
	s1 := pgCreate(ctx, t, p.URL)
	s2 := pgCreate(ctx, t, p.URL)
	t.Logf("session 1: id=%s bearer_prefix=%s", s1.Session.ID, s1.Bearer[:8])
	t.Logf("session 2: id=%s bearer_prefix=%s", s2.Session.ID, s2.Bearer[:8])

	// ── Subtest 1: cross-session bearer is rejected ───────────────────────────
	t.Run("cross_session_rejected", func(t *testing.T) {
		// Invariant: bearer B1 (issued for S1) cannot access S2.
		// The handler's GetPlaygroundSession checks: is acc.ID a session member
		// of the requested session? B1's account is a member of S1, not S2 →
		// the handler returns 401 with error "auth.not_a_member".
		//
		// This fires against the REAL membership table and REAL bearer store,
		// catching any middleware ordering bug that the unit suite's stubStore
		// cannot expose.
		resp, body, err := pgGET(ctx, p.URL+"/api/playground/sessions/"+s2.Session.ID, s1.Bearer)
		require.NoErrorf(t, err, "GET /api/playground/sessions/%s with S1 bearer: transport error", s2.Session.ID)
		require.Equalf(t, http.StatusUnauthorized, resp.StatusCode,
			"BEARER SCOPE VIOLATION: B1 accessed S2 (status %d).\n"+
				"body: %s\n\n"+
				"Expected: 401 — bearer B1 is a member of S1, not S2.\n"+
				"Got:      %d — if this is 200, the session membership check is broken.\n"+
				"Do NOT change this assertion; the mismatch is the bug.",
			resp.StatusCode, strings.TrimSpace(string(body)), resp.StatusCode)
		t.Logf("cross_session_rejected: S1 bearer correctly rejected on S2 (status=401, body=%s)",
			strings.TrimSpace(string(body)))
	})

	// ── Subtest 3: anon bearer cannot perform org-level operations ────────────
	// Note: subtests are ordered here so subtest 2 (which requires a SEPARATE
	// portal with a short hard-cap) can use its own stack without contaminating
	// the shared portal's clock offset.
	t.Run("anon_bearer_scoped_to_playground", func(t *testing.T) {
		// Invariant: the anonymous bearer's account holds session membership
		// only, not org-level membership. Routes requiring RequireOrgRole
		// middleware return 403 (not a member of this org). The anon account
		// is specifically not in the org_members table for org_playground —
		// only session_members. Confirmed: GET /api/me accepts anon bearers
		// (returns the synthetic anon account record). This subtest chooses
		// a route where the org-membership gate is the deciding factor.
		//
		// GET /api/orgs/{orgID}/members requires RequireOrgRole("creator","member").
		// The anon account (anon_* prefix) is a session member but NOT an org
		// member → 403 Forbidden.
		resp, body, err := pgGET(ctx, p.URL+"/api/orgs/org_playground/members", s1.Bearer)
		require.NoErrorf(t, err, "GET /api/orgs/org_playground/members with anon bearer: transport error")

		// We accept 401 OR 403 — both prove the anon bearer cannot list org
		// members. 401 would mean the bearer was rejected at the middleware
		// layer; 403 means the account was found but lacks org membership.
		// 200 is the bug: that would mean the anon bearer was granted org-
		// member access it was never intended to have.
		require.Containsf(t, []int{http.StatusUnauthorized, http.StatusForbidden}, resp.StatusCode,
			"SCOPE VIOLATION: anon bearer returned %d on org-member route.\n"+
				"body: %s\n\n"+
				"Expected: 401 or 403 — anon account is a session member only, not an org member.\n"+
				"Got:      %d — if this is 200, the org-membership gate is bypassed for anon bearers.\n"+
				"Do NOT widen this to accept 200; that would paper over a real security gap.",
			resp.StatusCode, strings.TrimSpace(string(body)), resp.StatusCode)
		t.Logf("anon_bearer_scoped_to_playground: correctly rejected (status=%d)", resp.StatusCode)
	})

	// ── Subtest 2: post-destruction bearer is revoked ─────────────────────────
	// Separate portal with a short hard-cap so AdvanceClock can blow past it
	// within the test. Shares neither clock state nor DB with the portal above.
	t.Run("post_destruction_revoked", func(t *testing.T) {
		// Infrastructure: short hard-cap (60s) + 1s sweep interval so the
		// destruction worker ticks quickly after the clock advance.
		// Idle timeout (300s) is larger than the hard-cap so the sweep
		// picks "hard_cap" as the reason (not "idle") — matching the golden
		// test's invariant for clock-advance-past-hard-cap.
		pg2 := postgres.Start(ctx, t, postgres.Options{})
		p2 := portal.Start(ctx, t, portal.Options{
			DBDriver: "postgres",
			DBDSN:    pg2.ContainerDSN,
			ExtraEnv: map[string]string{
				"JAMSESH_PLAYGROUND_ENABLED":                       "true",
				"JAMSESH_PLAYGROUND_HARD_CAP_S":                    "60",
				"JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S":                "300",
				"JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S":  "1",
				"JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR":            "180",
			},
		})

		// Create a session and capture its bearer.
		sess := pgCreate(ctx, t, p2.URL)
		sessionID := sess.Session.ID
		bearer := sess.Bearer
		t.Logf("post_destruction_revoked: session=%s bearer_prefix=%s", sessionID, bearer[:8])

		// Confirm the session is reachable before the clock advance (anti-
		// tautology guard: if it's already 401/404 we'd pass vacuously).
		preResp, preBody, preErr := pgGET(ctx, p2.URL+"/api/playground/sessions/"+sessionID, bearer)
		require.NoErrorf(t, preErr, "pre-advance GET session: transport error")
		require.Equalf(t, http.StatusOK, preResp.StatusCode,
			"pre-advance GET session must return 200 (session is alive).\n"+
				"body: %s\n"+
				"If this is already non-200, the bearer or session was not created correctly.",
			strings.TrimSpace(string(preBody)))
		t.Logf("post_destruction_revoked: session alive before clock advance (status=200)")

		// Advance clock 90s past the 60s hard-cap. The destruction worker's
		// ticker fires every 1s (real time); within ~2s of this call it will
		// query ListExpiredPlaygroundSessions with the injected Now() and sweep
		// the session. The sweep revokes the bearer as part of the cascade.
		p2.AdvanceClock(ctx, t, 90*time.Second)
		t.Logf("post_destruction_revoked: clock advanced 90s past hard-cap; polling tombstone")

		// Poll the tombstone endpoint until it returns 200 (sweep fired) or
		// the deadline elapses. The tombstone is public (no auth required).
		// We poll up to 10s — that is ~10 sweep cycles at 1s interval.
		const pollDeadline = 10 * time.Second
		const pollInterval = 200 * time.Millisecond
		deadline := time.Now().Add(pollDeadline)
		tombstoneFound := false
		for time.Now().Before(deadline) {
			tr, _, terr := pgGET(ctx, p2.URL+"/api/playground/sessions/"+sessionID+"/tombstone", "")
			if terr != nil {
				t.Logf("tombstone poll: transport error (retrying): %v", terr)
				time.Sleep(pollInterval)
				continue
			}
			if tr.StatusCode == http.StatusOK {
				tombstoneFound = true
				t.Logf("post_destruction_revoked: tombstone appeared (sweep confirmed)")
				break
			}
			time.Sleep(pollInterval)
		}

		if !tombstoneFound {
			t.Fatalf(
				"DESTRUCTION WORKER DID NOT SWEEP within %s after 90s clock advance.\n"+
					"Session %s: HARD_CAP_S=60, SWEEP_INTERVAL_S=1.\n"+
					"Expected: GET /tombstone returns 200 within ~2 sweep ticks.\n"+
					"Actual:   tombstone never appeared.\n\n"+
					"If consistently reproducible, this is a production bug in the\n"+
					"clock-inject → destruction-worker interaction. Park via\n"+
					"/agile-workflow:park with severity=High tag=playground.",
				pollDeadline, sessionID,
			)
		}

		// Now verify the original bearer is no longer usable.
		// After the destruction cascade:
		//   • The bearer row is revoked (RevokeOAuthToken called on all session bearers).
		//   • BearerMiddleware calls svc.Validate → ErrRevokedToken → 401.
		//   • If somehow the bearer were still valid, the session row may be gone → 404.
		// Either 401 or 404 proves the session is not accessible with the old bearer.
		// 200 is the bug: stale session state served after destruction.
		postResp, postBody, postErr := pgGET(ctx, p2.URL+"/api/playground/sessions/"+sessionID, bearer)
		require.NoErrorf(t, postErr, "post-destruction GET session: transport error")
		require.Containsf(t, []int{http.StatusUnauthorized, http.StatusNotFound}, postResp.StatusCode,
			"BEARER REVOCATION FAILURE: destroyed session returned %d.\n"+
				"body: %s\n\n"+
				"Expected: 401 (bearer revoked by destruction cascade) or 404 (session gone).\n"+
				"Got:      %d — if this is 200, the bearer-revocation step in the destruction\n"+
				"cascade is broken. The unit test TestDestruction_BearersRevoked uses stubStorage\n"+
				"and cannot catch a real DB cascade failure.\n"+
				"Do NOT widen this assertion to accept 200; that is the product bug, not a test gap.",
			postResp.StatusCode, strings.TrimSpace(string(postBody)), postResp.StatusCode)
		t.Logf("post_destruction_revoked: bearer correctly rejected after destruction (status=%d)",
			postResp.StatusCode)
	})
}

// ---------------------------------------------------------------------------
// Helpers private to this file
// ---------------------------------------------------------------------------

// pgCreateResponse is the 201 body of POST /api/playground/sessions.
type pgCreateResponse struct {
	Bearer   string `json:"bearer"`
	Nickname string `json:"nickname"`
	Session  struct {
		ID    string `json:"id"`
		OrgID string `json:"org_id"`
	} `json:"session"`
	ExpiresAt string `json:"expires_at"`
}

// pgCreate calls POST /api/playground/sessions and returns the parsed 201 body.
// Fails the test on any non-201 status or decode error.
func pgCreate(ctx context.Context, t *testing.T, baseURL string) pgCreateResponse {
	t.Helper()
	url := strings.TrimRight(baseURL, "/") + "/api/playground/sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("pgCreate: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("pgCreate: POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("pgCreate: status %d (want 201): %s", resp.StatusCode, body)
	}

	var r pgCreateResponse
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("pgCreate: decode response: %v\nbody: %s", err, body)
	}
	if r.Session.ID == "" {
		t.Fatalf("pgCreate: empty session.id in response: %s", body)
	}
	if r.Bearer == "" {
		t.Fatalf("pgCreate: empty bearer in response: %s", body)
	}
	return r
}

// pgGET performs a GET request with an optional bearer and returns the response,
// body, and transport-level error. It drains and closes the response body.
// The caller does NOT need to close the body.
func pgGET(ctx context.Context, url, bearer string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("pgGET: build request: %w", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("pgGET: do request: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp, nil, fmt.Errorf("pgGET: read body: %w", readErr)
	}
	return resp, body, nil
}
