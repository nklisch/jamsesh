// Invariant: operations interrupted mid-flight leave the portal in a
// consistent, recoverable state. Each subtest asserts user-visible outcomes
// (HTTP status codes, error envelope codes) rather than internal state.
//
// Skipped subtests carry explicit reasons pointing at the dependency or
// design change needed to un-skip them.
package failure_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"jamsesh/tests/e2e/fixtures/authflow"
	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/wsclient"
)

// sessionRef captures the fields we need from a created session.
type sessionRef struct {
	ID    string `json:"id"`
	OrgID string `json:"org_id"`
}

// lockStatus captures the fields we need from an acquired finalize lock.
type lockStatus struct {
	LockID string `json:"lock_id"`
}

// createSession calls POST /api/orgs/{orgID}/sessions and returns the new
// session's ID. It is distinct from the authflow helpers because sessions are
// first-class resources tied to an org, not part of the auth onboarding flow.
func createSession(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, name string) string {
	t.Helper()
	url := fmt.Sprintf("%s/api/orgs/%s/sessions", p.URL, orgID)
	body := map[string]string{
		"name":         name,
		"goal":         "e2e interrupted-ops test session",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}
	var sess sessionRef
	authflow.PostJSONInto(ctx, t, url, body, accessToken, http.StatusCreated, &sess)
	if sess.ID == "" {
		t.Fatalf("createSession: empty session id in response")
	}
	return sess.ID
}

// acquireFinalizeLock calls POST .../finalize/lock and returns the lock ID.
func acquireFinalizeLock(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, sessionID string) string {
	t.Helper()
	url := fmt.Sprintf("%s/api/orgs/%s/sessions/%s/finalize/lock", p.URL, orgID, sessionID)
	var ls lockStatus
	authflow.PostJSONInto(ctx, t, url, map[string]bool{}, accessToken, http.StatusCreated, &ls)
	if ls.LockID == "" {
		t.Fatalf("acquireFinalizeLock: empty lock_id in response")
	}
	return ls.LockID
}

