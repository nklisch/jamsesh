package prereceive

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// initTestRepo creates a fresh non-bare git repo in a temp directory and
// returns the go-git handle and directory path. Cleaned up when test ends.
func initTestRepo(t *testing.T) (*git.Repository, string) {
	t.Helper()
	dir := t.TempDir()
	runCmd(t, dir, "git", "init", "-b", "main")
	runCmd(t, dir, "git", "config", "user.email", "test@jamsesh.test")
	runCmd(t, dir, "git", "config", "user.name", "Test")
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	return repo, dir
}

// makeCommit creates a commit in the repo with the given files and message.
// parent may be nil for the root commit. Returns the resulting *object.Commit.
func makeCommit(t *testing.T, repo *git.Repository, dir string, parent *object.Commit, files map[string]string, message string) *object.Commit {
	t.Helper()

	// Write files.
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runCmd(t, dir, "git", "add", "-A")

	sig := object.Signature{
		Name:  "Test",
		Email: "test@jamsesh.test",
		When:  time.Now(),
	}

	opts := &git.CommitOptions{
		Author:    &sig,
		Committer: &sig,
	}
	if parent != nil {
		opts.Parents = []plumbing.Hash{parent.Hash}
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	hash, err := wt.Commit(message, opts)
	if err != nil {
		// Retry with AllowEmptyCommits in case working tree is clean.
		opts2 := &git.CommitOptions{
			Author:            &sig,
			Committer:         &sig,
			AllowEmptyCommits: true,
		}
		if parent != nil {
			opts2.Parents = []plumbing.Hash{parent.Hash}
		}
		hash, err = wt.Commit(message, opts2)
		if err != nil {
			t.Fatalf("commit %q: %v", message, err)
		}
	}

	c, err := repo.CommitObject(hash)
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	return c
}

// runCmd executes a command in dir and fails the test on error.
func runCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run %v: %v\n%s", args, err, out)
	}
}

// goodMsg returns a commit message with all three required trailers.
func goodMsg(session, turn, author string) string {
	return "subject line\n\nJam-Session: " + session + "\nJam-Turn: " + turn + "\nJam-Author: " + author
}

// allDocsScope returns a ScopeMatcher that allows everything under docs/.
func allDocsScope(t *testing.T) *ScopeMatcher {
	t.Helper()
	m, err := CompileScope([]string{"docs/**"})
	if err != nil {
		t.Fatalf("CompileScope: %v", err)
	}
	return m
}

// openScope returns a ScopeMatcher that allows any path (match-all).
func openScope(t *testing.T) *ScopeMatcher {
	t.Helper()
	m, err := CompileScope([]string{"**"})
	if err != nil {
		t.Fatalf("CompileScope: %v", err)
	}
	return m
}

// TestWalkAndValidate_RootCommit verifies a single root commit (OldSHA="")
// with a valid message and in-scope file passes validation.
func TestWalkAndValidate_RootCommit(t *testing.T) {
	repo, dir := initTestRepo(t)
	scope := allDocsScope(t)

	c1 := makeCommit(t, repo, dir, nil,
		map[string]string{"docs/README.md": "hello"},
		goodMsg("sess-1", "1", "alice"),
	)

	rej := WalkAndValidate(context.Background(), repo, RefUpdate{
		Ref:    "refs/heads/jam/sess-1/alice/main",
		OldSHA: "",
		NewSHA: c1.Hash.String(),
	}, scope)

	if len(rej) != 0 {
		t.Errorf("expected no rejections, got %v", rej)
	}
}

// TestWalkAndValidate_Chain verifies a 3-commit chain where every commit is
// valid — no rejections expected.
func TestWalkAndValidate_Chain(t *testing.T) {
	repo, dir := initTestRepo(t)
	scope := openScope(t)

	c1 := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)
	c2 := makeCommit(t, repo, dir, c1,
		map[string]string{"b.txt": "2"},
		goodMsg("sess-1", "2", "alice"),
	)
	c3 := makeCommit(t, repo, dir, c2,
		map[string]string{"c.txt": "3"},
		goodMsg("sess-1", "3", "alice"),
	)

	rej := WalkAndValidate(context.Background(), repo, RefUpdate{
		Ref:    "refs/heads/jam/sess-1/alice/main",
		OldSHA: c1.Hash.String(), // only c2 and c3 are new
		NewSHA: c3.Hash.String(),
	}, scope)

	if len(rej) != 0 {
		t.Errorf("expected no rejections, got %v", rej)
	}
}

