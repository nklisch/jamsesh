package portalcluster

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// WaitForHydration blocks until a pod can serve the session's git refs via
// ls-remote, or until timeout elapses.
//
// Strategy: run `git ls-remote <podURL>/git/<orgID>/<sessionID>.git` against
// the pod directly (not through the router). This succeeds if and only if the
// pod has hydrated the session's bare repo from object storage — it proves the
// pack files are locally present, which is exactly the post-hydration readiness
// signal required by the handoff tests.
//
// Authentication uses HTTP Basic auth with "x-access-token" as the username
// and the bearer token as the password — the same convention used by the
// git-smart-HTTP endpoint in all other e2e tests.
//
// The poll interval is fixed at 300 ms. The SLO from the parent story is:
//   - golden tests: pass timeout=30s
//   - chaos tests:  pass timeout=45s
//
// On timeout, t.Fatal is called with a clear message including the elapsed
// time and the last error from git ls-remote.
func (c *Cluster) WaitForHydration(
	ctx context.Context, t *testing.T,
	orgID, sessionID, accessToken string,
	podIndex int,
	timeout time.Duration,
) {
	t.Helper()

	if podIndex < 0 || podIndex >= len(c.Pods) {
		t.Fatalf("WaitForHydration: podIndex %d out of range (cluster has %d pods)", podIndex, len(c.Pods))
	}

	pod := c.Pods[podIndex]
	repoURL := lsRemoteURL(pod.URL, "x-access-token", accessToken, orgID, sessionID)

	const pollInterval = 300 * time.Millisecond
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		if err := tryLsRemote(ctx, repoURL); err == nil {
			return // hydration complete
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			t.Fatalf("WaitForHydration: context cancelled while waiting for pod %d to hydrate session %s: %v",
				podIndex, sessionID, ctx.Err())
			return
		case <-time.After(pollInterval):
		}
	}

	t.Fatalf("WaitForHydration: pod %d did not hydrate session %s within %v; last error: %v",
		podIndex, sessionID, timeout, lastErr)
}

// PollForHydration is the non-fatal variant of WaitForHydration. It returns
// true if the pod hydrates within timeout, false otherwise. Use this in tests
// that need to inspect state even after a hydration failure.
func (c *Cluster) PollForHydration(
	ctx context.Context,
	orgID, sessionID, accessToken string,
	podIndex int,
	timeout time.Duration,
) bool {
	if podIndex < 0 || podIndex >= len(c.Pods) {
		return false
	}

	pod := c.Pods[podIndex]
	repoURL := lsRemoteURL(pod.URL, "x-access-token", accessToken, orgID, sessionID)

	const pollInterval = 300 * time.Millisecond
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if err := tryLsRemote(ctx, repoURL); err == nil {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(pollInterval):
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// lsRemoteURL builds the authenticated git URL for a session's bare repo on a
// given pod. The format matches Clone in gitclient/gitclient.go:
//
//	http://x-access-token:<bearer>@<host:port>/git/<orgID>/<sessionID>.git
func lsRemoteURL(podURL, user, pass, orgID, sessionID string) string {
	// Insert basic-auth credentials into the URL.
	// podURL is http://host:port — insert credentials before the host.
	without, found := strings.CutPrefix(podURL, "http://")
	if !found {
		// Fallback for https or unexpected prefix.
		without = podURL
	}
	return fmt.Sprintf("http://%s:%s@%s/git/%s/%s.git",
		user, pass, without, orgID, sessionID)
}

// tryLsRemote runs `git ls-remote <repoURL>` and returns nil if it exits
// successfully. The output is discarded; we only care about the exit code.
//
// A non-zero exit means the pod either hasn't hydrated yet or can't serve the
// session — both are transient conditions during the polling window.
func tryLsRemote(ctx context.Context, repoURL string) error {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", repoURL)
	// Suppress git credential prompts — the URL already contains credentials.
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git ls-remote: %w; output: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// checkHTTPReadyz pings GET /readyz on the pod and returns nil if 200 OK.
// This is a lightweight liveness check used as a fallback in chaos scenarios
// where the git endpoint may be temporarily inaccessible but the pod is up.
// Not used in the main WaitForHydration path — ls-remote is a stronger signal.
func checkHTTPReadyz(ctx context.Context, podURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, podURL+"/readyz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("readyz: status %d", resp.StatusCode)
	}
	return nil
}
