// Invariant: A push that arrives at the portal while the destruction worker is
// about to (or has just) destroyed the session must either complete cleanly
// before destruction OR fail cleanly afterward. Either way, no torn state:
// no half-deleted bare repo, no orphaned tombstone, no stuck "ending" session.
//
// Chaos mechanism: start a git push goroutine, then immediately advance the
// portal clock past the hard-cap. The push is in flight when the destruction
// sweep fires (within 0-1s real time). Because the sweep's ticker phase is
// non-deterministic — the tick may fire in 0-1s — sometimes the push
// completes before destruction (push-wins), sometimes after (destroy-wins).
// Running 5 iterations exercises both orderings.
//
// Permitted outcomes (invariant holds if exactly one of these is true):
//
//  A. Push wins: push exited 0 AND session is not yet destroyed (tombstone
//     absent) after push. Eventually the sweep fires and commits_count in
//     the tombstone is >= 1 (the pushed commit is counted).
//
//  B. Destruction wins: push exited non-zero (portal returned 401/403/500
//     or git transport failure) AND tombstone exists AND session row is
//     gone AND bare repo is absent from disk.
//
// Forbidden outcome: push exited 0 (portal returned 2xx) AND the tombstone
// shows commits_count = 0 after destruction completes. That would be a
// silent data-loss bug: the portal claimed success but the commit was
// silently discarded.
//
// Per test-integrity discipline: if a forbidden outcome is observed reliably,
// park it as a High production bug before modifying any assertion.
package chaos_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestPlayground_DestructionDuringPush covers two scenarios that together
// verify the full invariant from both orderings:
//
//   - push_before_destroy (deterministic): push completes, then sweep fires.
//     Verifies that the tombstone's commits_count is >= 1 and the repo is
//     removed after destruction — no silent data loss.
//
//   - concurrent_race (5 iterations): push goroutine races concurrently with
//     the sweep. On fast hardware the sweep reliably wins because it can fire
//     and revoke the bearer between git's two auth probes (unauthenticated
//     challenge + authenticated retry); on slower systems the push may
//     sometimes win. Both outcomes are valid; the test asserts no torn state
//     in either case. Running 5 iterations amplifies the chance of observing
//     both orderings, and confirms the destroy-wins path is deterministically
//     clean when the sweep wins.
//
// Stack config (shared across both sub-tests via the same portal+postgres):
//   - HARD_CAP_S=60: bearer TTL = 60s real time (enough margin for setup).
//   - SWEEP_INTERVAL_S=1: ticker fires every 1s real time.
//   - IDLE_TIMEOUT_S=3600: only hard_cap reason fires.
//   - CREATE_PER_IP_HOUR=3600: high enough for 6+ back-to-back creates.
func TestPlayground_DestructionDuringPush(t *testing.T) {
	requireDocker(t)
	requirePortalImage(t)

	ctx := context.Background()

	// One portal+postgres stack for all sub-tests. The clock is cumulative and
	// forward-only. push_before_destroy advances by 70s. Each concurrent_race
	// iteration also advances by 70s. Total advance: 70s + 5*70s = 420s.
	// Sessions in later iterations use the higher offset as their creation
	// baseline; each AdvanceClock(70s) brings the session 10s past its
	// hard_cap_at, so it is always eligible for destruction.
	pg := postgres.Start(ctx, t, postgres.Options{})
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		ExtraEnv: map[string]string{
			"JAMSESH_PLAYGROUND_ENABLED": "true",
			// HARD_CAP_S=60: IssueAnonymousSessionBearer sets bearer TTL = HardCap
			// (real time). 60s gives ~50s of margin for git clone + commit + push
			// setup before the bearer expires naturally. We advance the portal clock
			// by 70s (10s past hard-cap) to trigger destruction while keeping the
			// bearer valid in real time.
			"JAMSESH_PLAYGROUND_HARD_CAP_S":                   "60",
			"JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S":               "3600",
			"JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S": "1",
			// perMinute = ceil(3600/60) = 60; burst = 60. Allows 60 creates in the
			// first minute — more than enough for 6 back-to-back test sessions.
			"JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR": "3600",
		},
	})

	// ── Sub-test 1: push completes before destruction ─────────────────────────
	// Deterministic push-wins scenario: push first (no clock advance yet), then
	// trigger destruction. Verifies that commits_count >= 1 in the tombstone.
	t.Run("push_before_destroy", func(t *testing.T) {
		// ── 1a. Create session and confirm repo exists ──────────────────────────
		created := dpushCreateSession(ctx, t, p.URL)
		sessionID := created.Session.ID
		bearer := created.Bearer
		t.Logf("push_before_destroy: session=%s", sessionID)

		repoPath := "/tmp/jamsesh-repos/orgs/org_playground/sessions/" + sessionID + ".git"
		code, out, err := p.Exec(ctx, []string{"ls", repoPath})
		require.NoError(t, err, "push_before_destroy: docker exec ls %s: API error", repoPath)
		require.Equal(t, 0, code,
			"push_before_destroy: bare repo not found at %s (ls exit %d): %s",
			repoPath, code, strings.TrimSpace(out))

		// ── 1b. Push a commit (no clock advance yet) ────────────────────────────
		// The push must succeed cleanly. This is the pre-chaos baseline: if the
		// push fails here, the stack is misconfigured and the chaos assertion
		// would be meaningless.
		accountID := dpushGetAccountID(ctx, t, p.URL, bearer)
		ref := "jam/" + sessionID + "/" + accountID + "/main"
		repoDir := dpushCloneAndCommit(ctx, t, p.URL, sessionID, accountID, bearer, 0)

		pushCmd := exec.CommandContext(ctx, "git", "push", "origin", "HEAD:refs/heads/"+ref)
		pushCmd.Dir = repoDir
		pushOut, pushErr := pushCmd.CombinedOutput()
		require.NoError(t, pushErr,
			"push_before_destroy: pre-chaos push must succeed; output: %s", pushOut)
		t.Logf("push_before_destroy: push succeeded (ref=%s)", ref)

		// ── 1c. Advance clock past hard-cap, wait for tombstone ─────────────────
		p.AdvanceClock(ctx, t, 70*time.Second)
		t.Logf("push_before_destroy: clock advanced 70s past hard-cap")

		var tomb dpushTombstone
		require.Eventually(t,
			func() bool {
				resp, body, err := dpushGET(ctx, p.URL+"/api/playground/sessions/"+sessionID+"/tombstone", "")
				if err != nil || resp.StatusCode != http.StatusOK {
					return false
				}
				return json.Unmarshal(body, &tomb) == nil && tomb.SessionID != ""
			},
			10*time.Second, 300*time.Millisecond,
			"push_before_destroy: tombstone did not appear within 10s after clock advance. "+
				"Session=%s. Destruction worker may be stuck.", sessionID,
		)
		t.Logf("push_before_destroy: tombstone: end_reason=%s commits_count=%d", tomb.EndReason, tomb.CommitsCount)

		// ── 1d. Assert tombstone integrity ──────────────────────────────────────
		// CRITICAL: commits_count must be >= 1. The push succeeded (exited 0),
		// so the portal accepted the commit. The tombstone must reflect this.
		// commits_count=0 after a successful push is silent data loss.
		require.GreaterOrEqual(t, tomb.CommitsCount, 1,
			"push_before_destroy: SILENT DATA LOSS — push succeeded but tombstone commits_count=%d. "+
				"The portal accepted the push but did not count the commit. "+
				"Do NOT weaken this assertion — park as High if consistently reproducible.",
			tomb.CommitsCount,
		)
		require.Equal(t, "hard_cap", tomb.EndReason,
			"push_before_destroy: end_reason must be 'hard_cap' (IDLE_TIMEOUT_S >> HARD_CAP_S)")

		// Session must be inaccessible (bearer revoked by cascade step 4).
		resp, _, err := dpushGET(ctx, p.URL+"/api/playground/sessions/"+sessionID, bearer)
		require.NoError(t, err, "push_before_destroy: GET session: transport error")
		require.NotEqual(t, http.StatusOK, resp.StatusCode,
			"push_before_destroy: TORN STATE — session still returns 200 after tombstone appears")

		// Repo must be gone from disk (cascade step 8).
		require.Eventually(t,
			func() bool {
				code, _, err := p.Exec(ctx, []string{"ls", repoPath})
				return err == nil && code != 0
			},
			5*time.Second, 300*time.Millisecond,
			"push_before_destroy: TORN STATE — bare repo still present at %s after tombstone", repoPath,
		)
		t.Logf("push_before_destroy: PASS — commits_count=%d, repo removed, session inaccessible",
			tomb.CommitsCount)
	})

	// ── Sub-test 2: concurrent race — 5 iterations ────────────────────────────
	// Push goroutine starts concurrently with the clock advance. The sweep fires
	// within 0-1s; the push takes ~100-400ms. Both orderings are valid. On fast
	// hardware the sweep tends to win by revoking the bearer between git's
	// unauthenticated probe and its authenticated retry. On slower hardware the
	// push may occasionally win. Either way, no torn state is the assertion.
	for i := 0; i < 5; i++ {
		i := i // capture for goroutine
		t.Run(fmt.Sprintf("concurrent_race/iter_%02d", i+1), func(t *testing.T) {
			// ── Create session ──────────────────────────────────────────────────
			created := dpushCreateSession(ctx, t, p.URL)
			sessionID := created.Session.ID
			bearer := created.Bearer

			require.NotEmpty(t, sessionID, "session ID must be non-empty")
			t.Logf("iter %02d: session=%s bearer_prefix=%s", i+1, sessionID, bearer[:10])

			repoPath := "/tmp/jamsesh-repos/orgs/org_playground/sessions/" + sessionID + ".git"
			code, out, err := p.Exec(ctx, []string{"ls", repoPath})
			require.NoError(t, err, "iter %d: docker exec ls %s: API error", i+1, repoPath)
			require.Equal(t, 0, code,
				"iter %d: bare repo not at %s (ls exit %d): %s",
				i+1, repoPath, code, strings.TrimSpace(out))

			// ── Clone + prepare commit (no push yet) ────────────────────────────
			accountID := dpushGetAccountID(ctx, t, p.URL, bearer)
			ref := "jam/" + sessionID + "/" + accountID + "/main"
			repoDir := dpushCloneAndCommit(ctx, t, p.URL, sessionID, accountID, bearer, i+1)
			t.Logf("iter %02d: commit prepared, ref=%s", i+1, ref)

			// ── Start push goroutine, then advance clock ─────────────────────────
			// The goroutine is spawned BEFORE the clock advance so the push can
			// get its first HTTP request out before the sweep fires.
			//
			// Note: on fast hardware, the sweep may fire before the goroutine's
			// authenticated info/refs request arrives (because git makes two
			// info/refs calls — unauthenticated challenge, then authenticated
			// retry — and the sweep can fire between them). destroy-wins is the
			// expected dominant outcome on local/CI hardware. push-wins may occur
			// on slower hosts or when the ticker phase happens to give the push
			// more than ~300ms of headroom.
			//
			// gitclient.Push is NOT used here because it calls t.Fatal on
			// non-zero exit. A non-zero exit (401 after bearer revocation) is
			// a valid "destruction wins" outcome and must not fail the test.
			type pushResult struct {
				exitErr error
				output  []byte
			}
			pushDone := make(chan pushResult, 1)
			go func() {
				cmd := exec.CommandContext(ctx, "git", "push", "origin", "HEAD:refs/heads/"+ref)
				cmd.Dir = repoDir
				out, err := cmd.CombinedOutput()
				pushDone <- pushResult{exitErr: err, output: out}
			}()

			p.AdvanceClock(ctx, t, 70*time.Second)
			t.Logf("iter %02d: clock advanced 70s past hard-cap; goroutine racing with sweep", i+1)

			// ── Collect push result ──────────────────────────────────────────────
			var pr pushResult
			select {
			case pr = <-pushDone:
			case <-time.After(15 * time.Second):
				t.Fatalf("iter %02d: git push goroutine did not return within 15s — "+
					"possible deadlock between portal receive-pack and destruction cascade. "+
					"Park as High if consistently reproducible: "+
					"'destruction cascade deadlocks git push transport'.",
					i+1)
			}

			pushSucceeded := pr.exitErr == nil
			t.Logf("iter %02d: push outcome: success=%v output=%s",
				i+1, pushSucceeded, strings.TrimSpace(string(pr.output)))

			// ── Assert no torn state ─────────────────────────────────────────────
			dpushAssertNoTornState(ctx, t, p, sessionID, bearer, repoPath, pushSucceeded, i+1)
		})
	}
}

