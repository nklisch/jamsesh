// Package automerger provides a pure three-way merge library for the
// jamsesh auto-merger.  Inputs are go-git Commit pointers; output is a
// classified [MergeResult].  No DB, no events, no ref updates are
// performed here — those belong to the outcomes/worker features.
package automerger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Merge runs a three-way merge between source and draftTip, using ancestor as
// the common base.  The caller is responsible for opening the repo and
// resolving the three commits (including the merge-base lookup).
//
// Perspective convention (matches git-merge-file argument order):
//
//	ours   = draftTip  (the live draft branch tip)
//	theirs = source    (the incoming commit)
//	base   = ancestor
//
// Short-circuit rules (per acceptance criteria):
//   - source == ancestor  → fast-forward to draftTip's tree.
//   - draftTip == ancestor → fast-forward to source's tree.
func Merge(
	ctx context.Context,
	repo *gogit.Repository,
	source, draftTip, ancestor *object.Commit,
) (MergeResult, error) {
	if result, done, err := tryShortCircuit(source, draftTip, ancestor); done {
		return result, err
	}

	diff, err := computeMergeDiff(ctx, repo, ancestor, draftTip, source)
	if err != nil {
		return MergeResult{}, err
	}

	state, err := applyChangesPerPath(repo, diff)
	if err != nil {
		return MergeResult{}, err
	}

	if len(state.conflictedFiles) > 0 && len(state.hardConflicts) == 0 {
		if result, resolved, err := resolveConflicts(repo, &state); resolved {
			return result, err
		}
	} else if len(state.conflictedFiles) > 0 {
		// hardConflicts already non-empty; add conflict entries for the
		// conflicted files too.
		for i := range state.conflictedFiles {
			cf := &state.conflictedFiles[i]
			state.hardConflicts = append(state.hardConflicts, Conflict{
				File:   cf.path,
				Ranges: cf.ranges,
			})
		}
	}

	if len(state.hardConflicts) > 0 {
		return MergeResult{Kind: HardConflict, Conflicts: state.hardConflicts}, nil
	}

	mergedTreeHash, err := buildTree(repo, state.mergedEntries)
	if err != nil {
		return MergeResult{}, fmt.Errorf("automerger: build merged tree: %w", err)
	}
	return MergeResult{Kind: CleanMerge, MergedTreeSHA: mergedTreeHash.String()}, nil
}

// tryShortCircuit checks whether the merge can be satisfied by a fast-forward.
//
// Inputs: the three commits (source = theirs, draftTip = ours, ancestor = base).
// Outputs: (result, true, nil) on a fast-forward hit; (zero, false, nil) when
// no short-circuit applies; (zero, true, err) on a tree-fetch error.
//
// Invariant: when done is true the caller must return (result, err) immediately
// without further processing.
func tryShortCircuit(
	source, draftTip, ancestor *object.Commit,
) (result MergeResult, done bool, err error) {
	// source is already in history → nothing new from source.
	if source.Hash == ancestor.Hash {
		t, err := draftTip.Tree()
		if err != nil {
			return MergeResult{}, true, fmt.Errorf("automerger: draftTip tree: %w", err)
		}
		return MergeResult{Kind: CleanMerge, MergedTreeSHA: t.Hash.String()}, true, nil
	}
	// draftTip is already in history → fast-forward to source.
	if draftTip.Hash == ancestor.Hash {
		t, err := source.Tree()
		if err != nil {
			return MergeResult{}, true, fmt.Errorf("automerger: source tree: %w", err)
		}
		return MergeResult{Kind: CleanMerge, MergedTreeSHA: t.Hash.String()}, true, nil
	}
	return MergeResult{}, false, nil
}

// mergeDiff holds the raw diff data produced by computeMergeDiff: the per-path
// change maps for both sides, the union of changed paths, and a mutable flat
// snapshot of the base tree used as the starting point for the merged result.
type mergeDiff struct {
	ourChanges   map[string]*sideChange
	theirChanges map[string]*sideChange
	pathSet      map[string]struct{}
	// mergedEntries is the flat base-tree snapshot. applyChangesPerPath modifies
	// it in place as each path is resolved, so it must not be shared.
	mergedEntries map[string]treeEntry
}

