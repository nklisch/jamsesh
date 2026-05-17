package finalizecmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// detectMidPick reports the SHA from CHERRY_PICK_HEAD if the cwd is
// in the middle of a cherry-pick (i.e. a previous run halted on a
// conflict and the user has not yet `--continue`d or `--abort`ed).
// Returns "" + nil error when there is no in-progress pick.
//
// Implementation note: `git rev-parse --git-path CHERRY_PICK_HEAD`
// prints the path to the file inside .git regardless of whether the
// file exists. We pair it with `--show-toplevel` so the returned
// path is anchored to the absolute repo root, not git's own cwd —
// this lets the os.Stat work even when the binary's working dir is
// not the same as git's (e.g. inside tests that pin git via env).
func detectMidPick(cwd string) (string, error) {
	// Resolve the absolute repo root first.
	top, err := runGitOutputCwd(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		// Not a git repo — treat as "not mid-pick"; pre-flight will
		// raise a clearer error if this is reached during the main flow.
		return "", nil
	}
	top = strings.TrimSpace(top)
	if top == "" {
		return "", nil
	}
	out, err := runGitOutputCwd(cwd, "rev-parse", "--git-path", "CHERRY_PICK_HEAD")
	if err != nil {
		return "", nil
	}
	path := strings.TrimSpace(out)
	if path == "" {
		return "", nil
	}
	// rev-parse returns a path relative to the repo root when called
	// from within a worktree; anchor it against the absolute toplevel
	// so os.Stat is independent of our process's cwd.
	if !filepath.IsAbs(path) {
		path = filepath.Join(top, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// reportMidPick prints the "you're mid-pick" message: the offending
// SHA, the remaining cherry-pick sequence (suffix of plan.SelectedCommits
// after the offending SHA), and the resume command. Plan may be nil
// when the plan fetch failed; in that case we still report the offending
// SHA + a generic resume hint.
func reportMidPick(w io.Writer, offendingSHA string, plan *Plan) {
	fmt.Fprintf(w, "Cherry-pick already in progress.\n")
	fmt.Fprintf(w, "  Offending commit: %s\n", offendingSHA)

	if plan != nil {
		remaining := remainingAfter(plan.SelectedCommits, offendingSHA)
		if len(remaining) > 0 {
			fmt.Fprintf(w, "  Remaining commits in this sequence:\n")
			for _, c := range remaining {
				fmt.Fprintf(w, "    %s  %s\n", c.SHA, c.Subject)
			}
		}
	}

	fmt.Fprintf(w, "\nResolve conflicts then run:\n")
	fmt.Fprintf(w, "  git cherry-pick --continue\n")
	fmt.Fprintf(w, "Or abort the entire sequence with:\n")
	fmt.Fprintf(w, "  git cherry-pick --abort\n")
	fmt.Fprintf(w, "\nRe-invoking `jamsesh finalize-run <plan-id>` after either action\n")
	fmt.Fprintf(w, "will re-report state without touching the working tree.\n")
}

// remainingAfter returns the suffix of commits strictly AFTER the
// offending SHA. If the SHA isn't found, the full list is returned —
// best-effort behaviour: the user still sees the sequence and the
// resume hint.
//
// Match is by prefix to accommodate the short-SHA forms git writes
// into CHERRY_PICK_HEAD versus the full-SHA forms in the plan.
func remainingAfter(commits []PlanCommit, offendingSHA string) []PlanCommit {
	offendingSHA = strings.TrimSpace(offendingSHA)
	if offendingSHA == "" {
		return nil
	}
	for i, c := range commits {
		if strings.HasPrefix(c.SHA, offendingSHA) || strings.HasPrefix(offendingSHA, c.SHA) {
			if i+1 >= len(commits) {
				return nil
			}
			return commits[i+1:]
		}
	}
	return nil
}
