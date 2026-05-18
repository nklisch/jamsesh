package automerger

// Internal test helpers used by addressing_test.go (package automerger).
// The external test package (automerger_test) has its own copies in merge_test.go
// and outcomes_test.go; Go compiles them as separate packages so there is no
// symbol collision.

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func initRepo(t *testing.T) (*gogit.Repository, string) {
	t.Helper()
	dir := t.TempDir()

	run(t, dir, "git", "init", "-b", "main")
	run(t, dir, "git", "config", "user.email", "test@jamsesh.test")
	run(t, dir, "git", "config", "user.name", "Test")

	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	return repo, dir
}

func commitFiles(
	t *testing.T,
	repo *gogit.Repository,
	repoDir string,
	parent *object.Commit,
	files map[string][]byte,
	message string,
) *object.Commit {
	t.Helper()

	// Clean working directory.
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		t.Fatalf("commitFiles ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		_ = os.RemoveAll(filepath.Join(repoDir, e.Name()))
	}

	// Write the target files.
	for name, content := range files {
		fullPath := filepath.Join(repoDir, name)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
			t.Fatalf("commitFiles mkdir: %v", err)
		}
		if err := os.WriteFile(fullPath, content, 0o600); err != nil {
			t.Fatalf("commitFiles write %s: %v", name, err)
		}
	}

	run(t, repoDir, "git", "add", "-A")

	if len(files) == 0 {
		run(t, repoDir, "git", "rm", "--cached", "-r", "--ignore-unmatch", ".")
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	status, err := wt.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	for f := range status {
		_, _ = wt.Add(f)
	}

	sig := object.Signature{
		Name:  "Test",
		Email: "test@jamsesh.test",
		When:  time.Now(),
	}

	opts := &gogit.CommitOptions{
		Author:    &sig,
		Committer: &sig,
	}
	if parent != nil {
		opts.Parents = []plumbing.Hash{parent.Hash}
	}

	hash, err := wt.Commit(message, opts)
	if err != nil {
		hash, err = wt.Commit(message, &gogit.CommitOptions{
			Author:    &sig,
			Committer: &sig,
			AllowEmptyCommits: true,
			Parents: func() []plumbing.Hash {
				if parent != nil {
					return []plumbing.Hash{parent.Hash}
				}
				return nil
			}(),
		})
		if err != nil {
			t.Fatalf("commit %q: %v", message, err)
		}
	}

	commit, err := repo.CommitObject(hash)
	if err != nil {
		t.Fatalf("CommitObject after commit: %v", err)
	}
	return commit
}

func commitFilesWithMessage(
	t *testing.T,
	repo *gogit.Repository,
	repoDir string,
	parent *object.Commit,
	files map[string][]byte,
	message string,
) *object.Commit {
	t.Helper()
	staged := commitFiles(t, repo, repoDir, parent, files, "tmp-stage")

	sig := object.Signature{
		Name:  "Test",
		Email: "test@jamsesh.test",
		When:  time.Now().UTC(),
	}
	var parents []plumbing.Hash
	if parent != nil {
		parents = []plumbing.Hash{parent.Hash}
	}
	c := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      message,
		TreeHash:     staged.TreeHash,
		ParentHashes: parents,
	}
	obj := repo.Storer.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		t.Fatalf("encode commit: %v", err)
	}
	h, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		t.Fatalf("store commit: %v", err)
	}
	result, err := object.GetCommit(repo.Storer, h)
	if err != nil {
		t.Fatalf("get commit: %v", err)
	}
	return result
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run %v: %v\n%s", args, err, out)
	}
}
