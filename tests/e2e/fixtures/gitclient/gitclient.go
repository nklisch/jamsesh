// Package gitclient wraps os/exec git invocations to perform clone, commit,
// push, and fetch operations against the portal's smart-HTTP git endpoints.
//
// Authentication uses HTTP Basic auth with the bearer token as the password
// (username is ignored by the server). Credentials are embedded in the clone
// URL so that subsequent push/fetch operations inside the working tree inherit
// them via the remote URL stored by git clone.
//
// Every commit MUST carry the required jamsesh trailers or the portal's
// pre-receive hook will reject the push with 422:
//
//	Jam-Session: <session-id>
//	Jam-Turn:    <turn-uuid>
//	Jam-Author:  <user-id>
package gitclient

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Repo is a working tree cloned from a session's bare repository.
type Repo struct {
	// Dir is the absolute path of the working tree on the test host.
	Dir string
	// SessionID is used to populate the Jam-Session trailer on every commit.
	SessionID string
	// OrgID is the org that owns the session (used to construct git URLs).
	OrgID string
	// UserID is used to populate the Jam-Author trailer and to derive the
	// push ref namespace (jam/<sessionID>/<userID>/<branch>).
	UserID string
	// bearer is the access token embedded in the git remote URL.
	bearer    string
	// portalURL is the portal base URL (http://host:port) without trailing slash.
	portalURL string
}

// Clone clones the session's bare repository into a new temporary directory.
// The bearer token is embedded in the remote URL as HTTP Basic auth so that
// push and fetch inside the repo do not need extra credential helpers.
//
// The caller must be a session member; otherwise the clone fails with 401.
//
// After cloning, if the user's default jam ref (jam/<sessionID>/<userID>/main)
// already exists on the remote, Clone checks out a local branch tracking it so
// that subsequent commits build on top of the existing ref tip. This mirrors
// the production CLI's `jamsesh session join` flow, which runs
// `git checkout -b jam/<sid>/<uid>/main <fromRef>` after the bare clone
// (cmd/jamsesh/sessioncmd/join.go).
//
// Without this, a fresh `git clone` lands on the server's unborn default branch
// (`git init --bare` leaves HEAD pointing at a refs/heads/<default> that never
// gets a commit). A commit built on that unborn branch is a root commit
// disconnected from the populated jam ref, so pushing it to
// jam/<sid>/<uid>/main is rejected as non-fast-forward. Checking out the
// existing ref restores the correct base.
func Clone(ctx context.Context, t *testing.T, portalURL, orgID, sessionID, userID, bearer string) *Repo {
	t.Helper()
	dir := t.TempDir()

	repoURL := basicAuthURL(portalURL, "x-access-token", bearer) +
		"/git/" + orgID + "/" + sessionID + ".git"

	run(ctx, t, "", "git", "clone", repoURL, dir)

	// Configure git identity inside this repo so commits don't fail on
	// missing user.email / user.name. Values are arbitrary for tests.
	run(ctx, t, dir, "git", "config", "user.email", userID+"@test.example")
	run(ctx, t, dir, "git", "config", "user.name", "Test "+userID)

	r := &Repo{
		Dir:       dir,
		SessionID: sessionID,
		OrgID:     orgID,
		UserID:    userID,
		bearer:    bearer,
		portalURL: portalURL,
	}

	// If the user's default jam ref already exists on the remote, position the
	// working tree on it so commits fast-forward on push. On the first clone of
	// a session the ref does not exist yet, so this is a no-op and the caller's
	// first commit creates the ref (the OldSHA="" path in pre-receive).
	defaultRef := "jam/" + sessionID + "/" + userID + "/main"
	r.checkoutIfRemoteRefExists(ctx, t, defaultRef)

	return r
}

// checkoutIfRemoteRefExists checks out a local branch tracking origin/<ref> if
// that remote-tracking ref exists in the freshly-cloned repo. If the remote ref
// does not exist (first clone of a session), it is a no-op and the working tree
// stays on the clone's default (possibly unborn) branch.
func (r *Repo) checkoutIfRemoteRefExists(ctx context.Context, t *testing.T, ref string) {
	t.Helper()
	// Use rev-parse --verify --quiet to detect the remote-tracking ref without
	// failing the test when it is absent (exit code 1 ⇒ absent, not an error).
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", "origin/"+ref)
	cmd.Dir = r.Dir
	if err := cmd.Run(); err != nil {
		// Remote ref absent: leave the working tree on the clone default.
		return
	}
	// Remote ref exists: check out a local branch tracking it so HEAD (and thus
	// the next commit's parent) is the existing ref tip. Mirrors join.go's
	// `git checkout -b jam/<sid>/<uid>/main <fromRef>`.
	run(ctx, t, r.Dir, "git", "checkout", "-B", ref, "origin/"+ref)
}