// computeMergeDiff fetches the base/ours/theirs trees, computes the two
// base-relative diffs, and flattens the base tree into a mutable snapshot.
//
// Inputs: ctx, repo, and the three commits in base/ours/theirs order.
// Outputs: a mergeDiff ready for applyChangesPerPath, or an error.
//
// Invariant: the returned mergeDiff.mergedEntries is a deep copy of the base
// tree; callers may mutate it freely.
func computeMergeDiff(
	ctx context.Context,
	repo *gogit.Repository,
	ancestor, draftTip, source *object.Commit,
) (mergeDiff, error) {
	baseTree, err := ancestor.Tree()
	if err != nil {
		return mergeDiff{}, fmt.Errorf("automerger: ancestor tree: %w", err)
	}
	oursTree, err := draftTip.Tree()
	if err != nil {
		return mergeDiff{}, fmt.Errorf("automerger: draftTip tree: %w", err)
	}
	theirsTree, err := source.Tree()
	if err != nil {
		return mergeDiff{}, fmt.Errorf("automerger: source tree: %w", err)
	}

	baseToOurs, err := object.DiffTreeWithOptions(ctx, baseTree, oursTree, object.DefaultDiffTreeOptions)
	if err != nil {
		return mergeDiff{}, fmt.Errorf("automerger: diff base→ours: %w", err)
	}
	baseToTheirs, err := object.DiffTreeWithOptions(ctx, baseTree, theirsTree, object.DefaultDiffTreeOptions)
	if err != nil {
		return mergeDiff{}, fmt.Errorf("automerger: diff base→theirs: %w", err)
	}

	ourChanges, err := buildSideChanges(baseToOurs, "ours")
	if err != nil {
		return mergeDiff{}, err
	}
	theirChanges, err := buildSideChanges(baseToTheirs, "theirs")
	if err != nil {
		return mergeDiff{}, err
	}

	// Build the union of changed paths.
	pathSet := make(map[string]struct{})
	for p := range ourChanges {
		pathSet[p] = struct{}{}
	}
	for p := range theirChanges {
		pathSet[p] = struct{}{}
	}

	// mergedEntries maps repo-relative path → blob hash (for the merged tree).
	// Start with a flat snapshot of baseTree.
	mergedEntries, err := flattenTree(baseTree)
	if err != nil {
		return mergeDiff{}, fmt.Errorf("automerger: flatten base tree: %w", err)
	}

	return mergeDiff{
		ourChanges:    ourChanges,
		theirChanges:  theirChanges,
		pathSet:       pathSet,
		mergedEntries: mergedEntries,
	}, nil
}

// conflictedFile holds the raw blob content for a file that git merge-file
// reported as conflicted, so we can attempt auto-resolution.
type conflictedFile struct {
	path   string
	base   []byte
	ours   []byte
	theirs []byte
	mode   filemode.FileMode
	ranges []LineRange
}

// mergeState holds the accumulated output of applyChangesPerPath: the
// (possibly partially-updated) flat tree, any hard conflicts detected during
// path-level merging, and the set of files that had content conflicts and
// need heuristic resolution.
type mergeState struct {
	mergedEntries  map[string]treeEntry
	hardConflicts  []Conflict
	conflictedFiles []conflictedFile
}

