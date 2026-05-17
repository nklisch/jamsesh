package finalizecmd

import (
	"fmt"
	"io"
	"strings"
)

// conflictError is returned by execute when a cherry-pick step fails
// with what looks like a merge conflict (i.e. CHERRY_PICK_HEAD ends
// up present afterwards). The caller renders the resume hint and
// exits non-zero with the message body intact.
type conflictError struct {
	// OffendingSHA is the commit git was trying to apply when it
	// halted. Read from CHERRY_PICK_HEAD post-failure.
	OffendingSHA string

	// Remaining is the suffix of cherry-pick SHAs that have not been
	// attempted yet. Pre-computed at conflict time so the caller does
	// not need to re-walk the plan.
	Remaining []PlanCommit

	// Underlying is the wrapped exec error, preserved so callers can
	// surface its exit code.
	Underlying error
}

func (e *conflictError) Error() string {
	return fmt.Sprintf("cherry-pick conflict at %s: %v", e.OffendingSHA, e.Underlying)
}

func (e *conflictError) Unwrap() error { return e.Underlying }

// runnerIdentity is the git user.name/user.email pair we feed to
// `git commit --author=` for the squash commit. Resolved from the
// local repo's git config — same source git would use without an
// explicit --author.
type runnerIdentity struct {
	Name  string
	Email string
}

// resolveRunnerIdentity pulls user.name + user.email from `git config`.
// Both fields must be non-empty for the squash mode to compose the
// --author argument; the caller should bail with a friendly hint if
// either is missing (mirroring git's own "Please tell me who you are"
// error path).
func resolveRunnerIdentity() (runnerIdentity, error) {
	name, err := runGitOutput("config", "user.name")
	if err != nil || strings.TrimSpace(name) == "" {
		return runnerIdentity{}, fmt.Errorf("git user.name is not set; run `git config user.name 'Your Name'`")
	}
	email, err := runGitOutput("config", "user.email")
	if err != nil || strings.TrimSpace(email) == "" {
		return runnerIdentity{}, fmt.Errorf("git user.email is not set; run `git config user.email you@example.com`")
	}
	return runnerIdentity{Name: strings.TrimSpace(name), Email: strings.TrimSpace(email)}, nil
}

// execute runs the mode-appropriate cherry-pick sequence. Each git
// invocation is preceded by a `+ git <args>` line on out (sh -x style)
// so the user can follow along in real time.
//
// On a cherry-pick conflict (detected by CHERRY_PICK_HEAD being present
// after a non-zero exit), execute returns a *conflictError carrying
// the offending SHA + the unattempted suffix. The caller renders the
// resume hint; the partial state stays in the user's working tree so
// they can fix it with their own Claude Code.
func execute(out io.Writer, plan *Plan, runner runnerIdentity) error {
	shas := plan.selectedSHAs()
	if len(shas) == 0 {
		return fmt.Errorf("plan has no commits to cherry-pick")
	}

	// Step A: branch creation.
	if err := runGitVerbose(out, "checkout", "-b", plan.TargetBranch, plan.BaseSHA); err != nil {
		return fmt.Errorf("creating branch %q from %s: %w", plan.TargetBranch, plan.BaseSHA, err)
	}

	switch plan.Mode {
	case "squash":
		// One cherry-pick command across every SHA, no commit. Composed
		// commit follows once the tree is staged.
		args := append([]string{"cherry-pick", "--no-commit"}, shas...)
		if err := runGitVerbose(out, args...); err != nil {
			return classifyCherryPickError(plan, shas, err)
		}
		// Composed commit message via stdin → no shell-quoting risk.
		commitArgs := []string{
			"commit",
			"--author=" + fmt.Sprintf("%s <%s>", runner.Name, runner.Email),
			"-F", "-",
		}
		fmt.Fprintf(out, "+ git %s   # message via stdin\n", strings.Join(commitArgs, " "))
		if err := runGitWithStdin(plan.CommitMessage, commitArgs...); err != nil {
			return fmt.Errorf("git commit failed: %w", err)
		}
	case "preserve":
		// One cherry-pick command across every SHA — preserves N commits
		// with their original authorship/timestamps.
		args := append([]string{"cherry-pick"}, shas...)
		if err := runGitVerbose(out, args...); err != nil {
			return classifyCherryPickError(plan, shas, err)
		}
	default:
		return fmt.Errorf("unknown plan mode %q (expected squash or preserve)", plan.Mode)
	}

	return nil
}

// runGitVerbose prints `+ git <args>` to out, flushes, then invokes
// runGit. The flush is best-effort: if out is *os.File we Sync; if it's
// a *bufio.Writer the caller is responsible for flushing externally.
func runGitVerbose(out io.Writer, args ...string) error {
	fmt.Fprintf(out, "+ git %s\n", strings.Join(args, " "))
	if f, ok := out.(interface{ Sync() error }); ok {
		_ = f.Sync()
	}
	return runGit(args...)
}

// classifyCherryPickError inspects the repo post-failure to decide
// whether the non-zero exit was a true conflict (CHERRY_PICK_HEAD
// present) or some other failure (wrong SHA, etc.). Conflict paths
// return *conflictError so the caller can render the resume hint.
func classifyCherryPickError(plan *Plan, shas []string, gitErr error) error {
	offending, derr := detectMidPick(".")
	if derr != nil || offending == "" {
		// Not a conflict — something else went wrong.
		return fmt.Errorf("cherry-pick failed: %w", gitErr)
	}
	return &conflictError{
		OffendingSHA: offending,
		Remaining:    remainingAfter(plan.SelectedCommits, offending),
		Underlying:   gitErr,
	}
}

// renderConflict writes the user-facing resume message for a
// conflictError. Centralised so finalize-run can call it without
// importing fmt formatters into its action body.
func renderConflict(out io.Writer, e *conflictError) {
	fmt.Fprintf(out, "\nCherry-pick failed at %s.\n", e.OffendingSHA)
	if len(e.Remaining) > 0 {
		fmt.Fprintf(out, "Remaining commits in this sequence:\n")
		for _, c := range e.Remaining {
			fmt.Fprintf(out, "  %s  %s\n", c.SHA, c.Subject)
		}
	} else {
		fmt.Fprintf(out, "No commits remain after this one.\n")
	}
	fmt.Fprintf(out, "\nResolve conflicts then run:\n")
	fmt.Fprintf(out, "  git cherry-pick --continue\n")
	fmt.Fprintf(out, "Or abort the entire sequence with:\n")
	fmt.Fprintf(out, "  git cherry-pick --abort\n")
}
