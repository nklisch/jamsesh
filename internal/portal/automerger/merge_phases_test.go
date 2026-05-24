package automerger

// Phase-level unit tests for the four extracted merge phases.
// These tests access package-private functions directly to exercise each
// phase in isolation with hand-rolled inputs.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ---------------------------------------------------------------------------
// Phase 1: tryShortCircuit
// ---------------------------------------------------------------------------

// TestTryShortCircuit_SourceEqualsAncestor verifies that when source ==
// ancestor tryShortCircuit returns (CleanMerge with draftTip tree, true, nil).
func TestTryShortCircuit_SourceEqualsAncestor(t *testing.T) {
	repo, repoDir := phaseInitRepo(t)

	base := phaseCommitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": []byte("base\n"),
	}, "base")
	draft := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"file.txt": []byte("draft\n"),
	}, "draft")

	// source == ancestor (both are base)
	result, done, err := tryShortCircuit(base, draft, base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !done {
		t.Fatal("expected done=true")
	}
	if result.Kind != CleanMerge {
		t.Errorf("kind: got %v, want CleanMerge", result.Kind)
	}
	draftTree, _ := draft.Tree()
	if result.MergedTreeSHA != draftTree.Hash.String() {
		t.Errorf("MergedTreeSHA: got %s, want draftTip tree %s", result.MergedTreeSHA, draftTree.Hash.String())
	}
}

// TestTryShortCircuit_DraftTipEqualsAncestor verifies that when draftTip ==
// ancestor tryShortCircuit returns (CleanMerge with source tree, true, nil).
func TestTryShortCircuit_DraftTipEqualsAncestor(t *testing.T) {
	repo, repoDir := phaseInitRepo(t)

	base := phaseCommitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": []byte("base\n"),
	}, "base")
	source := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"file.txt": []byte("source\n"),
	}, "source")

	// draftTip == ancestor (both are base)
	result, done, err := tryShortCircuit(source, base, base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !done {
		t.Fatal("expected done=true")
	}
	if result.Kind != CleanMerge {
		t.Errorf("kind: got %v, want CleanMerge", result.Kind)
	}
	sourceTree, _ := source.Tree()
	if result.MergedTreeSHA != sourceTree.Hash.String() {
		t.Errorf("MergedTreeSHA: got %s, want source tree %s", result.MergedTreeSHA, sourceTree.Hash.String())
	}
}

// TestTryShortCircuit_NoShortCircuit verifies that when both commits differ
// from ancestor tryShortCircuit returns done=false.
func TestTryShortCircuit_NoShortCircuit(t *testing.T) {
	repo, repoDir := phaseInitRepo(t)

	base := phaseCommitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": []byte("base\n"),
	}, "base")
	source := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"file.txt": []byte("source\n"),
	}, "source")
	draft := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"file.txt": []byte("draft\n"),
	}, "draft")

	_, done, err := tryShortCircuit(source, draft, base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if done {
		t.Fatal("expected done=false when neither side equals ancestor")
	}
}

// ---------------------------------------------------------------------------
// Phase 2: computeMergeDiff
// ---------------------------------------------------------------------------

// TestComputeMergeDiff_DisjointChanges verifies that computeMergeDiff correctly
// populates ourChanges and theirChanges for disjoint file additions, and that
// mergedEntries starts as a snapshot of the base tree.
func TestComputeMergeDiff_DisjointChanges(t *testing.T) {
	repo, repoDir := phaseInitRepo(t)
	ctx := context.Background()

	base := phaseCommitFiles(t, repo, repoDir, nil, map[string][]byte{
		"shared.txt": []byte("shared\n"),
	}, "base")
	ours := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"shared.txt": []byte("shared\n"),
		"ours.txt":   []byte("ours\n"),
	}, "ours")
	theirs := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"shared.txt":  []byte("shared\n"),
		"theirs.txt": []byte("theirs\n"),
	}, "theirs")

	diff, err := computeMergeDiff(ctx, repo, base, ours, theirs)
	if err != nil {
		t.Fatalf("computeMergeDiff error: %v", err)
	}

	if _, ok := diff.ourChanges["ours.txt"]; !ok {
		t.Error("ourChanges missing ours.txt")
	}
	if _, ok := diff.theirChanges["theirs.txt"]; !ok {
		t.Error("theirChanges missing theirs.txt")
	}
	// pathSet must contain both changed paths.
	if _, ok := diff.pathSet["ours.txt"]; !ok {
		t.Error("pathSet missing ours.txt")
	}
	if _, ok := diff.pathSet["theirs.txt"]; !ok {
		t.Error("pathSet missing theirs.txt")
	}
	// mergedEntries starts from base — shared.txt must be present.
	if _, ok := diff.mergedEntries["shared.txt"]; !ok {
		t.Error("mergedEntries missing shared.txt from base tree")
	}
}