// applyChangesPerPath iterates over every path touched by either side and
// resolves it into diff.mergedEntries. Files with content conflicts are
// collected into the returned state.conflictedFiles; hard conflicts (e.g.
// delete-vs-modify) go into state.hardConflicts.
//
// Inputs: repo (for blob reads and writes) and the diff produced by
// computeMergeDiff.
// Outputs: a mergeState whose mergedEntries is diff.mergedEntries after
// applying all clean resolutions, plus the accumulated conflicts.
//
// Invariant: for every path in diff.pathSet, exactly one of the following
// holds on return: (a) mergedEntries[path] is set to the correct resolved
// blob, (b) the path was deleted and is absent from mergedEntries, (c) the
// path appears in hardConflicts, or (d) the path appears in conflictedFiles
// with a placeholder conflict-marker blob in mergedEntries[path].
func applyChangesPerPath(repo *gogit.Repository, diff mergeDiff) (mergeState, error) {
	state := mergeState{
		mergedEntries: diff.mergedEntries,
	}

	for path := range diff.pathSet {
		ourCh := diff.ourChanges[path]
		theirCh := diff.theirChanges[path]

		switch {
		case ourCh == nil && theirCh != nil:
			// Only theirs changed — accept theirs.
			if theirCh.deleted {
				delete(state.mergedEntries, path)
			} else {
				state.mergedEntries[path] = treeEntry{hash: theirCh.toHash, mode: theirCh.mode}
			}

		case ourCh != nil && theirCh == nil:
			// Only ours changed — accept ours.
			if ourCh.deleted {
				delete(state.mergedEntries, path)
			} else {
				state.mergedEntries[path] = treeEntry{hash: ourCh.toHash, mode: ourCh.mode}
			}

		case ourCh != nil && theirCh != nil:
			// Both sides changed this path — delegate to helper.
			if err := mergeBothModifiedPath(repo, &state, path, ourCh, theirCh); err != nil {
				return mergeState{}, err
			}
		}
	}

	return state, nil
}

// mergeBothModifiedPath handles a single path where both ours and theirs
// changed relative to the base. It mutates state.mergedEntries,
// state.hardConflicts, and state.conflictedFiles in place.
//
// Four sub-cases:
//  1. delete/delete → take delete.
//  2. delete/modify (or modify/delete) → hard conflict.
//  3. identical edits → take either.
//  4. different edits → three-way merge via runThreeWayMerge.
func mergeBothModifiedPath(repo *gogit.Repository, state *mergeState, path string, ourCh, theirCh *sideChange) error {
	// delete/delete → take delete.
	if ourCh.deleted && theirCh.deleted {
		delete(state.mergedEntries, path)
		return nil
	}
	// delete/modify (or modify/delete) → hard conflict.
	if ourCh.deleted || theirCh.deleted {
		state.hardConflicts = append(state.hardConflicts, Conflict{File: path})
		return nil
	}
	// Identical edits → take either.
	if ourCh.toHash == theirCh.toHash {
		state.mergedEntries[path] = treeEntry{hash: ourCh.toHash, mode: ourCh.mode}
		return nil
	}
	// Different edits → three-way content merge.
	return runThreeWayMerge(repo, state, path, ourCh, theirCh)
}

// runThreeWayMerge invokes git merge-file on the three blob versions for path,
// then either writes a clean merged blob into state.mergedEntries or records a
// placeholder conflict-marker blob and appends to state.conflictedFiles.
//
// Invariant: on return, state.mergedEntries[path] is always set — either to
// the cleanly merged blob hash or to the conflict-marker placeholder blob hash.
// state.conflictedFiles is only appended when numConflicts > 0.
func runThreeWayMerge(repo *gogit.Repository, state *mergeState, path string, ourCh, theirCh *sideChange) error {
	baseContent, err := blobContent(repo, state.mergedEntries[path].hash)
	if err != nil {
		return fmt.Errorf("automerger: read base blob %s: %w", path, err)
	}
	oursContent, err := blobContent(repo, ourCh.toHash)
	if err != nil {
		return fmt.Errorf("automerger: read ours blob %s: %w", path, err)
	}
	theirsContent, err := blobContent(repo, theirCh.toHash)
	if err != nil {
		return fmt.Errorf("automerger: read theirs blob %s: %w", path, err)
	}

	merged, numConflicts, err := mergeFileContent(baseContent, oursContent, theirsContent)
	if err != nil {
		return fmt.Errorf("automerger: merge-file %s: %w", path, err)
	}

	if numConflicts == 0 {
		// Write merged blob and record new hash.
		newHash, err := writeBlob(repo, merged)
		if err != nil {
			return fmt.Errorf("automerger: write merged blob %s: %w", path, err)
		}
		state.mergedEntries[path] = treeEntry{hash: newHash, mode: ourCh.mode}
		return nil
	}

	// Conflict — record for auto-resolution attempt; write placeholder blob.
	// The placeholder is overwritten in resolveConflicts if auto-resolution
	// succeeds; otherwise it remains as the canonical conflict-marker blob.
	ranges := ParseConflictRanges(merged)
	state.conflictedFiles = append(state.conflictedFiles, conflictedFile{
		path:   path,
		base:   baseContent,
		ours:   oursContent,
		theirs: theirsContent,
		mode:   ourCh.mode,
		ranges: ranges,
	})
	newHash, err := writeBlob(repo, merged)
	if err != nil {
		return fmt.Errorf("automerger: write conflict blob %s: %w", path, err)
	}
	state.mergedEntries[path] = treeEntry{hash: newHash, mode: ourCh.mode}
	return nil
}