// TestWalkAndValidate_MissingTrailerInMid verifies that a commit with a
// missing trailer in the middle of a chain is rejected with CodeMissingTrailer.
func TestWalkAndValidate_MissingTrailerInMid(t *testing.T) {
	repo, dir := initTestRepo(t)
	scope := openScope(t)

	c1 := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)
	// c2 has no trailers at all.
	c2 := makeCommit(t, repo, dir, c1,
		map[string]string{"b.txt": "2"},
		"a commit with no trailers",
	)
	c3 := makeCommit(t, repo, dir, c2,
		map[string]string{"c.txt": "3"},
		goodMsg("sess-1", "3", "alice"),
	)

	rej := WalkAndValidate(context.Background(), repo, RefUpdate{
		Ref:    "refs/heads/jam/sess-1/alice/main",
		OldSHA: c1.Hash.String(),
		NewSHA: c3.Hash.String(),
	}, scope)

	// Exactly one rejection for c2.
	trailerRej := filterByCode(rej, CodeMissingTrailer)
	if len(trailerRej) == 0 {
		t.Errorf("expected a %s rejection for c2, got rejections: %v", CodeMissingTrailer, rej)
	}
}

// TestWalkAndValidate_OutOfScopePath verifies that a commit modifying a path
// outside the scope is rejected with CodeScopeViolation.
func TestWalkAndValidate_OutOfScopePath(t *testing.T) {
	repo, dir := initTestRepo(t)
	scope := allDocsScope(t) // only docs/** allowed

	c1 := makeCommit(t, repo, dir, nil,
		map[string]string{"docs/README.md": "hello"},
		goodMsg("sess-1", "1", "alice"),
	)
	// c2 touches src/ which is outside docs/**
	c2 := makeCommit(t, repo, dir, c1,
		map[string]string{"src/main.go": "package main"},
		goodMsg("sess-1", "2", "alice"),
	)

	rej := WalkAndValidate(context.Background(), repo, RefUpdate{
		Ref:    "refs/heads/jam/sess-1/alice/main",
		OldSHA: c1.Hash.String(),
		NewSHA: c2.Hash.String(),
	}, scope)

	scopeRej := filterByCode(rej, CodeScopeViolation)
	if len(scopeRej) == 0 {
		t.Errorf("expected a %s rejection, got rejections: %v", CodeScopeViolation, rej)
	}
}

// TestWalkAndValidate_BothViolations verifies that a commit can accumulate
// both a missing-trailer AND a scope-violation rejection.
func TestWalkAndValidate_BothViolations(t *testing.T) {
	repo, dir := initTestRepo(t)
	scope := allDocsScope(t)

	c1 := makeCommit(t, repo, dir, nil,
		map[string]string{"docs/README.md": "hello"},
		goodMsg("sess-1", "1", "alice"),
	)
	// c2: no trailers AND out-of-scope path.
	c2 := makeCommit(t, repo, dir, c1,
		map[string]string{"src/hack.go": "package main"},
		"no trailers here",
	)

	rej := WalkAndValidate(context.Background(), repo, RefUpdate{
		Ref:    "refs/heads/jam/sess-1/alice/main",
		OldSHA: c1.Hash.String(),
		NewSHA: c2.Hash.String(),
	}, scope)

	if filterByCode(rej, CodeMissingTrailer) == nil {
		t.Errorf("expected %s rejection", CodeMissingTrailer)
	}
	if filterByCode(rej, CodeScopeViolation) == nil {
		t.Errorf("expected %s rejection", CodeScopeViolation)
	}
}

// TestWalkAndValidate_OldSHAEqualsNewSHA verifies that no commits are visited
// when OldSHA == NewSHA (no-op update). This is an edge case but should be safe.
func TestWalkAndValidate_OldSHAEqualsNewSHA(t *testing.T) {
	repo, dir := initTestRepo(t)
	scope := openScope(t)

	c1 := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)

	rej := WalkAndValidate(context.Background(), repo, RefUpdate{
		Ref:    "refs/heads/jam/sess-1/alice/main",
		OldSHA: c1.Hash.String(),
		NewSHA: c1.Hash.String(),
	}, scope)

	if len(rej) != 0 {
		t.Errorf("no-op update: expected no rejections, got %v", rej)
	}
}

// TestWalkAndValidate_NilScope verifies that a nil scope skips the scope check
// (all paths pass).
func TestWalkAndValidate_NilScope(t *testing.T) {
	repo, dir := initTestRepo(t)

	c1 := makeCommit(t, repo, dir, nil,
		map[string]string{"anywhere/file.txt": "x"},
		goodMsg("sess-1", "1", "alice"),
	)

	rej := WalkAndValidate(context.Background(), repo, RefUpdate{
		Ref:    "refs/heads/jam/sess-1/alice/main",
		OldSHA: "",
		NewSHA: c1.Hash.String(),
	}, nil)

	// nil scope means no scope check; only trailer violations possible.
	if len(rej) != 0 {
		t.Errorf("nil scope: expected no rejections, got %v", rej)
	}
}

// filterByCode returns rejections matching code, or nil if none match.
func filterByCode(rejections []Rejection, code string) []Rejection {
	var out []Rejection
	for _, r := range rejections {
		if r.Code == code {
			out = append(out, r)
		}
	}
	return out
}