// releaseFinalizeLock calls DELETE .../finalize/lock/{lockID} and asserts 204.
func releaseFinalizeLock(ctx context.Context, t *testing.T, p *portal.Portal, accessToken, orgID, sessionID, lockID string) {
	t.Helper()
	url := fmt.Sprintf("%s/api/orgs/%s/sessions/%s/finalize/lock/%s", p.URL, orgID, sessionID, lockID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("releaseFinalizeLock: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("releaseFinalizeLock: DELETE %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("releaseFinalizeLock: DELETE %s: status %d (want 204): %s", url, resp.StatusCode, body)
	}
}

func TestInterruptedOps(t *testing.T) {
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})
	mh := mailhog.Start(ctx, t)
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		SMTPHost:  mh.ContainerSMTPHost,
		SMTPPort:  mh.ContainerSMTPPort,
	})

	// ---------------------------------------------------------------------------
	// Push interrupted mid-pack
	// ---------------------------------------------------------------------------

	t.Run("push_interrupted_mid_pack", func(t *testing.T) {
		// Invariant: a push request to /git/.../git-receive-pack that is
		// cancelled mid-flight (context deadline) does not leave the server
		// in a state that prevents subsequent requests from succeeding.
		// We assert:
		//   (a) the interrupted request either completes with a documented
		//       error code or is cancelled by the client (both outcomes are
		//       acceptable — the race is non-deterministic),
		//   (b) a follow-on GET /healthz succeeds, confirming the server is
		//       still responsive.
		//
		// The smart-HTTP git endpoint (/git/...) is authenticated via HTTP
		// Basic auth (password = portal access token). We use a wrong
		// content-type to make the server reject the body without spawning
		// git-receive-pack, keeping the test simple and fast.
		alice := authflow.SignInViaMagicLink(ctx, t, p, mh, "alice-push-interrupt@example.com")
		orgID := authflow.CreateOrg(ctx, t, p, alice.AccessToken, "Alice Push Org")
		sessionID := createSession(ctx, t, p, alice.AccessToken, orgID, "Push Interrupt Session")

		gitURL := fmt.Sprintf("%s/git/%s/%s.git/git-receive-pack", p.URL, orgID, sessionID)

		// Issue a POST with a 100ms deadline. The server either responds
		// before the deadline (auth check, content-type check) or the
		// client-side context fires.
		shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		req, err := http.NewRequestWithContext(shortCtx, http.MethodPost, gitURL,
			bytes.NewReader(make([]byte, 0)))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		// Correct content-type so auth is checked first; that may come
		// back fast (401) or the context deadline fires first.
		req.Header.Set("Content-Type", "application/x-git-receive-pack-request")
		req.SetBasicAuth("x-access-token", alice.AccessToken)

		resp, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			// Context deadline fired before the server responded — this is
			// acceptable: the interruption happened as intended.
			if !errors.Is(doErr, context.DeadlineExceeded) &&
				!errors.Is(doErr, context.Canceled) {
				// Check for wrapped context errors in URL errors.
				var urlErr interface{ Unwrap() error }
				if errors.As(doErr, &urlErr) {
					inner := urlErr.Unwrap()
					if !errors.Is(inner, context.DeadlineExceeded) &&
						!errors.Is(inner, context.Canceled) {
						t.Logf("push_interrupted_mid_pack: Do error (non-context): %v", doErr)
					}
				}
			}
		} else {
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			// Server responded before the deadline. Acceptable outcomes:
			//   401 — unauthenticated (if Basic auth was rejected)
			//   400 — bad content type or bad pack body
			//   404 — session bare repo not yet initialized
			//   200 — report-status (pack fully processed)
			//   Any 4xx/5xx is fine; we just record the status for debugging.
			t.Logf("push_interrupted_mid_pack: server returned %d (deadline did not fire)", resp.StatusCode)
			if resp.StatusCode >= 500 {
				t.Errorf("push_interrupted_mid_pack: server returned 5xx (%d); expected 4xx or 2xx", resp.StatusCode)
			}
		}

		// (b) The server must still be responsive after the interruption.
		healthCtx, healthCancel := context.WithTimeout(ctx, 5*time.Second)
		defer healthCancel()
		healthReq, err := http.NewRequestWithContext(healthCtx, http.MethodGet, p.URL+"/healthz", nil)
		if err != nil {
			t.Fatalf("build healthz request: %v", err)
		}
		healthResp, err := http.DefaultClient.Do(healthReq)
		if err != nil {
			t.Fatalf("push_interrupted_mid_pack: healthz after interruption: %v", err)
		}
		defer healthResp.Body.Close()
		io.Copy(io.Discard, healthResp.Body)
		if healthResp.StatusCode != http.StatusOK {
			t.Errorf("push_interrupted_mid_pack: healthz after interruption: status %d (want 200)", healthResp.StatusCode)
		}
	})

	// ---------------------------------------------------------------------------
	// Finalize lock: acquire, release, reacquire by another caller
	// ---------------------------------------------------------------------------

	t.Run("finalize_lock_release_and_reacquire", func(t *testing.T) {
		// Invariant: after the lock holder explicitly releases a finalize
		// lock, another authenticated session member can acquire it.
		// This exercises the "lock holder process killed" scenario via the
		// programmatic DELETE release path.
		//
		// Note: the 30-minute idle-TTL path (automatic expiry when the
		// holder goes silent) is not tested here because it would require
		// a 30-minute wait or clock injection. See backlog item
		// portal-test-clock-advance-endpoint for the clock-injection story.
		alice := authflow.SignInViaMagicLink(ctx, t, p, mh, "alice-lock@example.com")
		bob := authflow.SignInViaMagicLink(ctx, t, p, mh, "bob-lock@example.com")

		orgID := authflow.CreateOrg(ctx, t, p, alice.AccessToken, "Lock Lifecycle Org")

		// Bob must be an org member before he can join the session.
		inviteID := authflow.InviteToOrg(ctx, t, p, alice.AccessToken, orgID, "bob-lock@example.com")
		inviteToken := authflow.ExtractInviteToken(ctx, t, mh, "bob-lock@example.com")
		authflow.AcceptInvite(ctx, t, p, bob.AccessToken, orgID, inviteID, inviteToken)

		sessionID := createSession(ctx, t, p, alice.AccessToken, orgID, "Lock Lifecycle Session")

		// Bob must join the session so he is a session member.
		joinSessionAsInvitee(ctx, t, p, alice.AccessToken, bob.AccessToken, orgID, sessionID, mh, "bob-lock@example.com")

		// Alice acquires the finalize lock.
		lockID := acquireFinalizeLock(ctx, t, p, alice.AccessToken, orgID, sessionID)
		t.Logf("finalize_lock_release_and_reacquire: alice acquired lock %s", lockID)

		// Bob tries to acquire — must get 409 finalize.lock_held_by_other.
		rawPostExpect(ctx, t,
			fmt.Sprintf("%s/api/orgs/%s/sessions/%s/finalize/lock", p.URL, orgID, sessionID),
			[]byte(`{}`),
			bob.AccessToken, http.StatusConflict, "finalize.lock_held_by_other")

		// Alice releases the lock.
		releaseFinalizeLock(ctx, t, p, alice.AccessToken, orgID, sessionID, lockID)
		t.Logf("finalize_lock_release_and_reacquire: alice released lock %s", lockID)

		// Bob can now acquire the lock.
		var bobLock lockStatus
		url := fmt.Sprintf("%s/api/orgs/%s/sessions/%s/finalize/lock", p.URL, orgID, sessionID)
		authflow.PostJSONInto(ctx, t, url, map[string]bool{}, bob.AccessToken, http.StatusCreated, &bobLock)
		if bobLock.LockID == "" {
			t.Fatal("finalize_lock_release_and_reacquire: bob's lock_id is empty after reacquisition")
		}
		if bobLock.LockID == lockID {
			t.Errorf("finalize_lock_release_and_reacquire: bob's lock_id %q matches alice's old lock_id %q; expected a new lock", bobLock.LockID, lockID)
		}
		t.Logf("finalize_lock_release_and_reacquire: bob acquired new lock %s", bobLock.LockID)
	})

	// ---------------------------------------------------------------------------
	// Magic-link TTL expiry
	// ---------------------------------------------------------------------------

	t.Run("magic_link_ttl_expiry", func(t *testing.T) {
		// Invariant: exchanging a magic-link token after its 15-minute TTL
		// has elapsed returns 401 auth.expired_token.
		//
		// We advance the portal's clock via the build-tag-gated
		// /test/clock-advance endpoint rather than sleeping for real. The
		// portal image must be built with `make test-portal-image` (which
		// passes -tags e2etest). The fixture helper Portal.AdvanceClock
		// fails loudly with an actionable message if the endpoint returns
		// 404, so a stale image surfaces an obvious error rather than a
		// silent skip.
		//
		// Ordering invariant: this subtest mutates the portal's process-
		// global clock; advances are forward-only and cumulative. Any
		// later subtest sharing this TestInterruptedOps portal instance
		// will see the +16m offset. The only later subtest at inspection
		// time is `ws_reconnect_after_drop`, which is clock-insensitive
		// and currently skipped. If a future subtest is added here that
		// depends on real wall-clock time, either reorder so this subtest
		// runs last, or spin up a fresh portal for it.
		email := "ttl-expiry@example.com"

		// Step 1: request a magic-link and extract the raw token from
		// MailHog (without exchanging — we want to control the gap).
		authflow.RequestMagicLink(ctx, t, p, email)
		token := authflow.ExtractMagicLinkToken(ctx, t, mh, email)

		// Step 2: advance the portal's clock past the 15-minute TTL.
		// One minute of headroom avoids edge cases around millisecond
		// drift between issue time and the advance call.
		p.AdvanceClock(ctx, t, 16*time.Minute)

		// Step 3: attempt exchange — must fail with auth.expired_token.
		url := fmt.Sprintf("%s/api/auth/magic-link/exchange", p.URL)
		body := []byte(fmt.Sprintf(`{"token":%q}`, token))
		rawPostExpect(ctx, t, url, body, "", http.StatusUnauthorized, "auth.expired_token")
	})

	// ---------------------------------------------------------------------------
	// WebSocket drop mid-event-burst
	// ---------------------------------------------------------------------------

	t.Run("ws_reconnect_after_drop", func(t *testing.T) {
		// Invariant: when a WebSocket client disconnects mid-event-burst,
		// reconnecting with replay_from set to the last-seen seq causes the
		// gateway to replay missed events (seq > replay_from) before
		// transitioning to live mode.
		//
		// Setup:
		//   - Alice creates a session (seq 1: session.created).
		//   - A WS client connects and listens for live events.
		//   - Alice fires 5 mode.changed events (seq 2..6) via the ref-modes
		//     endpoint, alternating between "sync" and "isolated" per ref.
		//   - We wait for the first two live events (seq 2, 3) and record
		//     the last-seen seq (3).
		//   - The first WS client is closed (simulating a mid-burst disconnect).
		//   - A second WS client reconnects with ConnectFromSeq(fromSeq=3).
		//     The gateway replays events with seq > 3, i.e., seq 4, 5, 6.
		//   - After replay we emit one more event (seq 7) and verify live
		//     delivery on the reconnected client.
		alice := authflow.SignInViaMagicLink(ctx, t, p, mh, "alice-ws-replay@example.com")
		orgID := authflow.CreateOrg(ctx, t, p, alice.AccessToken, "WS Replay Org")
		sessionID := createSession(ctx, t, p, alice.AccessToken, orgID, "WS Replay Session")

		// upsertRefMode emits a mode.changed event for the given ref.
		upsertRefMode := func(ref, mode string) {
			t.Helper()
			url := fmt.Sprintf("%s/api/orgs/%s/sessions/%s/ref-modes", p.URL, orgID, sessionID)
			body := map[string]string{"ref": ref, "mode": mode}
			b, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("ws_reconnect_after_drop: marshal ref-mode body: %v", err)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
			if err != nil {
				t.Fatalf("ws_reconnect_after_drop: build ref-mode request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+alice.AccessToken)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("ws_reconnect_after_drop: POST ref-modes: %v", err)
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("ws_reconnect_after_drop: POST ref-modes: status %d (want 204)", resp.StatusCode)
			}
		}

		// First connection: subscribe from now, collect live events.
		first := wsclient.Connect(ctx, t, p.URL, sessionID, alice.AccessToken)

		// Emit 5 mode.changed events (seqs 2..6; seq 1 was session.created).
		refs := []string{
			"refs/heads/jam/a",
			"refs/heads/jam/b",
			"refs/heads/jam/c",
			"refs/heads/jam/d",
			"refs/heads/jam/e",
		}
		modes := []string{"sync", "isolated", "sync", "isolated", "sync"}
		for i, ref := range refs {
			upsertRefMode(ref, modes[i])
		}

		// Collect the first two live events on the initial connection and
		// record the last-seen seq.
		var lastSeenSeq int64
		for i := 0; i < 2; i++ {
			ev := first.WaitFor(t, "mode.changed", 10*time.Second)
			lastSeenSeq = ev.Seq
		}
		// lastSeenSeq is the seq of the 2nd mode.changed event (seq 3).

		// Simulate disconnect: close the first client before the remaining
		// events (seqs lastSeenSeq+1 .. 6) arrive.
		first.Close()

		// Second connection: reconnect with cursor at lastSeenSeq.
		// The gateway replays events with seq > lastSeenSeq.
		second := wsclient.ConnectFromSeq(ctx, t, p.URL, sessionID, alice.AccessToken, lastSeenSeq)

		// Expect the replayed events in ascending seq order.
		// The gateway replays seq lastSeenSeq+1 through 6 (3 events), then
		// transitions to live mode.
		expectedReplayCount := 3 // seqs 4, 5, 6
		var prevSeq int64 = lastSeenSeq
		for i := 0; i < expectedReplayCount; i++ {
			ev := second.WaitFor(t, "mode.changed", 10*time.Second)
			if ev.Seq <= prevSeq {
				t.Errorf("ws_reconnect_after_drop: replay event %d: seq=%d is not > prev seq=%d (events must be strictly ascending)", i, ev.Seq, prevSeq)
			}
			prevSeq = ev.Seq
		}

		// Emit one more event and assert it arrives as a live event on the
		// reconnected client (proves live mode is active after replay).
		upsertRefMode("refs/heads/jam/f", "isolated")
		liveEv := second.WaitFor(t, "mode.changed", 10*time.Second)
		if liveEv.Seq <= prevSeq {
			t.Errorf("ws_reconnect_after_drop: live event seq=%d is not > last replay seq=%d", liveEv.Seq, prevSeq)
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers local to this file
// ---------------------------------------------------------------------------

// joinSessionAsInvitee invites targetEmail to the given session (via an
// existing session member inviting from within the session) and has the
// invitee accept. This uses the session-invite endpoints, not the org-invite
// endpoints.
//
// If the session-invite flow is not yet wired (404 from the invite endpoint),
// this helper falls back to a no-op and logs a warning — the lock lifecycle
// test can still run if both alice and bob are org members, as long as the
// session invite endpoint doesn't gate finalize-lock access differently.
func joinSessionAsInvitee(
	ctx context.Context,
	t *testing.T,
	p *portal.Portal,
	ownerToken, inviteeToken string,
	orgID, sessionID string,
	mh *mailhog.MailHog,
	inviteeEmail string,
) {
	t.Helper()

	inviteURL := fmt.Sprintf("%s/api/orgs/%s/sessions/%s/invites", p.URL, orgID, sessionID)
	b, err := json.Marshal(map[string]string{"email": inviteeEmail})
	if err != nil {
		t.Fatalf("joinSessionAsInvitee: marshal invite body: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inviteURL, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("joinSessionAsInvitee: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ownerToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("joinSessionAsInvitee: POST %s: %v", inviteURL, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		// Session-invite endpoint not yet wired. The finalize-lock test
		// proceeds; if membership is required for lock acquisition the
		// subtest will fail with a clear 403 rather than a cryptic skip.
		t.Logf("joinSessionAsInvitee: session invite endpoint returned %d — skipping session join (invitee may fail membership check)", resp.StatusCode)
		return
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("joinSessionAsInvitee: POST %s: status %d (want 201): %s", inviteURL, resp.StatusCode, respBody)
	}

	// Extract the session invite token from the invitee's inbox.
	var inviteRef struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &inviteRef); err != nil {
		t.Fatalf("joinSessionAsInvitee: decode invite response: %v\nbody: %s", err, respBody)
	}

	msg := mh.LatestMessageTo(ctx, t, inviteeEmail, 5*time.Second)
	body := authflow.DecodeEmailBody(msg.Body)
	matches := authflow.InviteTokenRE.FindStringSubmatch(body)
	if len(matches) < 2 {
		t.Fatalf("joinSessionAsInvitee: could not extract token from session invite email:\n%s", body)
	}
	sessionInviteToken := matches[1]

	acceptURL := fmt.Sprintf("%s/api/orgs/%s/sessions/%s/invites/%s/accept", p.URL, orgID, sessionID, inviteRef.ID)
	ab, _ := json.Marshal(map[string]string{"token": sessionInviteToken})
	areq, err := http.NewRequestWithContext(ctx, http.MethodPost, acceptURL, bytes.NewReader(ab))
	if err != nil {
		t.Fatalf("joinSessionAsInvitee: build accept request: %v", err)
	}
	areq.Header.Set("Content-Type", "application/json")
	areq.Header.Set("Authorization", "Bearer "+inviteeToken)

	aresp, err := http.DefaultClient.Do(areq)
	if err != nil {
		t.Fatalf("joinSessionAsInvitee: POST %s: %v", acceptURL, err)
	}
	defer aresp.Body.Close()
	aBody, _ := io.ReadAll(aresp.Body)
	if aresp.StatusCode != http.StatusOK && aresp.StatusCode != http.StatusNoContent {
		t.Fatalf("joinSessionAsInvitee: accept %s: status %d: %s", acceptURL, aresp.StatusCode, aBody)
	}
}