// resolveConflicts attempts safe auto-resolution for all files in
// state.conflictedFiles. It is only called when len(state.conflictedFiles) > 0
// and len(state.hardConflicts) == 0.
//
// Inputs: repo (for blob writes) and a pointer to the mergeState so resolved
// entries can overwrite the placeholder blobs in state.mergedEntries.
// Outputs: (result, true, nil) when every conflicted file auto-resolves;
// (zero, false, nil) when any file cannot be auto-resolved (the caller
// inspects state.hardConflicts which resolveConflicts populates on failure).
//
// Invariant: on a true return, state.mergedEntries contains only clean
// resolved blobs (no conflict markers). On a false return, state.hardConflicts
// contains one entry per file that failed to auto-resolve.
func resolveConflicts(repo *gogit.Repository, state *mergeState) (MergeResult, bool, error) {
	worstPriority := -1
	worstHeuristic := ""
	allResolved := true

	for i := range state.conflictedFiles {
		cf := &state.conflictedFiles[i]
		resolved, heuristic, ok := tryAutoResolve(cf.base, cf.ours, cf.theirs)
		if !ok {
			allResolved = false
			state.hardConflicts = append(state.hardConflicts, Conflict{
				File:   cf.path,
				Ranges: cf.ranges,
			})
			continue
		}
		p := heuristicPriority(heuristic)
		if p > worstPriority {
			worstPriority = p
			worstHeuristic = heuristic
		}
		// Overwrite the placeholder blob with the cleanly resolved content.
		newHash, err := writeBlob(repo, resolved)
		if err != nil {
			return MergeResult{}, false, fmt.Errorf("automerger: write auto-resolved blob %s: %w", cf.path, err)
		}
		state.mergedEntries[cf.path] = treeEntry{hash: newHash, mode: cf.mode}
	}

	if !allResolved {
		return MergeResult{}, false, nil
	}

	mergedTreeHash, err := buildTree(repo, state.mergedEntries)
	if err != nil {
		return MergeResult{}, false, fmt.Errorf("automerger: build auto-resolved tree: %w", err)
	}
	return MergeResult{
		Kind:          SafeAutoResolve,
		MergedTreeSHA: mergedTreeHash.String(),
		Heuristic:     worstHeuristic,
	}, true, nil
}

// sideChange records what one side (ours or theirs) did to a path relative to
// the base tree.
type sideChange struct {
	fromHash plumbing.Hash
	toHash   plumbing.Hash
	fromPath string
	toPath   string
	mode     filemode.FileMode
	deleted  bool
	added    bool
}

// treeEntry holds the blob hash and mode for a single file in a flat tree
// snapshot.
type treeEntry struct {
	hash plumbing.Hash
	mode filemode.FileMode
}