// dpushAssertNoTornState polls the session and tombstone endpoints and asserts
// that the system is in one of the two permitted non-torn states:
//
// If pushSucceeded:
//   - The push was ACK'd (push exited 0). The session may still be alive
//     (destruction not yet complete) or already destroyed (destruction
//     completed while the push was in flight but after the commit was
//     recorded). Either sub-case is valid. HOWEVER: once the tombstone
//     appears, commits_count MUST be >= 1 — the pushed commit must be
//     counted. A tombstone with commits_count=0 after a successful push
//     is the forbidden "silent data loss" outcome.
//
// If !pushSucceeded:
//   - The push was rejected (push exited non-zero). The portal correctly
//     rejected it (bearer revoked or session gone). Destruction must
//     eventually complete: tombstone appears AND session gone AND repo gone.
//     The forbidden outcome here is "push rejected but repo or session
//     still exists after tomb appears" (partial destruction).
func dpushAssertNoTornState(
	ctx context.Context,
	t *testing.T,
	p *portal.Portal,
	sessionID, bearer, repoPath string,
	pushSucceeded bool,
	iterNum int,
) {
	t.Helper()

	const pollDeadline = 12 * time.Second
	const pollInterval = 300 * time.Millisecond

	if pushSucceeded {
		// Push won the race or destruction hasn't fired yet.
		// Poll until the tombstone appears (destruction always fires eventually
		// because the session is past hard-cap). Then assert commits_count >= 1.
		t.Logf("iter %02d: push succeeded; waiting for destruction sweep to complete (up to %s)",
			iterNum, pollDeadline)

		var tomb dpushTombstone
		require.Eventually(t,
			func() bool {
				resp, body, err := dpushGET(ctx, p.URL+"/api/playground/sessions/"+sessionID+"/tombstone", "")
				if err != nil || resp.StatusCode != http.StatusOK {
					return false
				}
				if err := json.Unmarshal(body, &tomb); err != nil {
					return false
				}
				return tomb.SessionID != ""
			},
			pollDeadline, pollInterval,
			"iter %02d: tombstone did not appear within %s after push succeeded — "+
				"destruction worker may be stuck. Session=%s. "+
				"If consistently reproducible, park as High: 'destruction sweep does not "+
				"fire for session past hard-cap after successful push'.",
			iterNum, pollDeadline, sessionID,
		)

		t.Logf("iter %02d: tombstone appeared: end_reason=%s members_count=%d commits_count=%d",
			iterNum, tomb.EndReason, tomb.MembersCount, tomb.CommitsCount)

		// CRITICAL: commits_count must be >= 1.
		// A successful push (push exited 0) means the portal returned 2xx to
		// the git client, which means the pre-receive hook accepted the push
		// and the commit was recorded in the portal's event log. The tombstone
		// must count it. commits_count=0 here is a silent data-loss bug.
		//
		// Per test-integrity rules: do NOT weaken this assertion. If it fires
		// reproducibly, park via /agile-workflow:park before modifying.
		require.GreaterOrEqualf(t, tomb.CommitsCount, 1,
			"iter %02d: SILENT DATA LOSS — push exited 0 (portal returned 2xx) but "+
				"tombstone commits_count=%d (expected >= 1). "+
				"The portal accepted the push but did not count the commit in the "+
				"destruction summary. This is an RPO violation. "+
				"Do NOT change this assertion — the mismatch IS the bug. "+
				"Park via /agile-workflow:park with severity=High.",
			iterNum, tomb.CommitsCount,
		)

		// Session must be inaccessible after tombstone appears (bearer revoked
		// or session row gone — either 401 or 404 is correct).
		resp, _, err := dpushGET(ctx, p.URL+"/api/playground/sessions/"+sessionID, bearer)
		require.NoError(t, err, "iter %02d: GET /api/playground/sessions/%s: transport error", iterNum, sessionID)
		require.NotEqualf(t, http.StatusOK, resp.StatusCode,
			"iter %02d: session returned 200 after tombstone appeared — session row was NOT cleaned up. "+
				"Tombstone and live session must not coexist. "+
				"This is a torn state: park as High before modifying this assertion.",
			iterNum,
		)
		t.Logf("iter %02d: session confirmed inaccessible after tombstone (status=%d)", iterNum, resp.StatusCode)

		// Bare repo must be gone after tombstone appears (destruction step 8).
		// The tombstone's appearance proves step 3 (record tombstone) ran, so
		// step 8 (RemoveRepo) should also have completed (or will retry soon).
		// We poll briefly for the repo to disappear after tombstone, allowing
		// for the small latency between step 3 and step 8 in the cascade.
		require.Eventually(t,
			func() bool {
				code, _, err := p.Exec(ctx, []string{"ls", repoPath})
				return err == nil && code != 0
			},
			5*time.Second, 300*time.Millisecond,
			"iter %02d: TORN STATE — bare repo still present at %s after tombstone appeared. "+
				"Destruction step 8 (RemoveRepo) did not complete. "+
				"If consistently reproducible, park as High: 'destruction cascade leaves repo on disk'.",
			iterNum, repoPath,
		)
		t.Logf("iter %02d: PASS (push-wins) — push ACK'd, tombstone commits_count=%d, repo removed",
			iterNum, tomb.CommitsCount)

	} else {
		// Destruction won the race. Push was correctly rejected.
		// The tombstone must appear (destruction completed), the session must
		// be gone, and the bare repo must be absent.
		t.Logf("iter %02d: push rejected (destruction won race); waiting for tombstone (up to %s)",
			iterNum, pollDeadline)

		var tomb dpushTombstone
		require.Eventually(t,
			func() bool {
				resp, body, err := dpushGET(ctx, p.URL+"/api/playground/sessions/"+sessionID+"/tombstone", "")
				if err != nil || resp.StatusCode != http.StatusOK {
					return false
				}
				if err := json.Unmarshal(body, &tomb); err != nil {
					return false
				}
				return tomb.SessionID != ""
			},
			pollDeadline, pollInterval,
			"iter %02d: push was rejected but tombstone never appeared within %s. "+
				"If destruction won the race it must complete the cascade. "+
				"Session=%s. Park as High if consistently reproducible.",
			iterNum, pollDeadline, sessionID,
		)

		t.Logf("iter %02d: tombstone: end_reason=%s members=%d commits=%d",
			iterNum, tomb.EndReason, tomb.MembersCount, tomb.CommitsCount)

		// After a rejected push, commits_count should be 0 OR it could be >= 1
		// if the push actually completed its pre-receive processing before the
		// rejection was returned (portal counted it then later revoked the bearer).
		// Both are valid — what we cannot accept is commits_count < 0 or a
		// type error. We don't enforce a specific count here.

		// Session must be inaccessible.
		resp, _, err := dpushGET(ctx, p.URL+"/api/playground/sessions/"+sessionID, bearer)
		require.NoError(t, err, "iter %02d: GET /api/playground/sessions/%s: transport error", iterNum, sessionID)
		require.NotEqualf(t, http.StatusOK, resp.StatusCode,
			"iter %02d: TORN STATE — session returned 200 after push rejection AND tombstone exists. "+
				"Tombstone and live session must not coexist. "+
				"Park as High before modifying this assertion.",
			iterNum,
		)
		t.Logf("iter %02d: session confirmed inaccessible (status=%d)", iterNum, resp.StatusCode)

		// Bare repo must be absent.
		require.Eventually(t,
			func() bool {
				code, _, err := p.Exec(ctx, []string{"ls", repoPath})
				return err == nil && code != 0
			},
			5*time.Second, 300*time.Millisecond,
			"iter %02d: TORN STATE — bare repo still present at %s after tombstone appeared AND push rejected. "+
				"RemoveRepo (destruction step 8) did not complete. "+
				"Park as High if consistently reproducible.",
			iterNum, repoPath,
		)
		t.Logf("iter %02d: PASS (destroy-wins) — push rejected, tombstone present, repo removed",
			iterNum)
	}
}

