package finalize

// Internal tests for firstParentLeafCommits — uses the package-private symbol.

import (
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// commitFixtureInternal is a thin wrapper to create commits via go-git's object
// store directly (no worktree needed). Used to deterministically build a chain
// with an auto-merger merge in the middle.
type commitFixtureInternal struct {
	repo *gogit.Repository
}

func newCommitFixtureInternal(t *testing.T) *commitFixtureInternal {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	_ = repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/main")))
	if err := repo.CreateBranch(&config.Branch{Name: "main"}); err != nil && err != gogit.ErrBranchExists {
		// not fatal
	}
	return &commitFixtureInternal{repo: repo}
}

func (f *commitFixtureInternal) makeCommit(t *testing.T, msg string, parents []plumbing.Hash, idx int) plumbing.Hash {
	t.Helper()
	when := time.Date(2026, 5, 17, 12, idx, 0, 0, time.UTC)
	sig := object.Signature{Name: "Test", Email: "t@x", When: when}

	tree := &object.Tree{}
	te := f.repo.Storer.NewEncodedObject()
	te.SetType(plumbing.TreeObject)
	if err := tree.Encode(te); err != nil {
		t.Fatalf("encode tree: %v", err)
	}
	treeHash, err := f.repo.Storer.SetEncodedObject(te)
	if err != nil {
		t.Fatalf("store tree: %v", err)
	}

	commit := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      msg,
		TreeHash:     treeHash,
		ParentHashes: parents,
	}
	ce := f.repo.Storer.NewEncodedObject()
	ce.SetType(plumbing.CommitObject)
	if err := commit.Encode(ce); err != nil {
		t.Fatalf("encode commit: %v", err)
	}
	hash, err := f.repo.Storer.SetEncodedObject(ce)
	if err != nil {
		t.Fatalf("store commit: %v", err)
	}
	return hash
}

// leafSubjectsInternal extracts first-line subjects from a slice of *object.Commit
// for nicer test diagnostics.
func leafSubjectsInternal(commits []*object.Commit) []string {
	out := make([]string, 0, len(commits))
	for _, c := range commits {
		out = append(out, firstLineOnlyInternal(c.Message))
	}
	return out
}

func firstLineOnlyInternal(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// TestFirstParentLeafCommits_AutoMergerInMiddle builds a 5-commit draft
// chain shaped like:
//
//	A → B → M(merge) → D → E
//	          ↘
//	           X → Y    (side branch integrated at M)
//
// where M is an auto-merger merge of (B, Y). firstParentLeafCommits(E)
// should return [A, B, X, Y, D, E] — the auto-merger merge itself is
// excluded; the integrated leaves X, Y appear in chronological order at
// the point of the merge.
func TestFirstParentLeafCommits_AutoMergerInMiddle(t *testing.T) {
	f := newCommitFixtureInternal(t)

	hA := f.makeCommit(t, "feat: A\n", nil, 1)
	hB := f.makeCommit(t, "feat: B\n", []plumbing.Hash{hA}, 2)

	hX := f.makeCommit(t, "feat: X side\n", []plumbing.Hash{hA}, 3)
	hY := f.makeCommit(t, "feat: Y side\n", []plumbing.Hash{hX}, 4)

	mergeMsg := "Merge agent X/Y into draft\n\nAuto-Merger: true\n"
	hM := f.makeCommit(t, mergeMsg, []plumbing.Hash{hB, hY}, 5)

	hD := f.makeCommit(t, "feat: D\n", []plumbing.Hash{hM}, 6)
	hE := f.makeCommit(t, "feat: E\n", []plumbing.Hash{hD}, 7)

	got, err := firstParentLeafCommits(f.repo, hE.String())
	if err != nil {
		t.Fatalf("firstParentLeafCommits: %v", err)
	}

	wantHashes := []plumbing.Hash{hA, hB, hX, hY, hD, hE}
	if len(got) != len(wantHashes) {
		t.Fatalf("returned commit count: got %d, want %d (commits: %v)", len(got), len(wantHashes), leafSubjectsInternal(got))
	}
	for i, w := range wantHashes {
		if got[i].Hash != w {
			t.Errorf("commit %d: got %s, want %s (subject=%q)",
				i, got[i].Hash, w, firstLineOnlyInternal(got[i].Message))
		}
	}

	for _, c := range got {
		if c.Hash == hM {
			t.Errorf("auto-merger merge commit %s should not be in output", hM)
		}
	}
}

func TestFirstParentLeafCommits_LinearChain(t *testing.T) {
	f := newCommitFixtureInternal(t)
	hA := f.makeCommit(t, "feat: A\n", nil, 1)
	hB := f.makeCommit(t, "feat: B\n", []plumbing.Hash{hA}, 2)
	hC := f.makeCommit(t, "feat: C\n", []plumbing.Hash{hB}, 3)

	got, err := firstParentLeafCommits(f.repo, hC.String())
	if err != nil {
		t.Fatalf("firstParentLeafCommits: %v", err)
	}
	wantHashes := []plumbing.Hash{hA, hB, hC}
	if len(got) != 3 {
		t.Fatalf("got %d commits, want 3", len(got))
	}
	for i, w := range wantHashes {
		if got[i].Hash != w {
			t.Errorf("commit %d: got %s, want %s", i, got[i].Hash, w)
		}
	}
}

func TestFirstParentLeafCommits_SingleCommit(t *testing.T) {
	f := newCommitFixtureInternal(t)
	hA := f.makeCommit(t, "feat: root\n", nil, 1)
	got, err := firstParentLeafCommits(f.repo, hA.String())
	if err != nil {
		t.Fatalf("firstParentLeafCommits: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d commits, want 1", len(got))
	}
	if got[0].Hash != hA {
		t.Errorf("got %s, want %s", got[0].Hash, hA)
	}
}
