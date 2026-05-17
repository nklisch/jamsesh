package prereceive

import (
	"context"
	"fmt"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// ValidateRef checks ref namespace and force-push semantics for a single ref
// update. It enforces the following rules:
//
//  1. The ref must sit in the authenticated user's namespace:
//     refs/heads/jam/<sessionID>/<accountKey>/<branch>
//
//  2. Exception: refs/heads/jam/<sessionID>/base is permitted ONLY when the
//     repository has no refs yet (the session creator's first push). Any push
//     to that ref once refs exist is rejected as a namespace violation.
//
//  3. refs/heads/jam/<sessionID>/draft (and any other "server-managed" refs
//     that do not contain a further slash) are always rejected.
//
//  4. Force-push detection: if OldSHA is non-empty, the old commit must be
//     an ancestor of the new commit. If it is not, the update is a force-push
//     and is rejected with push.force_push_rejected.
//
// accountKey is account.ID (ULID). sessionID is session.ID.
func ValidateRef(ctx context.Context, repo *git.Repository, sessionID, accountKey string, update RefUpdate) []Rejection {
	var rejections []Rejection

	// ---- 1. Namespace check ----
	nsOK, isBase := checkRefNamespace(ctx, repo, sessionID, accountKey, update.Ref)
	if !nsOK {
		rejections = append(rejections, Rejection{
			Code:    CodeRefNamespaceViolation,
			Message: fmt.Sprintf("ref %q is outside your writable namespace (expected refs/heads/jam/%s/%s/<branch>)", update.Ref, sessionID, accountKey),
			Details: map[string]any{
				"ref":        update.Ref,
				"session_id": sessionID,
				"account_id": accountKey,
			},
		})
		// No point checking force-push when the ref itself is not allowed.
		return rejections
	}

	// ---- 2. Force-push check ----
	// Skip for new refs (OldSHA empty) — those are always fast-forward by
	// definition. Also skip for base refs on first push (isBase && repo empty
	// path already validated in checkRefNamespace).
	if update.OldSHA != "" {
		if r, ok := checkForcePush(repo, update, isBase); !ok {
			rejections = append(rejections, r)
		}
	}

	return rejections
}

// checkRefNamespace validates the ref name against the jamsesh namespace rules.
// Returns (allowed bool, isBase bool). isBase is true for the special
// refs/heads/jam/<session>/base ref.
func checkRefNamespace(_ context.Context, repo *git.Repository, sessionID, accountKey, ref string) (allowed bool, isBase bool) {
	const prefix = "refs/heads/jam/"
	if !strings.HasPrefix(ref, prefix) {
		return false, false
	}
	rest := ref[len(prefix):]

	// rest should be "<sessionID>/<owner>[/<branch>]"
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		// Malformed: too few segments.
		return false, false
	}

	refSession := parts[0]
	ownerOrSpecial := parts[1]

	// The sessionID in the ref must match the one we're validating for.
	if refSession != sessionID {
		return false, false
	}

	// Special case: refs/heads/jam/<session>/base (exactly two segments after
	// the prefix, ownerOrSpecial == "base", no further slash).
	if ownerOrSpecial == "base" && len(parts) == 2 {
		// Allowed only when the repository is empty (no existing refs).
		empty, err := repoIsEmpty(repo)
		if err != nil || !empty {
			return false, true // isBase=true so caller can format a meaningful error
		}
		return true, true
	}

	// Server-managed refs (e.g. "draft") — two-segment refs that are not
	// "base" are always rejected.
	if len(parts) == 2 {
		return false, false
	}

	// Normal user ref: refs/heads/jam/<session>/<accountKey>/<branch>
	// parts[2] is the branch name (may contain slashes if further split, but
	// SplitN(3) keeps the tail intact).
	if ownerOrSpecial != accountKey {
		return false, false
	}

	// Branch segment must be non-empty.
	if parts[2] == "" {
		return false, false
	}

	return true, false
}

// repoIsEmpty reports whether the repository has no refs under refs/ (i.e. no
// branches, tags, or other refs). The HEAD symbolic ref is always present in a
// fresh git repo and is NOT counted. An error from IterReferences is treated
// as non-empty (safe default).
func repoIsEmpty(repo *git.Repository) (bool, error) {
	iter, err := repo.References()
	if err != nil {
		return false, err
	}
	defer iter.Close()

	found := false
	err = iter.ForEach(func(r *plumbing.Reference) error {
		// Skip symbolic references (e.g. HEAD). We only care about concrete
		// refs that represent stored commits.
		if r.Type() == plumbing.SymbolicReference {
			return nil
		}
		found = true
		return storer.ErrStop // stop on first real ref
	})
	if err != nil && err != storer.ErrStop {
		return false, err
	}
	return !found, nil
}

// checkForcePush verifies that OldSHA is an ancestor of NewSHA. If it is not,
// the update is a force-push and the function returns (Rejection, false).
// On success it returns (Rejection{}, true).
//
// isBase indicates the ref is refs/heads/jam/<session>/base — force-pushing
// shared refs is unconditionally rejected (same code, extra context).
func checkForcePush(repo *git.Repository, update RefUpdate, isBase bool) (Rejection, bool) {
	oldHash := plumbing.NewHash(update.OldSHA)
	newHash := plumbing.NewHash(update.NewSHA)

	oldCommit, err := repo.CommitObject(oldHash)
	if err != nil {
		// Cannot resolve old commit — conservatively reject.
		return Rejection{
			Code:    CodeForcePushRejected,
			Message: fmt.Sprintf("ref %q: could not resolve old commit %s: %v", update.Ref, update.OldSHA[:12], err),
			Details: map[string]any{"ref": update.Ref, "old_sha": update.OldSHA},
		}, false
	}

	newCommit, err := repo.CommitObject(newHash)
	if err != nil {
		return Rejection{
			Code:    CodeForcePushRejected,
			Message: fmt.Sprintf("ref %q: could not resolve new commit %s: %v", update.Ref, update.NewSHA[:12], err),
			Details: map[string]any{"ref": update.Ref, "new_sha": update.NewSHA},
		}, false
	}

	// IsAncestor checks: is oldCommit reachable from newCommit's history?
	isAncestor, err := oldCommit.IsAncestor(newCommit)
	if err != nil {
		return Rejection{
			Code:    CodeForcePushRejected,
			Message: fmt.Sprintf("ref %q: ancestry check failed: %v", update.Ref, err),
			Details: map[string]any{"ref": update.Ref, "old_sha": update.OldSHA, "new_sha": update.NewSHA},
		}, false
	}

	if !isAncestor {
		msg := fmt.Sprintf("force-push to ref %q rejected: %s is not an ancestor of %s", update.Ref, update.OldSHA[:12], update.NewSHA[:12])
		if isBase {
			msg = fmt.Sprintf("force-push to shared base ref %q is not allowed", update.Ref)
		}
		return Rejection{
			Code:    CodeForcePushRejected,
			Message: msg,
			Details: map[string]any{
				"ref":     update.Ref,
				"old_sha": update.OldSHA,
				"new_sha": update.NewSHA,
			},
		}, false
	}

	return Rejection{}, true
}
