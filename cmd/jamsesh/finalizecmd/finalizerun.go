package finalizecmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/cmd/jamsesh/state"
)

// FinalizeRunCommand returns the urfave/cli descriptor for
// `jamsesh finalize-run <plan-id>`. This is the workhorse the portal
// hands the user via copy-to-clipboard.
func FinalizeRunCommand() *cli.Command {
	return &cli.Command{
		Name:      "finalize-run",
		Usage:     "Execute a finalize plan in the current working directory",
		ArgsUsage: "<plan-id>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Skip interactive confirmations (proceed prompt + stash prompt)",
			},
			&cli.BoolFlag{
				Name:  "print-script",
				Usage: "Print the raw shell script instead of executing",
			},
		},
		Action: finalizeRunAction,
	}
}

func finalizeRunAction(ctx context.Context, cmd *cli.Command) error {
	out := os.Stdout

	// Step 1: parse the plan-id argument.
	raw := cmd.Args().First()
	if raw == "" {
		return errors.New("usage: jamsesh finalize-run <session>:<lock>")
	}
	pid, err := parsePlanID(raw)
	if err != nil {
		return err
	}

	// Resolve org for portal calls.
	orgID, err := readOrgIDForSession(pid.SessionID)
	if err != nil {
		return err
	}

	portalURL, err := state.ReadPortalURL()
	if err != nil {
		return fmt.Errorf("resolving portal URL: %w", err)
	}
	pc := &portalclient.Client{BaseURL: portalURL}

	// Step 2: mid-pick detection — BEFORE pre-flight, BEFORE anything
	// mutating. If the user is mid-pick we report state and exit clean.
	if offending, err := detectMidPick("."); err != nil {
		return fmt.Errorf("checking for in-progress cherry-pick: %w", err)
	} else if offending != "" {
		// Best-effort plan fetch so we can enumerate remaining commits.
		// Failure here is non-fatal — we still report the offending SHA.
		plan, _ := fetchPlan(ctx, pc, orgID, pid.SessionID, pid.LockID)
		reportMidPick(out, offending, plan)
		return nil
	}

	// Step 3: fetch the plan.
	plan, err := fetchPlan(ctx, pc, orgID, pid.SessionID, pid.LockID)
	if err != nil {
		return fmt.Errorf("fetching plan: %w", err)
	}

	// Step 4: print summary so the user confirms intent locally.
	printPlanSummary(out, plan)
	fmt.Fprintln(out)

	// Print-script short-circuit: dump and exit without touching the repo.
	if cmd.Bool("print-script") {
		return printScript(out, plan)
	}

	yes := cmd.Bool("yes")

	// Helper that --yes uses to auto-confirm any prompt.
	confirmer := confirmFn(func(prompt string, defaultYes bool) (bool, error) {
		if yes {
			return true, nil
		}
		return confirm(out, prompt, defaultYes)
	})

	// Step 5: pre-flight.
	pre, err := runPreflight(out, plan, confirmer, plan.PlanID)
	if err != nil {
		return err
	}
	_ = pre.OriginalBranch // story 2 wires the cleanup; recorded here.

	// Step 6: proceed prompt.
	ok, err := confirmer("Proceed with the finalize?", true)
	if err != nil {
		return err
	}
	if !ok {
		// Pop the stash we created if we created one — the user is
		// bailing on the whole flow.
		if pre.Stashed {
			fmt.Fprintf(out, "+ git stash pop\n")
			_ = runGit("stash", "pop")
		}
		return errors.New("aborted by user")
	}

	// Step 7: choose fetch source + fetch.
	fs, err := chooseFetchSource(ctx, pc, plan, pid.SessionID)
	if err != nil {
		return fmt.Errorf("choosing fetch source: %w", err)
	}
	defer func() {
		if cleanupErr := fs.cleanup(); cleanupErr != nil {
			fmt.Fprintf(out, "Warning: fetch-source cleanup failed: %v\n", cleanupErr)
		}
	}()
	if err := performFetch(out, fs); err != nil {
		return fmt.Errorf("fetching session refs: %w", err)
	}

	// Step 8: resolve runner identity for the (possible) squash commit.
	runner, err := resolveRunnerIdentity()
	if err != nil {
		return err
	}

	// Step 9: execute the cherry-pick sequence.
	execErr := execute(out, plan, runner)

	// Step 10a: conflict halt — print resume hint and exit non-zero.
	var conflict *conflictError
	if errors.As(execErr, &conflict) {
		renderConflict(out, conflict)
		// IMPORTANT: do NOT pop the stash on conflict — the user
		// needs a clean working tree to resolve.
		return execErr
	}
	if execErr != nil {
		return execErr
	}

	// Step 10b: clean exit — pop stash if we stashed, print next steps.
	if pre.Stashed {
		fmt.Fprintf(out, "+ git stash pop\n")
		if err := runGit("stash", "pop"); err != nil {
			// Non-fatal: the finalize succeeded; user can pop manually.
			fmt.Fprintf(out, "Warning: `git stash pop` failed (%v); resolve with `git stash list`.\n", err)
		}
	}
	fmt.Fprintf(out, "\nBranch %q is ready. Push when you're ready:\n", plan.TargetBranch)
	fmt.Fprintf(out, "  git push origin %s\n", plan.TargetBranch)
	fmt.Fprintf(out, "Then mark the session shipped in the portal.\n")
	return nil
}
