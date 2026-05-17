package finalizecmd

import (
	"fmt"
	"io"
	"strings"
)

// preflightResult carries side-effect state from runPreflight that the
// orchestration layer needs at cleanup time.
type preflightResult struct {
	// OriginalBranch is the branch the user was on before we ran.
	// Used to offer `git checkout -` on clean exit (story 2 wires the
	// cleanup; this story just records the value).
	OriginalBranch string

	// Stashed is true when we created a stash on the user's behalf to
	// clear a dirty working tree. The orchestrator pops the stash on
	// clean exit (NOT on conflict exit — the user needs the WT state
	// to resolve conflicts).
	Stashed bool

	// StashMessage is the -m argument we passed to `git stash push`,
	// used to target the right entry at pop time.
	StashMessage string
}

// runPreflight runs the 7 ordered checks in the feature design body.
// Any failure that bails returns a wrapped error suitable for direct
// printing to the user. The prompt callback handles the dirty-WT stash
// question; injectable so tests can simulate y/n without stdin.
func runPreflight(out io.Writer, plan *Plan, prompt confirmFn, planIDStr string) (*preflightResult, error) {
	res := &preflightResult{}

	// 1. cwd is a git repo.
	if _, err := runGitOutput("rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, fmt.Errorf("current directory is not a git repository (run from your source-repo checkout)")
	}

	// 2. Mid-pick — handled BEFORE preflight by the caller (it short-
	// circuits the whole flow with reportMidPick). We do not re-check
	// here; if the caller reached us, we are not mid-pick.

	// 3. Local branch collision.
	if _, err := runGitOutput("rev-parse", "--verify", "refs/heads/"+plan.TargetBranch); err == nil {
		return nil, fmt.Errorf("local branch %q already exists; rename or delete it (e.g. `git branch -d %s`) and re-run", plan.TargetBranch, plan.TargetBranch)
	}

	// 4. Remote branch collision (origin = user's source remote).
	remoteRefs, err := runGitOutput("ls-remote", "--heads", "origin", plan.TargetBranch)
	if err != nil {
		// Origin unreachable is caught by check 7 below; swallow here
		// rather than re-erroring with a bad message.
	} else if strings.TrimSpace(remoteRefs) != "" {
		return nil, fmt.Errorf("remote branch %q already exists on origin; rename the target or delete the remote branch first", plan.TargetBranch)
	}

	// 5. Dirty working tree.
	status, err := runGitOutput("status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("checking working tree status: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		ok, err := prompt("Working tree is dirty. Stash first?", true)
		if err != nil {
			return nil, fmt.Errorf("reading stash confirmation: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("working tree is dirty; commit or stash your changes and re-run")
		}
		stashMsg := fmt.Sprintf("jamsesh finalize-run %s", planIDStr)
		fmt.Fprintf(out, "+ git stash push -u -m %q\n", stashMsg)
		if err := runGit("stash", "push", "-u", "-m", stashMsg); err != nil {
			return nil, fmt.Errorf("git stash push failed: %w", err)
		}
		res.Stashed = true
		res.StashMessage = stashMsg
	}

	// 6. Current branch awareness (record; warn if unpushed commits).
	if cur, err := runGitOutput("symbolic-ref", "--short", "HEAD"); err == nil {
		res.OriginalBranch = strings.TrimSpace(cur)
		// Best-effort unpushed-commit warning. Failures are silent —
		// the upstream may not be configured, which is fine.
		if cur != "" {
			counts, cerr := runGitOutput("rev-list", "--left-right", "--count",
				"origin/"+cur+"...HEAD")
			if cerr == nil {
				// Output: "<left>\t<right>" — right is commits ahead of upstream.
				fields := strings.Fields(counts)
				if len(fields) == 2 && fields[1] != "0" {
					fmt.Fprintf(out, "Warning: current branch %q has %s unpushed commit(s) vs origin/%s.\n",
						cur, fields[1], cur)
				}
			}
		}
	}

	// 7. Origin reachable.
	if _, err := runGitOutput("ls-remote", "origin"); err != nil {
		return nil, fmt.Errorf("origin remote unreachable: %w (needed to push the final branch)", err)
	}

	return res, nil
}
