// Invariant: with JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES=1048576 (1 MiB), a git
// push that would bring the session's accumulated repo content past the cap is
// rejected at the real git-receive-pack pre-receive hook, and the on-disk repo
// size after the rejection is still at or below the cap (no partial-write commit).
//
// This test exercises the content-cap enforcement end-to-end through the real
// portal binary, real git-receive-pack subprocess, and real pre-receive hook
// wiring. A bug visible only at the subprocess boundary — e.g. the cap counted
// in compressed vs uncompressed bytes, the cumulative measurement broken, or
// the rejection happening AFTER the packfile is committed — cannot be caught by
// unit tests that stub the validator or use an in-process httptest server.
//
// Two subtests:
//
//  1. oversize_push_rejected: First push (small seed, ~2 KiB) succeeds; second
//     push (~1.5 MiB random blob) is rejected at pre-receive. On-disk repo size
//     after rejection is at or below the cap (1 MiB + 256 KiB metadata slop).
//
//  2. per_session_isolation: Filling session S1 to near-cap does not consume
//     session S2's quota. S2 can still accept a non-trivial push after S1 is
//     near its limit.
package failure_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/gitclient"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// duSizeRE matches the first run of decimal digits in a string. Used to
// extract the size field from `du -sb` output after stripping Docker exec
// multiplexer header bytes (which prepend 8 binary bytes to the real output).
var duSizeRE = regexp.MustCompile(`\d+`)

// capTestMaxBytes is the content cap configured for this test (1 MiB).
// This matches JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES below.
const capTestMaxBytes = 1 << 20 // 1 MiB

// capTestMetadataSlop is the allowance above the cap for git's own bookkeeping
// files (COMMIT_EDITMSG, config, packed-refs, etc.) that are not part of the
// pushed pack but live in the bare repo directory.
const capTestMetadataSlop = 256 << 10 // 256 KiB

// capTestOrgID is the playground org ID — pinned here for clarity.
const capTestOrgID = "org_playground"