// ---------------------------------------------------------------------------
// Phase 3: applyChangesPerPath
// ---------------------------------------------------------------------------

// TestApplyChangesPerPath_OnlyTheirs verifies that when only theirs changed a
// path the merged entry adopts theirs' hash.
func TestApplyChangesPerPath_OnlyTheirs(t *testing.T) {
	repo, repoDir := phaseInitRepo(t)
	ctx := context.Background()

	baseContent := []byte("original\n")
	theirsContent := []byte("modified by theirs\n")

	base := phaseCommitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": baseContent,
	}, "base")
	ours := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"file.txt": baseContent, // ours unchanged
	}, "ours")
	theirs := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"file.txt": theirsContent,
	}, "theirs")

	diff, err := computeMergeDiff(ctx, repo, base, ours, theirs)
	if err != nil {
		t.Fatalf("computeMergeDiff: %v", err)
	}
	state, err := applyChangesPerPath(repo, diff)
	if err != nil {
		t.Fatalf("applyChangesPerPath: %v", err)
	}

	if len(state.hardConflicts) != 0 {
		t.Errorf("expected no hard conflicts, got %v", state.hardConflicts)
	}
	if len(state.conflictedFiles) != 0 {
		t.Errorf("expected no conflicted files, got %d", len(state.conflictedFiles))
	}
	entry, ok := state.mergedEntries["file.txt"]
	if !ok {
		t.Fatal("file.txt missing from mergedEntries")
	}
	// Entry hash must match theirs' blob.
	theirTree, _ := theirs.Tree()
	theirEntry, _ := theirTree.File("file.txt")
	if entry.hash != theirEntry.Blob.Hash {
		t.Errorf("mergedEntries[file.txt] hash mismatch: got %s, want %s (theirs)", entry.hash, theirEntry.Blob.Hash)
	}
}

// TestApplyChangesPerPath_HardConflict_DeleteVsModify verifies that a
// delete-vs-modify pattern is classified as a hard conflict.
func TestApplyChangesPerPath_HardConflict_DeleteVsModify(t *testing.T) {
	repo, repoDir := phaseInitRepo(t)
	ctx := context.Background()

	baseContent := []byte("content\n")
	theirsContent := []byte("content modified\n")

	base := phaseCommitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": baseContent,
	}, "base")
	// ours deletes file.txt
	ours := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{}, "ours: delete")
	// theirs modifies file.txt
	theirs := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"file.txt": theirsContent,
	}, "theirs: modify")

	diff, err := computeMergeDiff(ctx, repo, base, ours, theirs)
	if err != nil {
		t.Fatalf("computeMergeDiff: %v", err)
	}
	state, err := applyChangesPerPath(repo, diff)
	if err != nil {
		t.Fatalf("applyChangesPerPath: %v", err)
	}

	if len(state.hardConflicts) == 0 {
		t.Error("expected a hard conflict for delete-vs-modify")
	}
	found := false
	for _, c := range state.hardConflicts {
		if c.File == "file.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("hard conflict for file.txt not found; got: %v", state.hardConflicts)
	}
}

