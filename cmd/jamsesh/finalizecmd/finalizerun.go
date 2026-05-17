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

	// Cleanup stack — wired with the root context so a SIGINT during
	// any subsequent step drains the stack before exit. Outcome is
	// flipped to outcomeSuccess only on a clean end-to-end completion;
	// every other exit path (user abort, fetch failure, conflict)
	// drains with outcomeAborted, which leaves the stash and the
	// partial branch intact for recovery but still removes the
	// temporary jamsesh remote.
	cleanup := newCleanupStack(ctx, out)
	outcome := outcomeAborted
	defer func() { _ = cleanup.Run(outcome) }()

	// Register the conditional cleanups first so they sit at the
	// bottom of the LIFO stack — the unconditional remote removal
	// runs before them on a clean exit, which is the order we want
	// (drop the throwaway credential ASAP).
	if pre.Stashed {
		// We never push another stash during the run, so popping by
		// HEAD targets the entry we created in preflight. The
		// pre.StashMessage value is retained on the struct for debug
		// surfaces (it is used by tests asserting the stash message
		// shape) but the pop itself does not need it.
		cleanup.Push("stash pop", true, func() error {
			fmt.Fprintf(out, "+ git stash pop\n")
			return runGit("stash", "pop")
		})
	}
	if pre.OriginalBranch != "" {
		original := pre.OriginalBranch
		cleanup.Push("original branch restore", true, func() error {
			fmt.Fprintf(out, "+ git checkout %s\n", original)
			return runGit("checkout", original)
		})
	}

	// Step 6: proceed prompt.
	ok, err := confirmer("Proceed with the finalize?", true)
	if err != nil {
		return err
	}
	if !ok {
		// User declined: drain the stack right now with outcomeSuccess
		// so the stash pops cleanly (we never registered a remote
		// cleanup, so the unconditional layer is empty). Set outcome
		// to success and let the deferred Run handle it — Run is
		// idempotent, so the defer above will no-op on second call.
		outcome = outcomeSuccess
		return errors.New("aborted by user")
	}

	// Step 7: choose fetch source. For the HTTPS path, chooseFetchSource
	// registers the temporary `jamsesh` remote before returning so the
	// cleanup func has something to remove. We push that cleanup
	// (unconditional — runs on every exit path) immediately so any
	// failure between here and Run drains the remote.
	fs, err := chooseFetchSource(ctx, pc, plan, orgID, pid.SessionID)
	if err != nil {
		return fmt.Errorf("choosing fetch source: %w", err)
	}
	cleanup.Push("fetch-source cleanup", false, fs.cleanup)

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

	// Step 10a: conflict halt — print resume hint, leave the WT in the
	// partial state, drain with outcomeAborted (the deferred Run
	// already does this by default).
	var conflict *conflictError
	if errors.As(execErr, &conflict) {
		renderConflict(out, conflict)
		return execErr
	}
	if execErr != nil {
		return execErr
	}

	// Step 10b: clean exit — print next steps and flip outcome so the
	// deferred Run pops the stash and restores the original branch.
	fmt.Fprintf(out, "\nBranch %q is ready. Push when you're ready:\n", plan.TargetBranch)
	fmt.Fprintf(out, "  git push origin %s\n", plan.TargetBranch)
	fmt.Fprintf(out, "Then mark the session shipped in the portal.\n")
	outcome = outcomeSuccess
	return nil
}