func TestPlayground_ContentCap_PreReceiveRejectsOversize(t *testing.T) {
	ctx := context.Background()

	// ── Infrastructure ───────────────────────────────────────────────────────
	// Portal with a low 1 MiB content cap for test speed.
	// CreatePerIPHour=180 → burst=3: allows the 2 rapid-fire creates in the
	// per-session isolation subtest without triggering the rate limiter.
	// Hard-cap and idle-timeout are generous so neither session expires.
	pg := postgres.Start(ctx, t, postgres.Options{})
	p := portal.Start(ctx, t, portal.Options{
		DBDriver: "postgres",
		DBDSN:    pg.ContainerDSN,
		ExtraEnv: map[string]string{
			"JAMSESH_PLAYGROUND_ENABLED":            "true",
			"JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES":  strconv.Itoa(capTestMaxBytes),
			"JAMSESH_PLAYGROUND_HARD_CAP_S":         "600",
			"JAMSESH_PLAYGROUND_IDLE_TIMEOUT_S":     "1200",
			"JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR": "180",
		},
	})

	// ── Subtest 1: oversize push rejected at pre-receive ─────────────────────
	t.Run("oversize_push_rejected", func(t *testing.T) {
		// Invariant: first push (small seed) succeeds; second push (large random
		// blob, gzip-incompressible, intended to exceed the 1 MiB cap) is rejected
		// at the real pre-receive hook. The on-disk repo size after rejection is at
		// or below cap + metadata slop (no partial-write committed).

		// Create a fresh session.
		sess := pgCreate(ctx, t, p.URL)
		sessionID := sess.Session.ID
		bearer := sess.Bearer
		t.Logf("content_cap: session=%s bearer_prefix=%s", sessionID, bearer[:8])

		// Derive the anonymous accountID via /api/me.
		me := capGetMe(ctx, t, p.URL, bearer)
		accountID := me.id
		require.NotEmpty(t, accountID, "accountID from /api/me must be non-empty")
		t.Logf("content_cap: accountID=%s", accountID)

		// Clone the empty session repo.
		repo := gitclient.Clone(ctx, t, p.URL, capTestOrgID, sessionID, accountID, bearer)

		// ── First push: small seed on the base ref ────────────────────────────
		// The base ref push is exempt from the content-cap pre-receive check
		// when the session is empty (first push) but IS counted toward the
		// cumulative size for subsequent pushes. We use a small plaintext file
		// (~2 KiB) so the bare repo stays well under the 1 MiB cap after this push.
		baseRef := "jam/" + sessionID + "/base"
		repo.Commit(ctx, t, "seed.md", strings.Repeat("x", 2048), "content_cap: seed base commit")
		repo.Push(ctx, t, baseRef)
		t.Logf("content_cap: base ref pushed (%s) — expect success", baseRef)

		// Sanity: the repo must actually exist and be non-empty after the first push.
		repoPath := "/tmp/jamsesh-repos/orgs/" + capTestOrgID + "/sessions/" + sessionID + ".git"
		exitCode, execOut, execErr := p.Exec(ctx, []string{"ls", repoPath})
		require.NoError(t, execErr, "docker exec ls %s: API error", repoPath)
		require.Equal(t, 0, exitCode,
			"bare repo must exist at %s after base push\noutput: %s", repoPath, execOut)

		// ── Generate a large random blob (~1.5 MiB) ───────────────────────────
		// Random bytes are gzip-incompressible, so the packfile size will be close
		// to the raw blob size. This guarantees the push will genuinely exceed the
		// 1 MiB cap regardless of git's delta compression.
		const blobSize = 1536 * 1024 // 1.5 MiB
		blob := make([]byte, blobSize)
		if _, err := rand.Read(blob); err != nil {
			t.Fatalf("content_cap: generate random blob: %v", err)
		}

		// ── Second push: large blob on a user ref — must be REJECTED ──────────
		// The random blob is committed as a binary file in the cloned worktree.
		// CommitBytes writes the bytes directly so no string-encoding issues arise.
		userRef := "jam/" + sessionID + "/" + accountID + "/main"
		repo.CommitBytes(ctx, t, "large-blob.bin", blob, "content_cap: oversize blob (1.5 MiB)")

		// Use a non-fatal push so the test can assert on the error rather than aborting.
		pushErr := capTryPush(ctx, repo, userRef)
		require.Error(t, pushErr,
			"CONTENT CAP NOT ENFORCED: oversize push (1.5 MiB blob into a 1 MiB cap) "+
				"returned success. Expected: git push exits non-zero (pre-receive rejected). "+
				"If this assertion fails, the content-cap pre-receive check is not wired "+
				"against the real git-receive-pack subprocess. "+
				"Do NOT weaken this to require.NoError — that would paper over the bug.")

		// The rejection from CheckPlaygroundCaps is forwarded via writeReportStatusRejection
		// and produces a sideband pkt-line report-status response (HTTP 200, ~241 bytes).
		// In practice the git client surfaces this as "fatal: the remote end hung up
		// unexpectedly" rather than "remote: playground session content limit exceeded"
		// because git's stateless-rpc two-POST protocol sends capabilities only in the
		// first (probe) POST — the second POST (with the pack data) may not re-advertise
		// side-band-64k, causing the rejection pkt-lines to be formatted incorrectly.
		//
		// The invariant we assert: exit code is non-zero (cap enforced). The
		// human-readable message routing is a separate UX concern tracked in the audit.
		// We document the actual error text here so a regression (push suddenly returning
		// 0) is immediately visible.
		t.Logf("content_cap: oversize push correctly rejected (exit non-zero): %v", pushErr)

		// ── Assert on-disk repo size is at or below cap + slop ────────────────
		// If the packfile was committed BEFORE the pre-receive check fired (a real
		// possible failure mode the audit calls out), the on-disk size will exceed
		// the cap. We allow a small slop for git's own metadata files.
		repoSize := capExecDuBytes(ctx, t, p, repoPath)
		maxAllowed := int64(capTestMaxBytes + capTestMetadataSlop)
		if repoSize > maxAllowed {
			// This is a production bug: the pre-receive check fired too late and
			// the oversize packfile was already committed to disk. Park it — don't
			// silently pass by adjusting the threshold.
			t.Fatalf(
				"CONTENT CAP PARTIAL-WRITE BUG: on-disk repo size after oversize-push rejection "+
					"is %d bytes (cap=%d, slop=%d, allowed=%d). "+
					"The pre-receive check fired after the packfile was already written to disk. "+
					"This is the 'cap enforced after partial write' failure mode documented in "+
					"the audit. Park via /agile-workflow:park with severity=High tag=playground.\n\n"+
					"Repo path: %s\n"+
					"Cap: %d bytes (JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES)\n"+
					"Observed: %d bytes on disk after rejection",
				repoSize, capTestMaxBytes, capTestMetadataSlop, maxAllowed,
				repoPath, capTestMaxBytes, repoSize,
			)
		}
		t.Logf("content_cap: on-disk repo size after rejection: %d bytes (cap=%d, slop=%d, allowed=%d) — PASS",
			repoSize, capTestMaxBytes, capTestMetadataSlop, maxAllowed)
	})

	// ── Subtest 2: per-session quota isolation ────────────────────────────────
	t.Run("per_session_isolation", func(t *testing.T) {
		// Invariant: filling session S1 close to the cap does not consume session
		// S2's quota. After S1 is near its limit, S2 can still accept a non-trivial
		// push.
		//
		// This catches a shared-state bug where the content-cap measurement is
		// global (e.g. summed across all playground sessions in the same org)
		// rather than per-session. Such a bug would only surface in an e2e test
		// with two concurrent sessions — unit tests mock the measurement.

		// Create two independent sessions.
		s1 := pgCreate(ctx, t, p.URL)
		s2 := pgCreate(ctx, t, p.URL)
		t.Logf("per_session_isolation: S1=%s S2=%s", s1.Session.ID, s2.Session.ID)

		// Derive accountIDs.
		me1 := capGetMe(ctx, t, p.URL, s1.Bearer)
		me2 := capGetMe(ctx, t, p.URL, s2.Bearer)

		// Clone both repos.
		repo1 := gitclient.Clone(ctx, t, p.URL, capTestOrgID, s1.Session.ID, me1.id, s1.Bearer)
		repo2 := gitclient.Clone(ctx, t, p.URL, capTestOrgID, s2.Session.ID, me2.id, s2.Bearer)

		// ── Push a near-cap blob to S1 (~900 KiB, random, incompressible) ─────
		const s1BlobSize = 900 * 1024 // 900 KiB: under the 1 MiB cap individually
		s1Blob := make([]byte, s1BlobSize)
		if _, err := rand.Read(s1Blob); err != nil {
			t.Fatalf("per_session_isolation: generate S1 blob: %v", err)
		}

		s1BaseRef := "jam/" + s1.Session.ID + "/base"
		repo1.Commit(ctx, t, "seed.md", "S1 seed", "content_cap/isolation: S1 seed")
		repo1.Push(ctx, t, s1BaseRef)

		s1UserRef := "jam/" + s1.Session.ID + "/" + me1.id + "/main"
		repo1.CommitBytes(ctx, t, "s1-blob.bin", s1Blob, "content_cap/isolation: S1 near-cap blob")
		// This push must succeed — 900 KiB is under the 1 MiB cap.
		s1PushErr := capTryPush(ctx, repo1, s1UserRef)
		require.NoError(t, s1PushErr,
			"per_session_isolation: S1 near-cap push (900 KiB) must succeed "+
				"(cap is 1 MiB; 900 KiB blob should be under the limit). "+
				"If this fails, the cap may be misconfigured or the slop for git metadata "+
				"is larger than expected — check the JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES value.")
		t.Logf("per_session_isolation: S1 near-cap push succeeded")

		// ── Push a non-trivial blob to S2 (same size, independent quota) ──────
		// If the cap is per-session (correct), S2's push should succeed because S2's
		// repo is still empty. If the cap is shared/global across the org (bug), S2's
		// push would fail because S1 already consumed most of the shared quota.
		s2BaseRef := "jam/" + s2.Session.ID + "/base"
		s2Blob := make([]byte, s1BlobSize)
		if _, err := rand.Read(s2Blob); err != nil {
			t.Fatalf("per_session_isolation: generate S2 blob: %v", err)
		}

		repo2.Commit(ctx, t, "seed.md", "S2 seed", "content_cap/isolation: S2 seed")
		repo2.Push(ctx, t, s2BaseRef)

		s2UserRef := "jam/" + s2.Session.ID + "/" + me2.id + "/main"
		repo2.CommitBytes(ctx, t, "s2-blob.bin", s2Blob, "content_cap/isolation: S2 near-cap blob")
		s2PushErr := capTryPush(ctx, repo2, s2UserRef)
		require.NoError(t, s2PushErr,
			"PER-SESSION ISOLATION BUG: S2 push (900 KiB) was rejected after S1 filled "+
				"900 KiB. This means the content cap is measured globally (shared across "+
				"sessions) rather than per-session. "+
				"Expected: S2 has an independent 1 MiB quota; its push must succeed. "+
				"Do NOT widen this assertion — the mismatch is the bug. "+
				"Park via /agile-workflow:park with severity=High tag=playground.")
		t.Logf("per_session_isolation: S2 push succeeded independently — quota is per-session — PASS")
	})
}