// TestApplyChangesPerPath_BothAddedSameContent verifies that when both sides
// independently add a file with identical content the merged result keeps the
// file without a conflict (identical-edit fast-path in mergeBothModifiedPath).
func TestApplyChangesPerPath_BothAddedSameContent(t *testing.T) {
	repo, repoDir := phaseInitRepo(t)
	ctx := context.Background()

	// Base has no "new.txt"; both ours and theirs add it with the same bytes.
	sharedContent := []byte("added by both\n")

	base := phaseCommitFiles(t, repo, repoDir, nil, map[string][]byte{
		"existing.txt": []byte("base content\n"),
	}, "base")
	ours := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"existing.txt": []byte("base content\n"),
		"new.txt":      sharedContent,
	}, "ours: add new.txt")
	theirs := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"existing.txt": []byte("base content\n"),
		"new.txt":      sharedContent,
	}, "theirs: add new.txt same content")

	diff, err := computeMergeDiff(ctx, repo, base, ours, theirs)
	if err != nil {
		t.Fatalf("computeMergeDiff: %v", err)
	}
	state, err := applyChangesPerPath(repo, diff)
	if err != nil {
		t.Fatalf("applyChangesPerPath: %v", err)
	}

	if len(state.hardConflicts) != 0 {
		t.Errorf("expected no hard conflicts, got %v", state.hardConflicts)
	}
	if len(state.conflictedFiles) != 0 {
		t.Errorf("expected no conflicted files, got %d", len(state.conflictedFiles))
	}
	entry, ok := state.mergedEntries["new.txt"]
	if !ok {
		t.Fatal("new.txt missing from mergedEntries")
	}
	// Hash must match the blob that ours (and theirs, identically) wrote.
	oursTree, _ := ours.Tree()
	oursEntry, _ := oursTree.File("new.txt")
	if entry.hash != oursEntry.Blob.Hash {
		t.Errorf("mergedEntries[new.txt] hash mismatch: got %s, want %s", entry.hash, oursEntry.Blob.Hash)
	}
}

// TestApplyChangesPerPath_BothDeleted verifies that when both sides delete the
// same file the merged result omits the file (delete/delete fast-path in
// mergeBothModifiedPath) and no conflict is recorded.
func TestApplyChangesPerPath_BothDeleted(t *testing.T) {
	repo, repoDir := phaseInitRepo(t)
	ctx := context.Background()

	base := phaseCommitFiles(t, repo, repoDir, nil, map[string][]byte{
		"keeper.txt":  []byte("keep me\n"),
		"deleted.txt": []byte("delete me\n"),
	}, "base")
	// Both sides remove deleted.txt but keep keeper.txt.
	ours := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"keeper.txt": []byte("keep me\n"),
	}, "ours: delete deleted.txt")
	theirs := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"keeper.txt": []byte("keep me\n"),
	}, "theirs: delete deleted.txt")

	diff, err := computeMergeDiff(ctx, repo, base, ours, theirs)
	if err != nil {
		t.Fatalf("computeMergeDiff: %v", err)
	}
	state, err := applyChangesPerPath(repo, diff)
	if err != nil {
		t.Fatalf("applyChangesPerPath: %v", err)
	}

	if len(state.hardConflicts) != 0 {
		t.Errorf("expected no hard conflicts for both-delete, got %v", state.hardConflicts)
	}
	if len(state.conflictedFiles) != 0 {
		t.Errorf("expected no conflicted files, got %d", len(state.conflictedFiles))
	}
	if _, present := state.mergedEntries["deleted.txt"]; present {
		t.Error("deleted.txt should be absent from mergedEntries after both-delete")
	}
	// keeper.txt must still be present.
	if _, present := state.mergedEntries["keeper.txt"]; !present {
		t.Error("keeper.txt should remain in mergedEntries")
	}
}

