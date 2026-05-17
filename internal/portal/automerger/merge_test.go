package automerger_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"jamsesh/internal/portal/automerger"
)

// ---------------------------------------------------------------------------
// testdata corpus runner
// ---------------------------------------------------------------------------

// expectedResult mirrors the shape of expected.json in each testdata scenario.
type expectedResult struct {
	Kind        string              `json:"kind"`
	Description string              `json:"description"`
	Conflicts   []expectedConflict  `json:"conflicts"`
}

type expectedConflict struct {
	File   string              `json:"file"`
	Ranges []expectedLineRange `json:"ranges"`
}

type expectedLineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// TestCorpus walks testdata/ and for each scenario directory that contains an
// expected.json, builds a synthetic git repo, calls Merge, and asserts the
// result matches expectations.
func TestCorpus(t *testing.T) {
	t.Helper()

	corpusRoot := "testdata"
	scenarios := collectScenarios(t, corpusRoot)
	if len(scenarios) == 0 {
		t.Fatal("no testdata scenarios found")
	}

	for _, scenDir := range scenarios {
		scenDir := scenDir // capture
		name := relScenarioName(corpusRoot, scenDir)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runScenario(t, scenDir)
		})
	}
}

// collectScenarios returns all leaf directories under root that contain an
// expected.json file.
func collectScenarios(t *testing.T, root string) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if _, statErr := os.Stat(filepath.Join(path, "expected.json")); statErr == nil {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("collectScenarios walk: %v", err)
	}
	return dirs
}

func relScenarioName(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return dir
	}
	return strings.ReplaceAll(rel, string(filepath.Separator), "/")
}

// runScenario loads the expected.json, builds a synthetic repo for the
// scenario kind, calls Merge, and asserts.
func runScenario(t *testing.T, dir string) {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, "expected.json"))
	if err != nil {
		t.Fatalf("read expected.json: %v", err)
	}
	var expected expectedResult
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatalf("parse expected.json: %v", err)
	}

	// Determine which scenario builder to use based on directory name.
	scenario := filepath.Base(dir)
	parent := filepath.Base(filepath.Dir(dir))

	var result automerger.MergeResult
	switch parent + "/" + scenario {
	case "clean/disjoint-files":
		result = buildDisjointFilesScenario(t, dir)
	case "clean/disjoint-lines":
		result = buildDisjointLinesScenario(t, dir)
	case "hard-conflict/same-line-different":
		result = buildSameLineDifferentScenario(t, dir)
	case "hard-conflict/delete-vs-modify":
		result = buildDeleteVsModifyScenario(t, dir)
	default:
		t.Skipf("no builder for scenario %s/%s", parent, scenario)
	}

	// Assert kind.
	if string(result.Kind) != expected.Kind {
		t.Errorf("kind: got %q, want %q", result.Kind, expected.Kind)
	}

	// For clean merges, verify a non-empty tree SHA.
	if expected.Kind == "clean-merge" {
		if result.MergedTreeSHA == "" {
			t.Error("CleanMerge result has empty MergedTreeSHA")
		}
	}

	// For hard conflicts, verify Conflicts is non-empty and matches files.
	if expected.Kind == "hard-conflict" {
		if len(result.Conflicts) == 0 {
			t.Error("HardConflict result has empty Conflicts slice")
		}
		for _, ec := range expected.Conflicts {
			found := false
			for _, ac := range result.Conflicts {
				if ac.File == ec.File {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected conflict for file %q not found; got: %v", ec.File, conflictFiles(result.Conflicts))
			}
		}
	}
}

func conflictFiles(cs []automerger.Conflict) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.File)
	}
	return out
}

// ---------------------------------------------------------------------------
// Scenario builders
// ---------------------------------------------------------------------------

// buildDisjointFilesScenario: base has file.txt; ours adds file-a.txt;
// theirs adds file-b.txt.  No overlap → CleanMerge.
func buildDisjointFilesScenario(t *testing.T, dir string) automerger.MergeResult {
	t.Helper()
	ctx := context.Background()

	repo, repoDir := initRepo(t)
	_ = repoDir

	baseContent, _ := os.ReadFile(filepath.Join(dir, "base.txt"))

	// Commit base: file.txt with base.txt content.
	baseCommit := commitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": baseContent,
	}, "base commit")

	// Ours (draftTip): add file-a.txt.
	oursCommit := commitFiles(t, repo, repoDir, baseCommit, map[string][]byte{
		"file.txt":  baseContent,
		"file-a.txt": []byte("added by ours\n"),
	}, "ours: add file-a.txt")

	// Theirs (source): add file-b.txt.
	theirsCommit := commitFiles(t, repo, repoDir, baseCommit, map[string][]byte{
		"file.txt":  baseContent,
		"file-b.txt": []byte("added by theirs\n"),
	}, "theirs: add file-b.txt")

	result, err := automerger.Merge(ctx, repo, theirsCommit, oursCommit, baseCommit)
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}
	return result
}