// ---------------------------------------------------------------------------
// HTTP helpers local to this file
// ---------------------------------------------------------------------------

// dpushCreateResp is a minimal decode target for POST /api/playground/sessions.
type dpushCreateResp struct {
	Session struct {
		ID    string `json:"id"`
		OrgID string `json:"org_id"`
	} `json:"session"`
	Bearer string `json:"bearer"`
}

// dpushTombstone is a minimal decode target for GET .../tombstone.
type dpushTombstone struct {
	SessionID    string `json:"session_id"`
	MembersCount int    `json:"members_count"`
	CommitsCount int    `json:"commits_count"`
	EndReason    string `json:"end_reason"`
}

// dpushCreateSession calls POST /api/playground/sessions and returns the
// decoded 201 body. Fails the test on any non-201 response.
func dpushCreateSession(ctx context.Context, t *testing.T, baseURL string) dpushCreateResp {
	t.Helper()
	url := strings.TrimRight(baseURL, "/") + "/api/playground/sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("dpushCreateSession: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("dpushCreateSession: POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("dpushCreateSession: status %d (want 201): %s", resp.StatusCode, body)
	}
	var r dpushCreateResp
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("dpushCreateSession: decode: %v\nbody: %s", err, body)
	}
	if r.Session.ID == "" {
		t.Fatalf("dpushCreateSession: empty session.id in response: %s", body)
	}
	if r.Bearer == "" {
		t.Fatalf("dpushCreateSession: empty bearer in response: %s", body)
	}
	return r
}