// TestApplyChangesPerPath_BothModified_NonOverlappingChunks verifies that when
// both sides modify non-overlapping regions of the same file the three-way
// merge succeeds cleanly (no conflicts) and the merged blob contains both sets
// of changes.
func TestApplyChangesPerPath_BothModified_NonOverlappingChunks(t *testing.T) {
	repo, repoDir := phaseInitRepo(t)
	ctx := context.Background()

	// Base: a file with a header section and a footer section separated by an
	// untouched middle.  Ours edits only the header; theirs edits only the footer.
	baseContent := []byte("HEADER original\nmiddle unchanged\nFOOTER original\n")
	oursContent := []byte("HEADER changed by ours\nmiddle unchanged\nFOOTER original\n")
	theirsContent := []byte("HEADER original\nmiddle unchanged\nFOOTER changed by theirs\n")

	base := phaseCommitFiles(t, repo, repoDir, nil, map[string][]byte{
		"file.txt": baseContent,
	}, "base")
	ours := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"file.txt": oursContent,
	}, "ours: header change")
	theirs := phaseCommitFiles(t, repo, repoDir, base, map[string][]byte{
		"file.txt": theirsContent,
	}, "theirs: footer change")

	diff, err := computeMergeDiff(ctx, repo, base, ours, theirs)
	if err != nil {
		t.Fatalf("computeMergeDiff: %v", err)
	}
	state, err := applyChangesPerPath(repo, diff)
	if err != nil {
		t.Fatalf("applyChangesPerPath: %v", err)
	}

	if len(state.hardConflicts) != 0 {
		t.Errorf("expected no hard conflicts for non-overlapping changes, got %v", state.hardConflicts)
	}
	if len(state.conflictedFiles) != 0 {
		t.Errorf("expected no conflicted files for non-overlapping changes, got %d", len(state.conflictedFiles))
	}
	entry, ok := state.mergedEntries["file.txt"]
	if !ok {
		t.Fatal("file.txt missing from mergedEntries")
	}
	// Merged blob must contain both edits.
	mergedBytes, err := blobContent(repo, entry.hash)
	if err != nil {
		t.Fatalf("read merged blob: %v", err)
	}
	if !bytes.Contains(mergedBytes, []byte("HEADER changed by ours")) {
		t.Errorf("merged content missing ours' header change:\n%s", mergedBytes)
	}
	if !bytes.Contains(mergedBytes, []byte("FOOTER changed by theirs")) {
		t.Errorf("merged content missing theirs' footer change:\n%s", mergedBytes)
	}
}

// ---------------------------------------------------------------------------
// Phase 4: resolveConflicts
// ---------------------------------------------------------------------------

// TestResolveConflicts_WhitespaceOnly verifies that a conflict that is purely
// a whitespace difference is auto-resolved under the "whitespace" heuristic.
func TestResolveConflicts_WhitespaceOnly(t *testing.T) {
	repo, _ := phaseInitRepo(t)

	// Base, ours, theirs differ only in trailing whitespace — both sides add
	// trailing spaces to the same line. tryAutoResolve should classify as
	// "whitespace" and succeed.
	baseContent := []byte("line one\nline two\n")
	oursContent := []byte("line one   \nline two\n")   // trailing spaces on line 1
	theirsContent := []byte("line one \nline two\n")    // fewer trailing spaces on line 1

	// Write blobs.
	baseHash, err := writeBlob(repo, baseContent)
	if err != nil {
		t.Fatalf("writeBlob base: %v", err)
	}
	oursHash, err := writeBlob(repo, oursContent)
	if err != nil {
		t.Fatalf("writeBlob ours: %v", err)
	}
	theirsHash, err := writeBlob(repo, theirsContent)
	if err != nil {
		t.Fatalf("writeBlob theirs: %v", err)
	}

	mode := filemode.Regular

	state := &mergeState{
		mergedEntries: map[string]treeEntry{
			"file.txt": {hash: baseHash, mode: mode}, // placeholder
		},
		conflictedFiles: []conflictedFile{
			{
				path:   "file.txt",
				base:   baseContent,
				ours:   oursContent,
				theirs: theirsContent,
				mode:   mode,
				ranges: nil,
			},
		},
	}
	// Override mergedEntries with a conflict-marker placeholder to mimic what
	// applyChangesPerPath would have written. We can reuse oursHash as a stand-in.
	state.mergedEntries["file.txt"] = treeEntry{hash: oursHash, mode: mode}
	_ = theirsHash // suppress unused

	result, resolved, err := resolveConflicts(repo, state)
	if err != nil {
		t.Fatalf("resolveConflicts: %v", err)
	}
	if !resolved {
		t.Fatal("expected whitespace-only conflict to auto-resolve")
	}
	if result.Kind != SafeAutoResolve {
		t.Errorf("kind: got %v, want SafeAutoResolve", result.Kind)
	}
	if result.Heuristic != "whitespace" {
		t.Errorf("heuristic: got %q, want %q", result.Heuristic, "whitespace")
	}
	if result.MergedTreeSHA == "" {
		t.Error("MergedTreeSHA must be non-empty on SafeAutoResolve")
	}
}