// flattenTree returns a map of repo-relative path → treeEntry for every file
// reachable from root (recursing into sub-trees).
func flattenTree(root *object.Tree) (map[string]treeEntry, error) {
	result := make(map[string]treeEntry)
	err := flattenTreeInto(root, "", result)
	return result, err
}

func flattenTreeInto(tree *object.Tree, prefix string, out map[string]treeEntry) error {
	for _, entry := range tree.Entries {
		fullPath := entry.Name
		if prefix != "" {
			fullPath = prefix + "/" + entry.Name
		}
		if entry.Mode == filemode.Dir || entry.Mode == filemode.Submodule {
			subTree, err := tree.Tree(entry.Name)
			if err != nil {
				return fmt.Errorf("flattenTree sub-tree %s: %w", fullPath, err)
			}
			if err := flattenTreeInto(subTree, fullPath, out); err != nil {
				return err
			}
		} else {
			out[fullPath] = treeEntry{hash: entry.Hash, mode: entry.Mode}
		}
	}
	return nil
}

// buildTree constructs a nested tree object from a flat map of paths, writes
// every tree object to the repo's ObjectStorer, and returns the root tree hash.
func buildTree(repo *gogit.Repository, entries map[string]treeEntry) (plumbing.Hash, error) {
	// Group entries by their immediate parent directory.
	// We need to build trees bottom-up.

	// Collect all directory prefixes needed.
	type dirEntry struct {
		name string
		hash plumbing.Hash
		mode filemode.FileMode
	}

	// Build a map: directory path → list of child entries (files + sub-dirs).
	// "" is the root.
	dirChildren := make(map[string][]dirEntry)

	// Seed with file entries.
	for path, entry := range entries {
		dir := filepath.Dir(path)
		if dir == "." {
			dir = ""
		}
		name := filepath.Base(path)
		dirChildren[dir] = append(dirChildren[dir], dirEntry{
			name: name,
			hash: entry.hash,
			mode: entry.mode,
		})
	}

	// Walk directories from deepest to shallowest, building tree objects.
	// Collect unique dirs first.
	allDirs := make(map[string]struct{})
	allDirs[""] = struct{}{}
	for path := range entries {
		parts := strings.Split(path, "/")
		for i := 1; i < len(parts); i++ {
			allDirs[strings.Join(parts[:i], "/")] = struct{}{}
		}
	}

	// Sort dirs by depth descending so children are processed before parents.
	sortedDirs := make([]string, 0, len(allDirs))
	for d := range allDirs {
		sortedDirs = append(sortedDirs, d)
	}
	sort.Slice(sortedDirs, func(i, j int) bool {
		di := strings.Count(sortedDirs[i], "/")
		dj := strings.Count(sortedDirs[j], "/")
		if di != dj {
			return di > dj // deeper first
		}
		return sortedDirs[i] > sortedDirs[j]
	})

	dirHash := make(map[string]plumbing.Hash)

	for _, dir := range sortedDirs {
		children := dirChildren[dir]
		// Sort children for deterministic tree objects.
		sort.Slice(children, func(i, j int) bool {
			return children[i].name < children[j].name
		})

		treeObj := &object.Tree{}
		for _, ch := range children {
			treeObj.Entries = append(treeObj.Entries, object.TreeEntry{
				Name: ch.name,
				Hash: ch.hash,
				Mode: ch.mode,
			})
		}

		encoded := repo.Storer.NewEncodedObject()
		if err := treeObj.Encode(encoded); err != nil {
			return plumbing.ZeroHash, fmt.Errorf("buildTree encode %q: %w", dir, err)
		}
		h, err := repo.Storer.SetEncodedObject(encoded)
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("buildTree store %q: %w", dir, err)
		}
		dirHash[dir] = h

		// Register this sub-tree as a child entry in its own parent.
		if dir != "" {
			parent := filepath.Dir(dir)
			if parent == "." {
				parent = ""
			}
			name := filepath.Base(dir)
			dirChildren[parent] = append(dirChildren[parent], dirEntry{
				name: name,
				hash: h,
				mode: filemode.Dir,
			})
		}
	}

	return dirHash[""], nil
}

