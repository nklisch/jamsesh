package prereceive

import (
	"context"
	"fmt"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// requiredTrailers lists the trailer keys that every jamsesh commit must carry.
// These are enforced by pre-receive; see docs/PROTOCOL.md for the full schema.
var requiredTrailers = []string{"Jam-Session", "Jam-Turn", "Jam-Author"}

// WalkAndValidate visits every new commit in update (commits reachable from
// NewSHA but not from OldSHA) and applies per-commit policy checks:
//
//  1. Required trailers must be present and non-empty
//     (Jam-Session, Jam-Turn, Jam-Author).
//  2. Every changed path in the commit's diff vs its first parent must match
//     the provided scope matcher.
//
// If OldSHA is empty the ref is being created for the first time and ALL
// ancestors of NewSHA are visited.
//
// Returns all rejections found. An empty slice means every commit passed.
func WalkAndValidate(ctx context.Context, repo *git.Repository, update RefUpdate, scope *ScopeMatcher) []Rejection {
	newHash := plumbing.NewHash(update.NewSHA)

	// Resolve the stop point. If OldSHA is empty, we walk the entire ancestry
	// of NewSHA (root-commit case).
	var stopHash plumbing.Hash
	hasStop := update.OldSHA != ""
	if hasStop {
		stopHash = plumbing.NewHash(update.OldSHA)
	}

	// Collect the commit hashes we need to validate using repo.Log (depth-first
	// history traversal starting from newHash). We stop when we reach stopHash
	// (which is already in the repo and therefore already validated).
	var commits []*object.Commit

	iter, err := repo.Log(&git.LogOptions{From: newHash})
	if err != nil {
		return []Rejection{{
			Code:    CodeScopeViolation,
			Message: fmt.Sprintf("could not walk commits: %v", err),
			Details: map[string]any{"ref": update.Ref, "new_sha": update.NewSHA},
		}}
	}
	defer iter.Close()

	err = iter.ForEach(func(c *object.Commit) error {
		if hasStop && c.Hash == stopHash {
			return storer.ErrStop
		}
		commits = append(commits, c)
		return nil
	})
	if err != nil && err != storer.ErrStop {
		return []Rejection{{
			Code:    CodeScopeViolation,
			Message: fmt.Sprintf("commit iteration error: %v", err),
			Details: map[string]any{"ref": update.Ref},
		}}
	}

	var rejections []Rejection
	for _, c := range commits {
		rejections = append(rejections, validateCommit(ctx, repo, c, scope)...)
	}
	return rejections
}

// validateCommit runs all per-commit checks and returns any rejections.
func validateCommit(_ context.Context, repo *git.Repository, c *object.Commit, scope *ScopeMatcher) []Rejection {
	var rejections []Rejection

	// 1. Trailer check.
	if missing := CheckRequiredTrailers(c.Message, requiredTrailers); len(missing) > 0 {
		rejections = append(rejections, Rejection{
			Code:    CodeMissingTrailer,
			Message: fmt.Sprintf("commit %s is missing required trailers: %v", c.Hash.String()[:12], missing),
			Details: map[string]any{
				"commit":  c.Hash.String(),
				"missing": missing,
			},
		})
	}

	// 2. Scope check — skip if scope has no patterns (everything allowed).
	if scope != nil && len(scope.globs) > 0 {
		violations := scopeViolations(repo, c, scope)
		if len(violations) > 0 {
			rejections = append(rejections, Rejection{
				Code:    CodeScopeViolation,
				Message: fmt.Sprintf("commit %s modifies paths outside the writable scope: %v", c.Hash.String()[:12], violations),
				Details: map[string]any{
					"commit": c.Hash.String(),
					"paths":  violations,
				},
			})
		}
	}

	return rejections
}

// scopeViolations returns the list of changed paths in commit c that do NOT
// match the scope. It diffs the commit's tree against its first parent's tree,
// or against the empty tree for a root commit.
func scopeViolations(repo *git.Repository, c *object.Commit, scope *ScopeMatcher) []string {
	commitTree, err := c.Tree()
	if err != nil {
		// Can't get tree — treat as a violation for safety.
		return []string{"<error: could not read commit tree>"}
	}

	var parentTree *object.Tree
	if c.NumParents() > 0 {
		parent, err := c.Parents().Next()
		if err == nil {
			parentTree, err = parent.Tree()
			if err != nil {
				parentTree = nil
			}
		}
	}
	// parentTree == nil means empty tree (root commit or unresolvable parent).

	// Use DiffTreeWithOptions for rename-aware diffing (consistent with the
	// auto-merger; see go-git skill notes on DefaultDiffTreeOptions).
	ctx := context.Background()
	changes, err := object.DiffTreeWithOptions(ctx, parentTree, commitTree, object.DefaultDiffTreeOptions)
	if err != nil {
		return []string{"<error: could not diff trees>"}
	}

	var violations []string
	for _, ch := range changes {
		// A change may affect a "From" path (deletion/rename source) and/or a
		// "To" path (creation/rename target). We check both non-empty paths.
		from, to := ch.From.Name, ch.To.Name
		if from != "" && !scope.Match(from) {
			violations = append(violations, from)
		}
		if to != "" && to != from && !scope.Match(to) {
			violations = append(violations, to)
		}
	}
	return violations
}