// TestResolveConflicts_Unresolvable verifies that when a conflicted file cannot
// be auto-resolved resolveConflicts returns resolved=false and populates
// state.hardConflicts.
func TestResolveConflicts_Unresolvable(t *testing.T) {
	repo, _ := phaseInitRepo(t)

	// Genuine conflicting edits on the same line — not resolvable by any
	// safe heuristic.
	baseContent := []byte("line\n")
	oursContent := []byte("line changed by ours\n")
	theirsContent := []byte("line changed by theirs\n")

	baseHash, err := writeBlob(repo, baseContent)
	if err != nil {
		t.Fatalf("writeBlob base: %v", err)
	}
	oursHash, err := writeBlob(repo, oursContent)
	if err != nil {
		t.Fatalf("writeBlob ours: %v", err)
	}
	_ = oursHash

	mode := filemode.Regular

	conflictRanges := []LineRange{{Start: 1, End: 5}}
	state := &mergeState{
		mergedEntries: map[string]treeEntry{
			"file.txt": {hash: baseHash, mode: mode},
		},
		conflictedFiles: []conflictedFile{
			{
				path:   "file.txt",
				base:   baseContent,
				ours:   oursContent,
				theirs: theirsContent,
				mode:   mode,
				ranges: conflictRanges,
			},
		},
	}

	_, resolved, err := resolveConflicts(repo, state)
	if err != nil {
		t.Fatalf("resolveConflicts: %v", err)
	}
	if resolved {
		t.Fatal("expected unresolvable conflict to return resolved=false")
	}
	if len(state.hardConflicts) == 0 {
		t.Error("expected hard conflicts to be populated on failure")
	}
	if state.hardConflicts[0].File != "file.txt" {
		t.Errorf("hard conflict file: got %q, want %q", state.hardConflicts[0].File, "file.txt")
	}
}

// ---------------------------------------------------------------------------
// Internal test helpers (mirrors the external helpers in merge_test.go)
// ---------------------------------------------------------------------------

func phaseInitRepo(t *testing.T) (*gogit.Repository, string) {
	t.Helper()
	dir := t.TempDir()
	phaseRun(t, dir, "git", "init", "-b", "main")
	phaseRun(t, dir, "git", "config", "user.email", "test@jamsesh.test")
	phaseRun(t, dir, "git", "config", "user.name", "Test")
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	return repo, dir
}

func phaseCommitFiles(
	t *testing.T,
	repo *gogit.Repository,
	repoDir string,
	parent *object.Commit,
	files map[string][]byte,
	message string,
) *object.Commit {
	t.Helper()

	entries, err := os.ReadDir(repoDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		_ = os.RemoveAll(filepath.Join(repoDir, e.Name()))
	}

	for name, content := range files {
		fullPath := filepath.Join(repoDir, name)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(fullPath, content, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	phaseRun(t, repoDir, "git", "add", "-A")

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
			Author:            &sig,
			Committer:         &sig,
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
		t.Fatalf("CommitObject: %v", err)
	}
	return commit
}

func phaseRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run %v: %v\n%s", args, err, out)
	}
}