// ---------------------------------------------------------------------------
// Helpers private to this file
// ---------------------------------------------------------------------------

// capMeResponse is a minimal decode of GET /api/me for the content-cap tests.
type capMeResponse struct {
	id string
}

// capGetMe calls GET /api/me with the given bearer and returns the account ID.
// Fails the test on any non-200 response or missing ID.
func capGetMe(ctx context.Context, t *testing.T, baseURL, bearer string) capMeResponse {
	t.Helper()
	resp, body, err := pgGET(ctx, strings.TrimRight(baseURL, "/")+"/api/me", bearer)
	if err != nil {
		t.Fatalf("capGetMe: GET /api/me: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("capGetMe: status %d (want 200)\nbody: %s", resp.StatusCode, body)
	}
	// Minimal JSON decode: we only need the "id" field.
	type meBody struct {
		ID string `json:"id"`
	}
	var m meBody
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("capGetMe: decode /api/me: %v\nbody: %s", err, body)
	}
	if m.ID == "" {
		t.Fatalf("capGetMe: empty id in /api/me response\nbody: %s", body)
	}
	return capMeResponse{id: m.ID}
}

// capTryPush pushes HEAD to the given ref on repo's origin remote, returning
// an error instead of calling t.Fatal on failure. This allows the oversize-push
// subtest to observe the rejection without aborting the test.
//
// The returned error wraps the combined stdout+stderr from git push so callers
// can assert on rejection message substrings.
func capTryPush(ctx context.Context, repo *gitclient.Repo, ref string) error {
	cmd := exec.CommandContext(ctx, "git", "push", "origin", "HEAD:refs/heads/"+ref)
	cmd.Dir = repo.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push HEAD:refs/heads/%s: %w\noutput: %s", ref, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// capExecDuBytes runs `du -sb <path>` inside the portal container and returns
// the reported size in bytes. Fails the test if du exits non-zero or if the
// output cannot be parsed as an integer.
//
// `du -sb` produces a single line: "<bytes>\t<path>".
func capExecDuBytes(ctx context.Context, t *testing.T, p *portal.Portal, path string) int64 {
	t.Helper()
	exitCode, out, err := p.Exec(ctx, []string{"du", "-sb", path})
	if err != nil {
		t.Fatalf("capExecDuBytes: docker exec du -sb %s: API error: %v", path, err)
	}
	if exitCode != 0 {
		t.Fatalf("capExecDuBytes: du -sb %s exited %d\noutput: %s", path, exitCode, out)
	}
	// Output is "<size>\t<path>\n", but Docker exec may prepend an 8-byte
	// multiplexer header (stream type + length). Extract the first decimal
	// run from the raw output to handle both cases.
	match := duSizeRE.FindString(out)
	if match == "" {
		t.Fatalf("capExecDuBytes: no decimal number in du -sb %s output: %q", path, out)
	}
	size, parseErr := strconv.ParseInt(match, 10, 64)
	if parseErr != nil {
		t.Fatalf("capExecDuBytes: parse du output %q: %v", match, parseErr)
	}
	return size
}
