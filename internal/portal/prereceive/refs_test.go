package prereceive

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// newBareRepo creates a fresh bare repository on disk for testing.
func newBareRepo(t *testing.T) *git.Repository {
	t.Helper()
	dir := t.TempDir()
	runCmd(t, dir, "git", "init", "--bare", dir)
	r, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("git.PlainOpen bare: %v", err)
	}
	return r
}

// runCmdOutput runs a command and returns its combined stdout+stderr output.
func runCmdOutput(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

// createRef sets a ref in the repository directly against the object store,
// pointing at the given commit hash.
func createRef(t *testing.T, repo *git.Repository, name, sha string) {
	t.Helper()
	ref := plumbing.NewHashReference(plumbing.ReferenceName(name), plumbing.NewHash(sha))
	if err := repo.Storer.SetReference(ref); err != nil {
		t.Fatalf("SetReference %s: %v", name, err)
	}
}

// ---- Namespace tests ----

// TestValidateRef_UserNamespace verifies that a ref in the authenticated
// user's namespace is accepted.
func TestValidateRef_UserNamespace(t *testing.T) {
	repo, dir := initTestRepo(t)
	c := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)

	rej := ValidateRef(context.Background(), repo,
		"sess-1", "acc-alice",
		RefUpdate{
			Ref:    "refs/heads/jam/sess-1/acc-alice/main",
			OldSHA: "",
			NewSHA: c.Hash.String(),
		})

	if len(rej) != 0 {
		t.Errorf("expected no rejections for valid user ref, got %v", rej)
	}
}

// TestValidateRef_WrongOwner verifies that a ref belonging to a different user
// is rejected with push.ref_namespace_violation.
func TestValidateRef_WrongOwner(t *testing.T) {
	repo, dir := initTestRepo(t)
	c := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)

	rej := ValidateRef(context.Background(), repo,
		"sess-1", "acc-alice",
		RefUpdate{
			Ref:    "refs/heads/jam/sess-1/acc-bob/main", // wrong owner
			OldSHA: "",
			NewSHA: c.Hash.String(),
		})

	if len(rej) != 1 {
		t.Fatalf("expected 1 rejection, got %v", rej)
	}
	if rej[0].Code != CodeRefNamespaceViolation {
		t.Errorf("code: got %q, want %q", rej[0].Code, CodeRefNamespaceViolation)
	}
}

// TestValidateRef_BaseWhenEmpty verifies that refs/heads/jam/<sess>/base is
// allowed when the repository has no refs.
//
// We use a fresh non-bare git repo BEFORE any commits exist so it has no refs,
// then build a commit object manually via the CLI in an intermediate bare repo
// and copy its hash.
func TestValidateRef_BaseWhenEmpty(t *testing.T) {
	// Create a non-bare repo WITH a commit so we have a valid hash to use.
	repoWithCommit, dir := initTestRepo(t)
	c := makeCommit(t, repoWithCommit, dir, nil,
		map[string]string{"README.md": "init"},
		goodMsg("sess-1", "1", "creator"),
	)

	// Create a SEPARATE bare repo that is completely empty (no refs) to test
	// the first-push-to-base logic.
	emptyRepo := newBareRepo(t)

	// The commit object doesn't need to exist in the empty repo for the
	// namespace check — ValidateRef only needs to determine whether the repo
	// has refs. The force-push check is skipped because OldSHA is empty.
	rej := ValidateRef(context.Background(), emptyRepo,
		"sess-1", "acc-creator",
		RefUpdate{
			Ref:    "refs/heads/jam/sess-1/base",
			OldSHA: "",
			NewSHA: c.Hash.String(),
		})

	if len(rej) != 0 {
		t.Errorf("expected no rejections for base push on empty repo, got %v", rej)
	}
}

// TestValidateRef_BaseWhenNotEmpty verifies that refs/heads/jam/<sess>/base is
// rejected once the repo already has refs.
func TestValidateRef_BaseWhenNotEmpty(t *testing.T) {
	repo, dir := initTestRepo(t)
	c := makeCommit(t, repo, dir, nil,
		map[string]string{"README.md": "init"},
		goodMsg("sess-1", "1", "creator"),
	)
	// Seed an existing ref so the repo is no longer empty.
	createRef(t, repo, "refs/heads/jam/sess-1/base", c.Hash.String())

	c2 := makeCommit(t, repo, dir, c,
		map[string]string{"README.md": "update"},
		goodMsg("sess-1", "2", "creator"),
	)

	rej := ValidateRef(context.Background(), repo,
		"sess-1", "acc-creator",
		RefUpdate{
			Ref:    "refs/heads/jam/sess-1/base",
			OldSHA: "",
			NewSHA: c2.Hash.String(),
		})

	if len(rej) != 1 {
		t.Fatalf("expected 1 rejection, got %v", rej)
	}
	if rej[0].Code != CodeRefNamespaceViolation {
		t.Errorf("code: got %q, want %q", rej[0].Code, CodeRefNamespaceViolation)
	}
}