// buildDisjointLinesScenario: same file, ours edits line 2, theirs edits
// line 4 → git merge-file resolves cleanly.
func buildDisjointLinesScenario(t *testing.T, dir string) automerger.MergeResult {
	t.Helper()
	ctx := context.Background()

	repo, repoDir := initRepo(t)

	baseContent, _ := os.ReadFile(filepath.Join(dir, "base.txt"))
	oursContent, _ := os.ReadFile(filepath.Join(dir, "ours.txt"))
	theirsContent, _ := os.ReadFile(filepath.Join(dir, "theirs.txt"))

	baseCommit := commitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": baseContent,
	}, "base")
	oursCommit := commitFiles(t, repo, repoDir, baseCommit, map[string][]byte{
		"file.txt": oursContent,
	}, "ours")
	theirsCommit := commitFiles(t, repo, repoDir, baseCommit, map[string][]byte{
		"file.txt": theirsContent,
	}, "theirs")

	result, err := automerger.Merge(ctx, repo, theirsCommit, oursCommit, baseCommit)
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}
	return result
}

// buildSameLineDifferentScenario: both sides modify the exact same line
// differently → HardConflict.
func buildSameLineDifferentScenario(t *testing.T, dir string) automerger.MergeResult {
	t.Helper()
	ctx := context.Background()

	repo, repoDir := initRepo(t)

	baseContent, _ := os.ReadFile(filepath.Join(dir, "base.txt"))
	oursContent, _ := os.ReadFile(filepath.Join(dir, "ours.txt"))
	theirsContent, _ := os.ReadFile(filepath.Join(dir, "theirs.txt"))

	baseCommit := commitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": baseContent,
	}, "base")
	oursCommit := commitFiles(t, repo, repoDir, baseCommit, map[string][]byte{
		"file.txt": oursContent,
	}, "ours")
	theirsCommit := commitFiles(t, repo, repoDir, baseCommit, map[string][]byte{
		"file.txt": theirsContent,
	}, "theirs")

	result, err := automerger.Merge(ctx, repo, theirsCommit, oursCommit, baseCommit)
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}
	return result
}

// buildDeleteVsModifyScenario: ours deletes file.txt, theirs modifies it
// → HardConflict.
func buildDeleteVsModifyScenario(t *testing.T, dir string) automerger.MergeResult {
	t.Helper()
	ctx := context.Background()

	repo, repoDir := initRepo(t)

	baseContent, _ := os.ReadFile(filepath.Join(dir, "base.txt"))
	theirsContent := append([]byte(nil), baseContent...)
	theirsContent = append(theirsContent, []byte("an extra line added by theirs\n")...)

	baseCommit := commitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": baseContent,
	}, "base")

	// Ours deletes file.txt by committing without it.
	oursCommit := commitFiles(t, repo, repoDir, baseCommit, map[string][]byte{}, "ours: delete file.txt")

	// Theirs modifies file.txt.
	theirsCommit := commitFiles(t, repo, repoDir, baseCommit, map[string][]byte{
		"file.txt": theirsContent,
	}, "theirs: modify file.txt")

	result, err := automerger.Merge(ctx, repo, theirsCommit, oursCommit, baseCommit)
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}
	return result
}

// ---------------------------------------------------------------------------
// Short-circuit tests (no corpus directory needed)
// ---------------------------------------------------------------------------

// TestShortCircuitSourceEqualsAncestor verifies that when source == ancestor
// the function returns CleanMerge with draftTip's tree.
func TestShortCircuitSourceEqualsAncestor(t *testing.T) {
	ctx := context.Background()
	repo, repoDir := initRepo(t)

	baseCommit := commitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": []byte("base\n"),
	}, "base")
	draftCommit := commitFiles(t, repo, repoDir, baseCommit, map[string][]byte{
		"file.txt": []byte("draft changes\n"),
	}, "draft")

	// source == ancestor  (i.e., source is the same as ancestor)
	result, err := automerger.Merge(ctx, repo, baseCommit, draftCommit, baseCommit)
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}
	if result.Kind != automerger.CleanMerge {
		t.Errorf("kind: got %v, want CleanMerge", result.Kind)
	}
	draftTree, _ := draftCommit.Tree()
	if result.MergedTreeSHA != draftTree.Hash.String() {
		t.Errorf("MergedTreeSHA: got %s, want draftTip tree %s", result.MergedTreeSHA, draftTree.Hash.String())
	}
}

