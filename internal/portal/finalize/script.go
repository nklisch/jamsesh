// Package finalize: deterministic bash-script composition for finalize-run.
//
// Plan-generation reads the curated SHAs from the lock state and emits a
// bash script the human copy/pastes (or the plugin executes). The bytes
// are deterministic — same input → same output — so the portal UI and the
// plugin preview render identical strings.
//
// Three literal placeholders are left in the body unsubstituted; the
// finalize-run plugin replaces them at execution time:
//
//	$JAMSESH_FETCH_REMOTE   — local path or HTTPS URL the plugin picks
//	$JAMSESH_RUNNER_NAME    — git user.name of the local checkout
//	$JAMSESH_RUNNER_EMAIL   — git user.email of the local checkout
//
// Splitting placeholder substitution to the plugin means the portal-side
// script bytes are independent of the runner; the same plan text on a
// fresh fetch produces the same bytes.
package finalize

import (
	"fmt"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// autoMergerTrailer is the commit-message trailer the auto-merger sets on
// every merge commit it produces. Locked in PROTOCOL.md > Commit trailers
// and verified against internal/portal/automerger/outcomes.go (the
// trailer is literally "Auto-Merger: true").
const autoMergerTrailer = "Auto-Merger: true"

// ScriptInput is the deterministic input shape for BuildScript.
//
// All fields are required to be set by the caller. SquashMessageBody is
// ignored in preserve mode but the caller is expected to pass an empty
// string in that case for clarity.
type ScriptInput struct {
	Mode              string   // "squash" or "preserve"
	TargetBranch      string   // refs/heads/<TargetBranch> the script creates
	BaseSHA           string   // base commit the new branch starts from
	SelectedSHAs      []string // ordered list of curated SHAs
	SquashMessageBody string   // composed via RenderSquashMessageBody (squash only)
}

// BuildScript dispatches on Mode and returns the script body. Returns an
// empty string when Mode is unrecognised — the caller is expected to
// validate before reaching here.
func BuildScript(in ScriptInput) string {
	switch in.Mode {
	case "squash":
		return buildSquashScript(in)
	case "preserve":
		return buildPreserveScript(in)
	}
	return ""
}

// buildSquashScript composes the cherry-pick-and-squash script body.
//
// Layout:
//
//	#!/usr/bin/env bash
//	set -euo pipefail
//	echo "==> Fetching session refs"
//	git fetch "$JAMSESH_FETCH_REMOTE" <base-and-tip-refs>
//	echo "==> Creating target branch <target> at <base-short>"
//	git checkout -b "<target>" <base-sha>
//	echo "==> Staging <N> curated commits"
//	git cherry-pick --no-commit <sha1> <sha2> ... <shaN>
//	echo "==> Composing squash commit"
//	git commit --author="$JAMSESH_RUNNER_NAME <$JAMSESH_RUNNER_EMAIL>" -F - <<'JAMSESH_MSG'
//	<composed message body>
//	JAMSESH_MSG
//	echo "==> Done. Push when ready: git push origin <target>"
func buildSquashScript(in ScriptInput) string {
	var b strings.Builder
	writeScriptPrologue(&b)
	writeFetchStep(&b, in.BaseSHA, in.SelectedSHAs)
	writeCheckoutStep(&b, in.TargetBranch, in.BaseSHA)

	b.WriteString(fmt.Sprintf("echo \"==> Staging %d curated commit", len(in.SelectedSHAs)))
	if len(in.SelectedSHAs) != 1 {
		b.WriteString("s")
	}
	b.WriteString("\"\n")
	if len(in.SelectedSHAs) > 0 {
		b.WriteString("git cherry-pick --no-commit")
		for _, sha := range in.SelectedSHAs {
			b.WriteString(" ")
			b.WriteString(sha)
		}
		b.WriteString("\n")
	}

	b.WriteString("echo \"==> Composing squash commit\"\n")
	b.WriteString("git commit --author=\"$JAMSESH_RUNNER_NAME <$JAMSESH_RUNNER_EMAIL>\" -F - <<'JAMSESH_MSG'\n")
	b.WriteString(in.SquashMessageBody)
	if !strings.HasSuffix(in.SquashMessageBody, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("JAMSESH_MSG\n")

	b.WriteString(fmt.Sprintf("echo \"==> Done. Push when ready: git push origin %s\"\n", shellquote(in.TargetBranch)))
	return b.String()
}

// buildPreserveScript composes the preserve-all (cherry-pick-each) script.
//
// Layout:
//
//	#!/usr/bin/env bash
//	set -euo pipefail
//	echo "==> Fetching session refs"
//	git fetch "$JAMSESH_FETCH_REMOTE" <base-and-tip-refs>
//	echo "==> Creating target branch <target> at <base-short>"
//	git checkout -b "<target>" <base-sha>
//	echo "==> Cherry-picking commit 1 of N: <sha>"
//	git cherry-pick <sha>
//	(repeated per commit)
//	echo "==> Done. Push when ready: git push origin <target>"
//
// Each cherry-pick retains its own author + message. A conflict on any
// pick halts the script (set -e); the plugin's resume logic kicks in on
// re-invocation.
func buildPreserveScript(in ScriptInput) string {
	var b strings.Builder
	writeScriptPrologue(&b)
	writeFetchStep(&b, in.BaseSHA, in.SelectedSHAs)
	writeCheckoutStep(&b, in.TargetBranch, in.BaseSHA)

	total := len(in.SelectedSHAs)
	for i, sha := range in.SelectedSHAs {
		b.WriteString(fmt.Sprintf("echo \"==> Cherry-picking commit %d of %d: %s\"\n", i+1, total, sha))
		b.WriteString("git cherry-pick ")
		b.WriteString(sha)
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("echo \"==> Done. Push when ready: git push origin %s\"\n", shellquote(in.TargetBranch)))
	return b.String()
}

// writeScriptPrologue writes the shebang and `set -euo pipefail` line.
func writeScriptPrologue(b *strings.Builder) {
	b.WriteString("#!/usr/bin/env bash\n")
	b.WriteString("set -euo pipefail\n")
}

// writeFetchStep writes the verbose fetch step. The `$JAMSESH_FETCH_REMOTE`
// placeholder is substituted by the plugin (local path or HTTPS URL). The
// fetched refspecs are explicit SHAs — `git fetch <url>` with no refspec
// fetches the remote's HEAD, but session bare repos don't have HEAD
// pointing at a real ref (refs live under jam/<session>/...), so the
// default fetch errors with "couldn't find remote ref HEAD". Fetching by
// SHA bypasses HEAD entirely.
func writeFetchStep(b *strings.Builder, baseSHA string, shas []string) {
	b.WriteString("echo \"==> Fetching session refs\"\n")
	b.WriteString("git fetch \"$JAMSESH_FETCH_REMOTE\"")
	if baseSHA != "" {
		b.WriteString(" ")
		b.WriteString(baseSHA)
	}
	for _, sha := range shas {
		// Skip empty and duplicate-of-baseSHA to keep the fetch tidy.
		if sha == "" || sha == baseSHA {
			continue
		}
		b.WriteString(" ")
		b.WriteString(sha)
	}
	b.WriteString("\n")
}

// writeCheckoutStep writes the branch-creation step.
//
// Both targetBranch and baseSHA are shell-quoted via shellquote as
// defense-in-depth: even if an attacker-controlled value slips past the
// PatchFinalizeLock validator, the single-quote wrapping prevents bash from
// interpreting any special characters in the generated script.
func writeCheckoutStep(b *strings.Builder, targetBranch, baseSHA string) {
	short := baseSHA
	if len(short) > 12 {
		short = short[:12]
	}
	b.WriteString(fmt.Sprintf("echo \"==> Creating target branch %s at %s\"\n", targetBranch, short))
	b.WriteString(fmt.Sprintf("git checkout -b %s %s\n", shellquote(targetBranch), shellquote(baseSHA)))
}

// firstParentLeafCommits walks the first-parent chain from draftTipSHA and
// returns the underlying leaf agent commits in DAG-natural chronological
// order (oldest first). Auto-merger merge commits are NOT included — only
// the commits they integrated.
//
// Algorithm:
//
//  1. Walk first-parent from draftTipSHA, collecting commits in reverse
//     (so we can reverse at the end to get oldest-first).
//  2. When a commit has 2+ parents AND carries the autoMergerTrailer:
//     - Skip the merge commit itself.
//     - The first parent is the trunk side (continues the chain).
//     - The second parent is the side branch being integrated. Walk its
//       first-parent chain back to the merge-base of (firstParent,
//       secondParent). The commits along that walk (excluding the
//       merge-base) are the integrated leaves.
//     - Add the integrated leaves in chronological order (i.e. reversed
//       order of the walk, since the walk goes tip → base).
//
// Order semantics: example A → B(am-merge of X, Y) → C with C being the
// tip. The function returns [A, X, Y] — A is the oldest first-parent
// leaf, X and Y are the integrated leaves in chronological order.
func firstParentLeafCommits(repo *gogit.Repository, draftTipSHA string) ([]*object.Commit, error) {
	tip, err := repo.CommitObject(plumbing.NewHash(draftTipSHA))
	if err != nil {
		return nil, fmt.Errorf("finalize: resolve draft tip %s: %w", draftTipSHA, err)
	}

	// Collect everything in chronological order (oldest-first) by walking
	// tip → root and reversing at the end. For each auto-merger merge we
	// encounter, the integrated leaves are emitted in chronological order
	// at the point we cross that merge.
	//
	// Implementation note: we walk tip → root, accumulating into a stack
	// (newest-first). When we hit an auto-merger merge:
	//   - skip the merge commit (do NOT push it onto the stack)
	//   - push the integrated leaves onto the stack in reverse-
	//     chronological order so that when the stack is reversed at the
	//     end, they appear in chronological order at the right point.

	var stack []*object.Commit
	cur := tip
	for cur != nil {
		if cur.NumParents() >= 2 && isAutoMergerCommit(cur) {
			// Resolve parents.
			p1, err := cur.Parent(0)
			if err != nil {
				return nil, fmt.Errorf("finalize: resolve first parent of %s: %w", cur.Hash, err)
			}
			p2, err := cur.Parent(1)
			if err != nil {
				return nil, fmt.Errorf("finalize: resolve second parent of %s: %w", cur.Hash, err)
			}

			// Find the merge-base of p1 and p2. The integrated leaves are
			// the commits on p2's first-parent chain back to (but not
			// including) that merge-base.
			bases, err := p1.MergeBase(p2)
			if err != nil {
				return nil, fmt.Errorf("finalize: merge-base of %s..%s: %w", p1.Hash, p2.Hash, err)
			}
			baseHash := plumbing.ZeroHash
			if len(bases) > 0 {
				baseHash = bases[0].Hash
			}

			// Walk p2's first-parent chain until we hit baseHash.
			integratedTipFirst := []*object.Commit{}
			side := p2
			for side != nil && side.Hash != baseHash {
				integratedTipFirst = append(integratedTipFirst, side)
				if side.NumParents() == 0 {
					break
				}
				next, perr := side.Parent(0)
				if perr != nil {
					return nil, fmt.Errorf("finalize: walk side branch %s: %w", side.Hash, perr)
				}
				side = next
			}
			// integratedTipFirst is tip-first (newest-first); push onto
			// stack in the SAME order — when the outer stack is reversed at
			// the end, they end up oldest-first at the right insertion
			// point in the chronological output.
			stack = append(stack, integratedTipFirst...)

			// Continue down the first-parent chain.
			cur = p1
			continue
		}

		// Non-merge or non-auto-merger merge: include the commit.
		stack = append(stack, cur)

		if cur.NumParents() == 0 {
			break
		}
		parent, err := cur.Parent(0)
		if err != nil {
			return nil, fmt.Errorf("finalize: walk first parent of %s: %w", cur.Hash, err)
		}
		cur = parent
	}

	// Reverse the stack to produce chronological order (oldest first).
	out := make([]*object.Commit, len(stack))
	for i, c := range stack {
		out[len(stack)-1-i] = c
	}
	return out, nil
}

// isAutoMergerCommit returns true when the commit message carries the
// Auto-Merger trailer set by internal/portal/automerger.
func isAutoMergerCommit(c *object.Commit) bool {
	return strings.Contains(c.Message, autoMergerTrailer)
}