// TestValidateRef_DraftRejected verifies that the server-managed draft ref is
// always rejected.
func TestValidateRef_DraftRejected(t *testing.T) {
	repo, dir := initTestRepo(t)
	c := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)

	rej := ValidateRef(context.Background(), repo,
		"sess-1", "acc-alice",
		RefUpdate{
			Ref:    "refs/heads/jam/sess-1/draft",
			OldSHA: "",
			NewSHA: c.Hash.String(),
		})

	if len(rej) != 1 {
		t.Fatalf("expected 1 rejection, got %v", rej)
	}
	if rej[0].Code != CodeRefNamespaceViolation {
		t.Errorf("code: got %q, want %q", rej[0].Code, CodeRefNamespaceViolation)
	}
}

// TestValidateRef_WrongSession verifies that a ref for a different session ID
// is rejected.
func TestValidateRef_WrongSession(t *testing.T) {
	repo, dir := initTestRepo(t)
	c := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-2", "1", "alice"),
	)

	rej := ValidateRef(context.Background(), repo,
		"sess-1", "acc-alice", // session sess-1 but ref says sess-2
		RefUpdate{
			Ref:    "refs/heads/jam/sess-2/acc-alice/main",
			OldSHA: "",
			NewSHA: c.Hash.String(),
		})

	if len(rej) != 1 {
		t.Fatalf("expected 1 rejection, got %v", rej)
	}
	if rej[0].Code != CodeRefNamespaceViolation {
		t.Errorf("code: got %q, want %q", rej[0].Code, CodeRefNamespaceViolation)
	}
}

// ---- Force-push tests ----

// TestValidateRef_FastForwardOK verifies that a normal fast-forward update
// (old is ancestor of new) is accepted.
func TestValidateRef_FastForwardOK(t *testing.T) {
	repo, dir := initTestRepo(t)
	c1 := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)
	c2 := makeCommit(t, repo, dir, c1,
		map[string]string{"b.txt": "2"},
		goodMsg("sess-1", "2", "alice"),
	)

	rej := ValidateRef(context.Background(), repo,
		"sess-1", "acc-alice",
		RefUpdate{
			Ref:    "refs/heads/jam/sess-1/acc-alice/main",
			OldSHA: c1.Hash.String(),
			NewSHA: c2.Hash.String(),
		})

	if len(rej) != 0 {
		t.Errorf("expected no rejections for fast-forward, got %v", rej)
	}
}

// TestValidateRef_ForcePushRejected verifies that a non-fast-forward update
// is rejected with push.force_push_rejected.
//
// To create two divergent commits, we:
//  1. Create c1 on main.
//  2. Use git CLI to create an orphan branch with a different root commit c2.
func TestValidateRef_ForcePushRejected(t *testing.T) {
	repo, dir := initTestRepo(t)

	// c1 — root commit on main.
	c1 := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)

	// Create an orphan branch via CLI so we get a true root commit unrelated
	// to c1. go-git's wt.Commit always attaches to HEAD when Parents is nil.
	runCmd(t, dir, "git", "checkout", "--orphan", "orphan-branch")
	runCmd(t, dir, "git", "rm", "-rf", ".")
	runCmd(t, dir, "bash", "-c", "echo diverged > z.txt")
	runCmd(t, dir, "git", "add", "z.txt")
	runCmd(t, dir, "git",
		"-c", "user.email=test@jamsesh.test",
		"-c", "user.name=Test",
		"commit", "-m", "orphan commit",
	)

	// Read the orphan commit hash from git.
	orphanHashOut, err := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	orphanHash := strings.TrimSpace(orphanHashOut)

	// Return to main branch so the repo state is consistent.
	runCmd(t, dir, "git", "checkout", "main")

	rej := ValidateRef(context.Background(), repo,
		"sess-1", "acc-alice",
		RefUpdate{
			Ref:    "refs/heads/jam/sess-1/acc-alice/main",
			OldSHA: c1.Hash.String(),
			NewSHA: orphanHash, // orphan root — not a descendant of c1
		})

	if len(rej) != 1 {
		t.Fatalf("expected 1 rejection, got %v", rej)
	}
	if rej[0].Code != CodeForcePushRejected {
		t.Errorf("code: got %q, want %q", rej[0].Code, CodeForcePushRejected)
	}
}

// TestValidateRef_NewRef_NoForcePushCheck verifies that when OldSHA is empty
// (new ref), no force-push check is performed.
func TestValidateRef_NewRef_NoForcePushCheck(t *testing.T) {
	repo, dir := initTestRepo(t)
	c := makeCommit(t, repo, dir, nil,
		map[string]string{"a.txt": "1"},
		goodMsg("sess-1", "1", "alice"),
	)

	rej := ValidateRef(context.Background(), repo,
		"sess-1", "acc-alice",
		RefUpdate{
			Ref:    "refs/heads/jam/sess-1/acc-alice/main",
			OldSHA: "", // new ref — no force-push check
			NewSHA: c.Hash.String(),
		})

	if len(rej) != 0 {
		t.Errorf("expected no rejections for new ref creation, got %v", rej)
	}
}