// TestShortCircuitDraftTipEqualsAncestor verifies that when draftTip ==
// ancestor the function returns CleanMerge with source's tree.
func TestShortCircuitDraftTipEqualsAncestor(t *testing.T) {
	ctx := context.Background()
	repo, repoDir := initRepo(t)

	baseCommit := commitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": []byte("base\n"),
	}, "base")
	sourceCommit := commitFiles(t, repo, repoDir, baseCommit, map[string][]byte{
		"file.txt": []byte("source changes\n"),
	}, "source")

	// draftTip == ancestor
	result, err := automerger.Merge(ctx, repo, sourceCommit, baseCommit, baseCommit)
	if err != nil {
		t.Fatalf("Merge error: %v", err)
	}
	if result.Kind != automerger.CleanMerge {
		t.Errorf("kind: got %v, want CleanMerge", result.Kind)
	}
	sourceTree, _ := sourceCommit.Tree()
	if result.MergedTreeSHA != sourceTree.Hash.String() {
		t.Errorf("MergedTreeSHA: got %s, want source tree %s", result.MergedTreeSHA, sourceTree.Hash.String())
	}
}

// ---------------------------------------------------------------------------
// ParseConflictRanges unit tests
// ---------------------------------------------------------------------------

func TestParseConflictRanges_SingleRegion(t *testing.T) {
	content := `line 1
<<<<<<< ours
our change
=======
their change
>>>>>>> theirs
line 7
`
	ranges := automerger.ParseConflictRanges([]byte(content))
	if len(ranges) != 1 {
		t.Fatalf("expected 1 range, got %d: %v", len(ranges), ranges)
	}
	if ranges[0].Start != 2 || ranges[0].End != 6 {
		t.Errorf("range: got {%d,%d}, want {2,6}", ranges[0].Start, ranges[0].End)
	}
}

func TestParseConflictRanges_MultiRegion(t *testing.T) {
	content := `context line
<<<<<<< ours
ours block A
=======
theirs block A
>>>>>>> theirs
middle context
<<<<<<< ours
ours block B
=======
theirs block B
>>>>>>> theirs
trailing context
`
	ranges := automerger.ParseConflictRanges([]byte(content))
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d: %v", len(ranges), ranges)
	}
	if ranges[0].Start != 2 || ranges[0].End != 6 {
		t.Errorf("range[0]: got {%d,%d}, want {2,6}", ranges[0].Start, ranges[0].End)
	}
	if ranges[1].Start != 8 || ranges[1].End != 12 {
		t.Errorf("range[1]: got {%d,%d}, want {8,12}", ranges[1].Start, ranges[1].End)
	}
}

func TestParseConflictRanges_NoConflicts(t *testing.T) {
	content := "line 1\nline 2\nline 3\n"
	ranges := automerger.ParseConflictRanges([]byte(content))
	if len(ranges) != 0 {
		t.Errorf("expected 0 ranges, got %d", len(ranges))
	}
}

// ---------------------------------------------------------------------------
// Synthetic repo helpers
// ---------------------------------------------------------------------------

// initRepo creates a temporary non-bare git repository and returns the opened
// go-git Repository handle plus the working directory path. The repo is
// automatically cleaned up when the test ends.
func initRepo(t *testing.T) (*gogit.Repository, string) {
	t.Helper()
	dir := t.TempDir()

	// Use git CLI for init so we get a proper HEAD ref.
	run(t, dir, "git", "init", "-b", "main")
	run(t, dir, "git", "config", "user.email", "test@jamsesh.test")
	run(t, dir, "git", "config", "user.name", "Test")

	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	return repo, dir
}

// commitFiles writes the given files to the working tree (removing all others),
// stages everything, and creates a commit with the given message. If parent is
// non-nil the commit records it as its sole parent (for simplicity — we use
// the go-git object model directly so we don't need the worktree to track
// parents).
//
// The returned *object.Commit is fully resolved from the repo.
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

	// If there are no files at all, create a placeholder so git doesn't
	// complain about nothing to commit.
	if len(files) == 0 {
		// We need at least a .gitkeep so the tree isn't empty.
		run(t, repoDir, "git", "rm", "--cached", "-r", "--ignore-unmatch", ".")
	}

	// Build the commit via go-git object model so we can set the parent
	// explicitly (git CLI doesn't easily let us create detached commits with
	// an arbitrary parent).
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	status, err := wt.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	// Stage everything.
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
		// Fallback: try with allow-empty.
		hash, err = wt.Commit(message, &gogit.CommitOptions{
			Author:          &sig,
			Committer:       &sig,
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

// run executes a command in dir and fails the test on error.
func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run %v: %v\n%s", args, err, out)
	}
}
