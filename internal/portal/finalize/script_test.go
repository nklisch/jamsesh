package finalize_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"jamsesh/internal/portal/finalize"
)

// scriptInputCase describes one golden-test scenario for BuildScript.
type scriptInputCase struct {
	name           string
	mode           string
	target         string
	base           string
	shas           []string
	squashBody     string
	goldenFilename string
}

// fixedSquashBody is a sample composed message body used in the script
// goldens. It includes a body bullet list and a co-author trailer so the
// heredoc shape is exercised end-to-end.
const fixedSquashBody = `Ship the comments feature

- feat: comments REST endpoint
- feat: comments WebSocket gateway
- docs: document comments protocol

Co-authored-by: Alice Smith <alice@example.com>
Co-authored-by: Bob Tan <bob@example.com>
`

func TestBuildScript_Goldens(t *testing.T) {
	one := []string{
		"1111111111111111111111111111111111111111",
	}
	three := []string{
		"1111111111111111111111111111111111111111",
		"2222222222222222222222222222222222222222",
		"3333333333333333333333333333333333333333",
	}
	ten := []string{
		"0000000000000000000000000000000000000001",
		"0000000000000000000000000000000000000002",
		"0000000000000000000000000000000000000003",
		"0000000000000000000000000000000000000004",
		"0000000000000000000000000000000000000005",
		"0000000000000000000000000000000000000006",
		"0000000000000000000000000000000000000007",
		"0000000000000000000000000000000000000008",
		"0000000000000000000000000000000000000009",
		"000000000000000000000000000000000000000a",
	}

	const target = "ship/comments"
	const base = "fedcba9876543210fedcba9876543210fedcba98"

	cases := []scriptInputCase{
		{
			name:           "squash_1_commit",
			mode:           "squash",
			target:         target,
			base:           base,
			shas:           one,
			squashBody:     fixedSquashBody,
			goldenFilename: "squash_script_1commit.golden.txt",
		},
		{
			name:           "squash_3_commits",
			mode:           "squash",
			target:         target,
			base:           base,
			shas:           three,
			squashBody:     fixedSquashBody,
			goldenFilename: "squash_script_3commits.golden.txt",
		},
		{
			name:           "squash_10_commits",
			mode:           "squash",
			target:         target,
			base:           base,
			shas:           ten,
			squashBody:     fixedSquashBody,
			goldenFilename: "squash_script_10commits.golden.txt",
		},
		{
			name:           "preserve_1_commit",
			mode:           "preserve",
			target:         target,
			base:           base,
			shas:           one,
			goldenFilename: "preserve_script_1commit.golden.txt",
		},
		{
			name:           "preserve_3_commits",
			mode:           "preserve",
			target:         target,
			base:           base,
			shas:           three,
			goldenFilename: "preserve_script_3commits.golden.txt",
		},
		{
			name:           "preserve_10_commits",
			mode:           "preserve",
			target:         target,
			base:           base,
			shas:           ten,
			goldenFilename: "preserve_script_10commits.golden.txt",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := finalize.BuildScript(finalize.ScriptInput{
				Mode:              tc.mode,
				TargetBranch:      tc.target,
				BaseSHA:           tc.base,
				SelectedSHAs:      tc.shas,
				SquashMessageBody: tc.squashBody,
			})
			goldenPath := filepath.Join("testdata", tc.goldenFilename)
			if *updateGoldens {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v", goldenPath, err)
			}
			if got != string(want) {
				t.Errorf("golden mismatch %s\n--- got ---\n%s\n--- want ---\n%s",
					tc.goldenFilename, got, string(want))
			}
		})
	}
}

func TestBuildScript_SquashCarriesPlaceholdersAndSafeHeader(t *testing.T) {
	got := finalize.BuildScript(finalize.ScriptInput{
		Mode:              "squash",
		TargetBranch:      "ship/foo",
		BaseSHA:           "deadbeefcafebabe",
		SelectedSHAs:      []string{"abc123"},
		SquashMessageBody: "Subject\n",
	})
	mustContain := []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"$JAMSESH_FETCH_REMOTE",
		"$JAMSESH_RUNNER_NAME",
		"$JAMSESH_RUNNER_EMAIL",
		"<<'JAMSESH_MSG'",
		"JAMSESH_MSG\n",
		"==> Composing squash commit",
	}
	for _, m := range mustContain {
		if !strings.Contains(got, m) {
			t.Errorf("squash script missing %q\n--- script ---\n%s", m, got)
		}
	}
}

func TestBuildScript_PreserveOnePickPerSHA(t *testing.T) {
	shas := []string{"aaa", "bbb", "ccc"}
	got := finalize.BuildScript(finalize.ScriptInput{
		Mode:         "preserve",
		TargetBranch: "ship/x",
		BaseSHA:      "deadbeef",
		SelectedSHAs: shas,
	})
	for _, sha := range shas {
		if !strings.Contains(got, "git cherry-pick "+sha) {
			t.Errorf("preserve script missing git cherry-pick %s", sha)
		}
	}
	// Should NOT contain --no-commit (that's the squash variant).
	if strings.Contains(got, "--no-commit") {
		t.Error("preserve script should not contain --no-commit")
	}
	// Should NOT have a heredoc / JAMSESH_MSG sentinel.
	if strings.Contains(got, "JAMSESH_MSG") {
		t.Error("preserve script should not contain JAMSESH_MSG heredoc")
	}
}