// dpushGetAccountID calls GET /api/me with the anonymous bearer and returns
// the account ID. The anonymous bearer is accepted by /api/me because the
// BearerMiddleware supports anonymous tokens.
func dpushGetAccountID(ctx context.Context, t *testing.T, baseURL, bearer string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(baseURL, "/")+"/api/me", nil)
	if err != nil {
		t.Fatalf("dpushGetAccountID: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("dpushGetAccountID: GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dpushGetAccountID: status %d (want 200): %s", resp.StatusCode, body)
	}
	var me struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("dpushGetAccountID: decode: %v\nbody: %s", err, body)
	}
	if me.ID == "" {
		t.Fatalf("dpushGetAccountID: empty id in /api/me response: %s", body)
	}
	return me.ID
}

// dpushCloneAndCommit clones the playground session repo, writes a file, and
// commits it (with required Jam-* trailers) — but does NOT push. Returns the
// working tree directory, ready for a subsequent git push.
//
// The bearer is embedded in the remote URL for credential-less push. The
// commit message includes the iteration number so log output is traceable.
func dpushCloneAndCommit(
	ctx context.Context,
	t *testing.T,
	portalURL, sessionID, accountID, bearer string,
	iterIdx int,
) string {
	t.Helper()

	dir := t.TempDir()

	// Build git clone URL with embedded credentials.
	u, err := url.Parse(portalURL)
	if err != nil {
		t.Fatalf("dpushCloneAndCommit: parse portal URL %q: %v", portalURL, err)
	}
	u.User = url.UserPassword("x-access-token", bearer)
	repoURL := u.String() + "/git/org_playground/" + sessionID + ".git"

	gitRun(ctx, t, "", "git", "clone", repoURL, dir)
	gitRun(ctx, t, dir, "git", "config", "user.email", accountID+"@chaos.example")
	gitRun(ctx, t, dir, "git", "config", "user.name", "Chaos Test")

	// Write a test file into the working tree.
	fileName := fmt.Sprintf("chaos-iter-%02d.md", iterIdx+1)
	content := fmt.Sprintf("# Chaos iteration %d\nThis file tests the destruction-during-push race.\n", iterIdx+1)
	absPath := filepath.Join(dir, fileName)
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatalf("dpushCloneAndCommit: write %s: %v", absPath, err)
	}
	gitRun(ctx, t, dir, "git", "add", fileName)

	turnID := uuid.New().String()
	commitMsg := fmt.Sprintf(
		"chaos: destruction-during-push iter %d\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
		iterIdx+1, sessionID, turnID, accountID,
	)
	msgFile := filepath.Join(t.TempDir(), "commit-msg")
	if err := os.WriteFile(msgFile, []byte(commitMsg), 0o644); err != nil {
		t.Fatalf("dpushCloneAndCommit: write message file: %v", err)
	}
	gitRun(ctx, t, dir, "git", "commit", "-F", msgFile)

	return dir
}

// dpushGET performs a GET with an optional bearer and returns the response,
// body, and any transport-level error. It does NOT fatal — callers decide.
func dpushGET(ctx context.Context, rawURL, bearer string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("dpushGET: build request: %w", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("dpushGET: do request: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp, nil, fmt.Errorf("dpushGET: read body: %w", readErr)
	}
	return resp, body, nil
}

// gitRun runs a git command in dir, fataling the test on any error.
// Delegates to exec.CommandContext directly (not gitclient.run, which is
// unexported) to avoid import coupling.
func gitRun(ctx context.Context, t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dpush/gitRun: %s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}
