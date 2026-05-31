package finalizecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/cmd/jamsesh/state"
	"jamsesh/internal/api/openapi"
)

// jamseshRemoteName is the temporary remote we register for HTTPS-fallback
// fetches. The cleanup func removes it on every exit path so the user's
// `git remote -v` does not accumulate jamsesh entries between runs.
const jamseshRemoteName = "jamsesh"

// fetchSource is the resolved local-or-https endpoint git fetch will pull
// from, plus a cleanup func registered with the cleanupStack. For "local"
// the cleanup is a no-op; for "https" it removes the temporary jamsesh
// remote (idempotent — a missing remote is treated as success so a
// double-invocation of the cleanup stack is safe).
type fetchSource struct {
	Kind    string // "local" | "https"
	URL     string // local path OR plain https URL (no credentials)
	Token   string // Bearer token for HTTPS fetches; empty for local
	cleanup func() error
}

// chooseFetchSource picks local-first when the state file
// `${data-dir}/sessions/<sid>/local_path` points to a real git
// repo on disk; otherwise falls back to HTTPS by minting an ephemeral
// fetch-token via the portal and registering a temporary jamsesh remote.
//
// Local path:
//   - reads the local_path state file written at join-time by a sibling
//     story (absent today — the HTTPS fallback covers everything until
//     that lands)
//   - verifies the path is a directory that looks like a git repo
//     (either contains `.git` or is itself a bare repo with HEAD)
//   - returns Kind:"local", URL set to the path, cleanup is a no-op
//
// HTTPS path:
//   - POST /api/orgs/<org>/sessions/<sid>/finalize/fetch-token (Bearer
//     attached via portalclient, refresh-on-401)
//   - response carries a plain `remote_url` (no credentials) and a
//     separate `token` field
//   - `git remote add jamsesh <url>` is run here so the cleanup is
//     active by the time we return; the caller pushes our cleanup func
//     onto the cleanupStack before the corresponding `git fetch jamsesh`
//   - the token is stored in fetchSource.Token; performFetch passes it
//     through Git's GIT_CONFIG_* environment channel as http.extraHeader so
//     it never appears in .git/config or argv
//   - cleanup runs `git remote remove jamsesh`, swallowing "No such
//     remote" so double-invocation is harmless
func chooseFetchSource(ctx context.Context, pc *portalclient.Client, plan *Plan, orgID, sessionID string) (*fetchSource, error) {
	if local, ok := localPathForSession(sessionID); ok {
		return &fetchSource{
			Kind:    "local",
			URL:     local,
			cleanup: func() error { return nil },
		}, nil
	}

	// HTTPS fallback — mint an ephemeral fetch-only token and register a
	// temporary remote.
	path := fmt.Sprintf("/api/orgs/%s/sessions/%s/finalize/fetch-token", orgID, sessionID)
	resp, err := portalclient.PostJSON[openapi.FetchTokenResponse](ctx, pc, path, struct{}{})
	if err != nil {
		return nil, fmt.Errorf("issuing fetch token: %w", err)
	}
	if strings.TrimSpace(resp.RemoteUrl) == "" {
		return nil, errors.New("portal returned an empty remote_url for the fetch token")
	}
	if strings.TrimSpace(resp.Token) == "" {
		return nil, errors.New("portal returned an empty token for the fetch token")
	}

	// Register the temporary remote up-front so the cleanup func has
	// something to remove. Any pre-existing `jamsesh` remote (e.g. from a
	// previous run killed with `kill -9`) is removed first so `remote add`
	// doesn't fail with "already exists". The URL carries no credentials —
	// the token is passed at fetch time via the GIT_CONFIG_* environment
	// channel for http.extraHeader.
	_ = removeJamseshRemote() // best-effort pre-clean
	if err := runGit("remote", "add", jamseshRemoteName, resp.RemoteUrl); err != nil {
		return nil, fmt.Errorf("registering jamsesh remote: %w", err)
	}

	return &fetchSource{
		Kind:    "https",
		URL:     resp.RemoteUrl,
		Token:   resp.Token,
		cleanup: removeJamseshRemote,
	}, nil
}

// performFetch runs the kind-appropriate `git fetch` with verbose
// logging. For the local path we fetch from the on-disk repo URL; for
// HTTPS we fetch from the pre-registered jamsesh remote, passing the
// bearer token through Git's GIT_CONFIG_* environment channel as
// http.extraHeader so it is never persisted into .git/config or visible in
// argv.
func performFetch(out io.Writer, fs *fetchSource) error {
	switch fs.Kind {
	case "local":
		return runGitVerbose(out, "fetch", fs.URL)
	case "https":
		header := "Authorization: Bearer " + fs.Token
		return runGitVerboseWithEnv(out, gitExtraHeaderEnv(header), "fetch", jamseshRemoteName)
	default:
		return fmt.Errorf("unknown fetch source kind %q", fs.Kind)
	}
}

// removeJamseshRemote runs `git remote remove jamsesh` and treats the
// "no such remote" condition as success. Idempotency lets the cleanup
// stack be drained more than once (e.g. main flow + SIGINT goroutine)
// without surfacing spurious errors.
//
// We can't reliably classify the underlying *exec.ExitError without
// capturing stderr (which the package-level runGit doesn't expose), so
// we route through a dedicated capturing variant. Any error that is
// NOT the missing-remote case is returned wrapped so the caller can
// log it without masking the primary flow error.
var removeJamseshRemote = func() error {
	out, err := runGitCombined("remote", "remove", jamseshRemoteName)
	if err == nil {
		return nil
	}
	low := strings.ToLower(out)
	if strings.Contains(low, "no such remote") || strings.Contains(low, "error: no such remote") {
		return nil
	}
	return fmt.Errorf("git remote remove %s: %w: %s", jamseshRemoteName, err, strings.TrimSpace(out))
}

// localPathForSession reads ${data-dir}/sessions/<sid>/local_path
// and verifies the recorded path looks like a git repo. Returns
// (path, true) on success; (_ , false) on any miss (state dir absent,
// file absent, path missing on disk, path is not a git repo). The
// errors are intentionally swallowed: local-first is a best-effort
// optimization and any miss falls back to HTTPS.
func localPathForSession(sessionID string) (string, bool) {
	dir, err := state.DataDir()
	if err != nil {
		return "", false
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sessions", sessionID, "local_path"))
	if err != nil {
		return "", false
	}
	path := strings.TrimSpace(string(raw))
	if path == "" {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", false
	}
	if !looksLikeGitRepo(path) {
		return "", false
	}
	return path, true
}

// looksLikeGitRepo reports whether path is a worktree (contains a `.git`
// entry, file or directory) or a bare repo (HEAD file present at the
// top level). Used solely as a sanity gate for the local-first
// optimization; misclassification is harmless — it just routes us
// through the HTTPS fallback.
func looksLikeGitRepo(path string) bool {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "HEAD")); err == nil {
		return true
	}
	return false
}
