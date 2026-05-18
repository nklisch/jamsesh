package automerger

import (
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// computeAddressedTo returns the set of addressing tokens for a conflict
// event. The set includes:
//
//  1. The source ref's owner: "@<user>/<branch>" extracted from
//     "refs/heads/jam/<sessionID>/<user>/<branch>".
//  2. The author email of the most-recent draft commit (within the last 100)
//     that last modified each conflicted file.
//
// v1 limitation: the walk is bounded to 100 commits from draftTip. A conflict
// against a change made more than 100 commits ago will not include the
// original author in addressed_to.
func computeAddressedTo(repo *gogit.Repository, draftTip plumbing.Hash, conflicts []Conflict, sourceRef string) ([]string, error) {
	set := make(map[string]bool)

	// 1. Source-ref owner.
	if owner := parseSourceRefOwner(sourceRef); owner != "" {
		set[owner] = true
	}

	if len(conflicts) == 0 {
		return sortedKeys(set), nil
	}

	// 2. Walk draft history to find the last modifier of each conflicted file.
	needFiles := make(map[string]bool, len(conflicts))
	for _, c := range conflicts {
		needFiles[c.File] = true
	}

	iter, err := repo.Log(&gogit.LogOptions{From: draftTip})
	if err != nil {
		return sortedKeys(set), err
	}

	count := 0
	err = iter.ForEach(func(c *object.Commit) error {
		if count >= 100 || len(needFiles) == 0 {
			return storer.ErrStop
		}
		count++

		// Diff this commit vs its first parent to find changed files.
		var parentTree *object.Tree
		if len(c.ParentHashes) > 0 {
			parent, err := c.Parents().Next()
			if err != nil {
				return nil // skip on error; don't abort walk
			}
			parentTree, err = parent.Tree()
			if err != nil {
				return nil
			}
		}

		commitTree, err := c.Tree()
		if err != nil {
			return nil
		}

		changes, err := object.DiffTree(parentTree, commitTree)
		if err != nil {
			return nil
		}

		for _, ch := range changes {
			from, to, err := ch.Files()
			if err != nil {
				continue
			}
			var path string
			if to != nil {
				path = to.Name
			} else if from != nil {
				path = from.Name
			}
			if needFiles[path] {
				set[c.Author.Email] = true
				delete(needFiles, path) // first (most-recent) modifier wins
			}
		}

		return nil
	})
	if err != nil && err != storer.ErrStop {
		return sortedKeys(set), err
	}

	return sortedKeys(set), nil
}

// parseSourceRefOwner extracts the "@<user>/<branch>" token from a jamsesh
// source ref of the form "refs/heads/jam/<sessionID>/<user>/<branch>".
// Returns "" if the ref does not match the expected format.
func parseSourceRefOwner(sourceRef string) string {
	// Strip the "refs/heads/" prefix.
	const prefix = "refs/heads/"
	rel := strings.TrimPrefix(sourceRef, prefix)

	// Expect "jam/<sessionID>/<user>/<branch...>"
	parts := strings.SplitN(rel, "/", 4)
	if len(parts) < 4 || parts[0] != "jam" {
		return ""
	}
	// parts[0] = "jam", parts[1] = sessionID, parts[2] = user, parts[3] = branch (may contain /)
	return "@" + parts[2] + "/" + parts[3]
}

// sortedKeys returns the keys of a bool map sorted alphabetically.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
