package finalizecmd

import (
	"fmt"
	"io"
	"strings"
)

// printScript renders the mode-appropriate bash script the binary
// would execute, as a single human-readable block. Used by `finalize
// --local` and `finalize-run --print-script`.
//
// We compose the script locally rather than echoing plan.Script for
// two reasons:
//   1. We control the heredoc terminator + the runner placeholders so
//      the user sees exactly what the binary will run, not the portal's
//      generic template.
//   2. `--print-script` works even if the server omitted the script
//      field (older portal versions, or a stub-mode plan).
func printScript(w io.Writer, plan *Plan) error {
	fmt.Fprintf(w, "#!/usr/bin/env bash\n")
	fmt.Fprintf(w, "set -euo pipefail\n\n")
	fmt.Fprintf(w, "# jamsesh finalize plan %s\n", plan.PlanID)
	fmt.Fprintf(w, "# mode: %s | target: %s | base: %s\n\n", plan.Mode, plan.TargetBranch, plan.BaseSHA)

	fmt.Fprintf(w, "echo '==> Creating branch %s from %s'\n", plan.TargetBranch, plan.BaseSHA)
	fmt.Fprintf(w, "git checkout -b %s %s\n\n", plan.TargetBranch, plan.BaseSHA)

	switch plan.Mode {
	case "squash":
		if err := printSquashBody(w, plan); err != nil {
			return err
		}
	case "preserve":
		printPreserveBody(w, plan)
	default:
		return fmt.Errorf("unknown plan mode %q", plan.Mode)
	}
	return nil
}

func printSquashBody(w io.Writer, plan *Plan) error {
	fmt.Fprintf(w, "echo '==> Cherry-picking %d commits without committing'\n", len(plan.SelectedCommits))
	shas := plan.selectedSHAs()
	fmt.Fprintf(w, "git cherry-pick --no-commit %s\n\n", strings.Join(shas, " "))

	terminator, err := pickHeredocTerminator(plan.CommitMessage)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "echo '==> Composing squash commit'\n")
	fmt.Fprintf(w, "git commit --author=\"${JAMSESH_RUNNER_NAME} <${JAMSESH_RUNNER_EMAIL}>\" -F - <<'%s'\n", terminator)
	fmt.Fprintf(w, "%s", plan.CommitMessage)
	if !strings.HasSuffix(plan.CommitMessage, "\n") {
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "%s\n", terminator)
	return nil
}

func printPreserveBody(w io.Writer, plan *Plan) {
	fmt.Fprintf(w, "echo '==> Cherry-picking %d commits (preserve authorship)'\n", len(plan.SelectedCommits))
	for _, c := range plan.SelectedCommits {
		fmt.Fprintf(w, "git cherry-pick %s  # %s\n", c.SHA, c.Subject)
	}
}

// pickHeredocTerminator selects a heredoc sentinel that does NOT
// appear as a standalone line inside body. The default is JAMSESH_EOF;
// if that string is already present (extremely unlikely but worth
// guarding) we append digits until we find a unique one. Returns
// an error if we somehow can't find one in 1000 tries (which would
// require a 6 KB message of crafted terminators).
func pickHeredocTerminator(body string) (string, error) {
	base := "JAMSESH_EOF"
	if !containsStandaloneLine(body, base) {
		return base, nil
	}
	for i := 0; i < 1000; i++ {
		cand := fmt.Sprintf("%s_%d", base, i)
		if !containsStandaloneLine(body, cand) {
			return cand, nil
		}
	}
	return "", fmt.Errorf("could not find a unique heredoc terminator")
}

// containsStandaloneLine reports whether sentinel appears as its own
// complete line anywhere in body (with or without trailing newline).
func containsStandaloneLine(body, sentinel string) bool {
	for _, line := range strings.Split(body, "\n") {
		if line == sentinel {
			return true
		}
	}
	return false
}

// printPlanSummary prints a human-readable summary header used by
// finalize-run before the proceed prompt and by `finalize --local`
// alongside the script body.
func printPlanSummary(w io.Writer, plan *Plan) {
	fmt.Fprintf(w, "Finalize plan %s\n", plan.PlanID)
	fmt.Fprintf(w, "  Mode:          %s\n", plan.Mode)
	fmt.Fprintf(w, "  Target branch: %s\n", plan.TargetBranch)
	fmt.Fprintf(w, "  Base SHA:      %s\n", plan.BaseSHA)
	fmt.Fprintf(w, "  Commits:       %d\n", len(plan.SelectedCommits))
	for _, c := range plan.SelectedCommits {
		fmt.Fprintf(w, "    %s  %s  (%s)\n", short(c.SHA), c.Subject, c.AuthorName)
	}
	if plan.Mode == "squash" && len(plan.CoAuthors) > 0 {
		fmt.Fprintf(w, "  Co-authors:\n")
		for _, ca := range plan.CoAuthors {
			fmt.Fprintf(w, "    %s <%s>\n", ca.Name, ca.Email)
		}
	}
	if plan.Mode == "squash" && plan.CommitMessage != "" {
		fmt.Fprintf(w, "  Composed commit message:\n")
		for _, line := range strings.Split(strings.TrimRight(plan.CommitMessage, "\n"), "\n") {
			fmt.Fprintf(w, "    | %s\n", line)
		}
	}
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