func TestBuildScript_Deterministic(t *testing.T) {
	in := finalize.ScriptInput{
		Mode:              "squash",
		TargetBranch:      "ship/det",
		BaseSHA:           "deadbeef",
		SelectedSHAs:      []string{"a", "b", "c"},
		SquashMessageBody: "Subject\n",
	}
	a := finalize.BuildScript(in)
	b := finalize.BuildScript(in)
	if a != b {
		t.Error("BuildScript not deterministic across two invocations")
	}
}

// ---------- FirstParentLeafCommits ----------

// commitOpts is a thin wrapper to create commits via go-git's object store
// directly (no worktree needed). Used to deterministically build a chain
// with an auto-merger merge in the middle.
type commitFixture struct {
	repo *gogit.Repository
}

func newCommitFixture(t *testing.T) *commitFixture {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	// Ensure a 'main' branch exists for tip operations.
	_ = repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/main")))
	if err := repo.CreateBranch(&config.Branch{Name: "main"}); err != nil && err != gogit.ErrBranchExists {
		// not fatal; not strictly required for the test
	}
	return &commitFixture{repo: repo}
}

// makeCommit creates a commit with the given message and parent hashes.
// Tree is the empty tree (we don't care about contents for the leaf-walk
// test). The committer time defaults to a fixed timestamp + idx*minute so
// chronological order is well-defined.
func (f *commitFixture) makeCommit(t *testing.T, msg string, parents []plumbing.Hash, idx int) plumbing.Hash {
	t.Helper()
	when := time.Date(2026, 5, 17, 12, idx, 0, 0, time.UTC)
	sig := object.Signature{Name: "Test", Email: "t@x", When: when}

	// Build an empty tree object.
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

// TestFirstParentLeafCommits_AutoMergerInMiddle builds a 5-commit draft
// chain shaped like:
//
//	A → B → M(merge) → D → E
//	          ↘
//	           X → Y    (side branch integrated at M)
//
// where M is an auto-merger merge of (B, Y). FirstParentLeafCommits(E)
// should return [A, B, X, Y, D, E] — the auto-merger merge itself is
// excluded; the integrated leaves X, Y appear in chronological order at
// the point of the merge.
func TestFirstParentLeafCommits_AutoMergerInMiddle(t *testing.T) {
	f := newCommitFixture(t)

	hA := f.makeCommit(t, "feat: A\n", nil, 1)
	hB := f.makeCommit(t, "feat: B\n", []plumbing.Hash{hA}, 2)

	// Side branch off of A.
	hX := f.makeCommit(t, "feat: X side\n", []plumbing.Hash{hA}, 3)
	hY := f.makeCommit(t, "feat: Y side\n", []plumbing.Hash{hX}, 4)

	// Auto-merger merge of B and Y.
	mergeMsg := "Merge agent X/Y into draft\n\nAuto-Merger: true\n"
	hM := f.makeCommit(t, mergeMsg, []plumbing.Hash{hB, hY}, 5)

	hD := f.makeCommit(t, "feat: D\n", []plumbing.Hash{hM}, 6)
	hE := f.makeCommit(t, "feat: E\n", []plumbing.Hash{hD}, 7)

	got, err := finalize.FirstParentLeafCommits(f.repo, hE.String())
	if err != nil {
		t.Fatalf("FirstParentLeafCommits: %v", err)
	}

	// Should be [A, B, X, Y, D, E] — 6 commits, no merge commit.
	wantHashes := []plumbing.Hash{hA, hB, hX, hY, hD, hE}
	if len(got) != len(wantHashes) {
		t.Fatalf("returned commit count: got %d, want %d (commits: %v)", len(got), len(wantHashes), leafSubjects(got))
	}
	for i, w := range wantHashes {
		if got[i].Hash != w {
			t.Errorf("commit %d: got %s, want %s (subject=%q)",
				i, got[i].Hash, w, firstLineOnly(got[i].Message))
		}
	}

	// Sanity: the auto-merger commit must NOT be in the output.
	for _, c := range got {
		if c.Hash == hM {
			t.Errorf("auto-merger merge commit %s should not be in output", hM)
		}
	}
}

func TestFirstParentLeafCommits_LinearChain(t *testing.T) {
	f := newCommitFixture(t)
	hA := f.makeCommit(t, "feat: A\n", nil, 1)
	hB := f.makeCommit(t, "feat: B\n", []plumbing.Hash{hA}, 2)
	hC := f.makeCommit(t, "feat: C\n", []plumbing.Hash{hB}, 3)

	got, err := finalize.FirstParentLeafCommits(f.repo, hC.String())
	if err != nil {
		t.Fatalf("FirstParentLeafCommits: %v", err)
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
	f := newCommitFixture(t)
	hA := f.makeCommit(t, "feat: root\n", nil, 1)
	got, err := finalize.FirstParentLeafCommits(f.repo, hA.String())
	if err != nil {
		t.Fatalf("FirstParentLeafCommits: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d commits, want 1", len(got))
	}
	if got[0].Hash != hA {
		t.Errorf("got %s, want %s", got[0].Hash, hA)
	}
}

// leafSubjects extracts first-line subjects from a slice of *object.Commit
// for nicer test diagnostics.
func leafSubjects(commits []*object.Commit) []string {
	out := make([]string, 0, len(commits))
	for _, c := range commits {
		out = append(out, firstLineOnly(c.Message))
	}
	return out
}

func firstLineOnly(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