// blobContent reads the full content of a blob identified by hash.
func blobContent(repo *gogit.Repository, hash plumbing.Hash) ([]byte, error) {
	if hash == plumbing.ZeroHash {
		return nil, nil
	}
	blob, err := repo.BlobObject(hash)
	if err != nil {
		return nil, err
	}
	r, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// writeBlob encodes content as a git blob object, writes it to the repo's
// ObjectStorer, and returns the resulting hash.
func writeBlob(repo *gogit.Repository, content []byte) (plumbing.Hash, error) {
	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	obj.SetSize(int64(len(content)))
	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := w.Write(content); err != nil {
		_ = w.Close()
		return plumbing.ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, err
	}
	return repo.Storer.SetEncodedObject(obj)
}

// mergeFileContent invokes `git merge-file --stdout` on three byte slices
// written to temporary files. Returns the merged content and the number of
// conflict hunks (0 means clean merge). Any OS/exec error is returned as a
// non-nil error.
func mergeFileContent(base, ours, theirs []byte) (merged []byte, numConflicts int, err error) {
	dir, err := os.MkdirTemp("", "jamsesh-merge-*")
	if err != nil {
		return nil, -1, fmt.Errorf("mergeFileContent mktemp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	oursFile := filepath.Join(dir, "ours")
	baseFile := filepath.Join(dir, "base")
	theirsFile := filepath.Join(dir, "theirs")

	if err := os.WriteFile(oursFile, ours, 0o600); err != nil {
		return nil, -1, fmt.Errorf("mergeFileContent write ours: %w", err)
	}
	if err := os.WriteFile(baseFile, base, 0o600); err != nil {
		return nil, -1, fmt.Errorf("mergeFileContent write base: %w", err)
	}
	if err := os.WriteFile(theirsFile, theirs, 0o600); err != nil {
		return nil, -1, fmt.Errorf("mergeFileContent write theirs: %w", err)
	}

	// git merge-file --stdout -L ours -L base -L theirs ours base theirs
	// modifies `ours` in place; --stdout redirects to stdout instead.
	cmd := exec.Command("git", "merge-file",
		"--stdout",
		"-L", "ours", "-L", "base", "-L", "theirs",
		oursFile, baseFile, theirsFile,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard

	runErr := cmd.Run()
	if runErr == nil {
		return out.Bytes(), 0, nil
	}
	if ee, ok := runErr.(*exec.ExitError); ok && ee.ExitCode() > 0 {
		// ExitCode == number of conflict hunks.
		return out.Bytes(), ee.ExitCode(), nil
	}
	return nil, -1, fmt.Errorf("mergeFileContent git merge-file: %w", runErr)
}

// effectivePath returns the canonical repo-relative path for a change,
// preferring the "to" path (post-rename destination) over the "from" path.
func effectivePath(sc *sideChange) string {
	if sc.toPath != "" {
		return sc.toPath
	}
	return sc.fromPath
}

// buildSideChanges walks an object.Changes list and builds a map from
// effectivePath to *sideChange.  side ("ours" or "theirs") is used only in
// error messages so callers receive the same strings as the inlined loops did.
func buildSideChanges(changes object.Changes, side string) (map[string]*sideChange, error) {
	out := make(map[string]*sideChange)
	for _, ch := range changes {
		from, to, err := ch.Files()
		if err != nil {
			return nil, fmt.Errorf("automerger: %s change files: %w", side, err)
		}
		sc := &sideChange{}
		if from != nil {
			sc.fromHash = from.Blob.Hash
			sc.fromPath = from.Name
			sc.mode = from.Mode
		}
		if to != nil {
			sc.toHash = to.Blob.Hash
			sc.toPath = to.Name
			sc.mode = to.Mode
		}
		sc.deleted = (to == nil)
		sc.added = (from == nil)
		out[effectivePath(sc)] = sc
	}
	return out, nil
}