// Commit writes content to relPath inside the working tree, stages it, and
// creates a commit with the supplied message plus the required Jam-* trailers.
//
// A fresh Jam-Turn UUID is generated for each commit (each commit is its own
// turn from the pre-receive validator's perspective).
//
// Returns the full commit SHA (40 hex chars).
func (r *Repo) Commit(ctx context.Context, t *testing.T, relPath, content, message string) string {
	t.Helper()

	absPath := filepath.Join(r.Dir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("gitclient.Commit: mkdir %s: %v", filepath.Dir(absPath), err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatalf("gitclient.Commit: write %s: %v", absPath, err)
	}

	run(ctx, t, r.Dir, "git", "add", relPath)

	// Build the full commit message with required trailers in the last paragraph.
	// git-interpret-trailers expects the trailer block to be separated from the
	// body by a blank line and to contain only Key: value lines.
	turnID := uuid.New().String()
	fullMessage := fmt.Sprintf("%s\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
		message, r.SessionID, turnID, r.UserID)

	// Write message to a temp file and use git commit -F to avoid shell quoting
	// issues with multi-line messages.
	msgFile := filepath.Join(t.TempDir(), "commit-msg")
	if err := os.WriteFile(msgFile, []byte(fullMessage), 0o644); err != nil {
		t.Fatalf("gitclient.Commit: write message file: %v", err)
	}

	run(ctx, t, r.Dir, "git", "commit", "-F", msgFile)

	// Capture HEAD SHA.
	out := runOutput(ctx, t, r.Dir, "git", "rev-parse", "HEAD")
	return strings.TrimSpace(out)
}

// CommitBytes writes raw bytes to relPath inside the working tree, stages it,
// and creates a commit with the supplied message plus the required Jam-*
// trailers. Unlike Commit, the content parameter is a raw byte slice so callers
// can commit binary or randomly-generated data without Go's string-conversion
// semantics interfering with the byte values.
//
// A fresh Jam-Turn UUID is generated for each commit.
//
// Returns the full commit SHA (40 hex chars).
func (r *Repo) CommitBytes(ctx context.Context, t *testing.T, relPath string, content []byte, message string) string {
	t.Helper()

	absPath := filepath.Join(r.Dir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("gitclient.CommitBytes: mkdir %s: %v", filepath.Dir(absPath), err)
	}
	if err := os.WriteFile(absPath, content, 0o644); err != nil {
		t.Fatalf("gitclient.CommitBytes: write %s: %v", absPath, err)
	}

	run(ctx, t, r.Dir, "git", "add", relPath)

	turnID := uuid.New().String()
	fullMessage := fmt.Sprintf("%s\n\nJam-Session: %s\nJam-Turn: %s\nJam-Author: %s\n",
		message, r.SessionID, turnID, r.UserID)

	msgFile := filepath.Join(t.TempDir(), "commit-msg")
	if err := os.WriteFile(msgFile, []byte(fullMessage), 0o644); err != nil {
		t.Fatalf("gitclient.CommitBytes: write message file: %v", err)
	}

	run(ctx, t, r.Dir, "git", "commit", "-F", msgFile)

	out := runOutput(ctx, t, r.Dir, "git", "rev-parse", "HEAD")
	return strings.TrimSpace(out)
}

// Push pushes HEAD to the given ref on the origin remote. The ref should be
// in the jam/<sessionID>/<userID>/<branch> namespace; other namespaces are
// rejected by the portal's pre-receive hook.
//
// Example: r.Push(ctx, t, "jam/"+r.SessionID+"/"+r.UserID+"/main")
func (r *Repo) Push(ctx context.Context, t *testing.T, ref string) {
	t.Helper()
	run(ctx, t, r.Dir, "git", "push", "origin", "HEAD:refs/heads/"+ref)
}

// Fetch fetches all refs from the origin remote into the local repo.
// After Fetch, callers can inspect peer refs via git log or git rev-parse.
func (r *Repo) Fetch(ctx context.Context, t *testing.T) {
	t.Helper()
	run(ctx, t, r.Dir, "git", "fetch", "origin", "--no-tags")
}

// RevParse returns the SHA that the given ref resolves to, or fails the test
// if the ref does not exist. Useful for asserting that a peer's ref is visible
// after Fetch.
func (r *Repo) RevParse(ctx context.Context, t *testing.T, ref string) string {
	t.Helper()
	out := runOutput(ctx, t, r.Dir, "git", "rev-parse", "origin/"+ref)
	return strings.TrimSpace(out)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// basicAuthURL injects user:pass HTTP Basic credentials into the given base
// URL so that git uses them for the clone remote.
//
//	http://host:port  →  http://user:pass@host:port
func basicAuthURL(baseURL, user, pass string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		// baseURL comes from the test fixture — a parse error is a programmer bug.
		panic(fmt.Sprintf("gitclient: basicAuthURL: parse %q: %v", baseURL, err))
	}
	u.User = url.UserPassword(user, pass)
	return u.String()
}

// run executes a git command in dir, failing the test on any error.
// Pass dir="" to run in the current directory.
func run(ctx context.Context, t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gitclient: %s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

// runOutput executes a git command in dir and returns its stdout, failing on
// error.
func runOutput(ctx context.Context, t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("gitclient: %s %s: %v\n%s", name, strings.Join(args, " "), err, stderr)
	}
	return string(out)
}
