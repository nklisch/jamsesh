package hooks

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/hookio"
	"jamsesh/cmd/jamsesh/pusherr"
	"jamsesh/cmd/jamsesh/retryqueue"
)

// stopInput mirrors the CC Stop hook input schema.
type stopInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
}

// stopOutput is the CC Stop hook output.
type stopOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// maxQueueSize is the retry-queue depth that triggers a "session wedged" error.
const maxQueueSize = 10

// handleStop implements the stop hook logic.
func handleStop(_ context.Context, _ stopInput) (stopOutput, error) {
	ss, err := resolveHookSession()
	if err != nil {
		return stopOutput{}, err
	}
	if ss == nil {
		return stopOutput{}, nil
	}

	// Step 1: Check if working tree is dirty.
	statusOut, _, _ := HookRunGit("status", "--porcelain")
	isDirty := strings.TrimSpace(statusOut) != ""

	if isDirty {
		// Step 2: Auto-commit the dirty working tree.
		_, _, addCode := HookRunGit("add", "-A")
		if addCode != 0 {
			fmt.Fprintln(os.Stderr, "jamsesh: git add -A failed during auto-commit")
			// Continue anyway — the commit may still capture staged changes.
		}

		commitMsg := "WIP [jamsesh auto-commit at turn end]"
		_, commitStderr, commitCode := HookRunGit(
			"commit",
			"-m", commitMsg,
			"--trailer", "Jam-Auto-Commit: true",
		)
		if commitCode != 0 {
			// Nothing to commit (e.g. untracked-only changes) or other error.
			if !strings.Contains(commitStderr, "nothing to commit") {
				fmt.Fprintf(os.Stderr, "jamsesh: auto-commit failed: %s\n", commitStderr)
			}
			// Do not abort stop — continue to push and queue checks.
		}
	}

	// Step 3: Final push with retry policy.
	refOut, _, _ := HookRunGit("rev-parse", "--abbrev-ref", "HEAD")
	ref := strings.TrimSpace(refOut)
	if ref == "" || ref == "HEAD" {
		ref = ss.Ref
	}

	result := pushCommitWithRetry(ref, 3)
	switch result.Class {
	case pusherr.Transient:
		// All retries exhausted — enqueue.
		sha, _, _ := HookRunGit("rev-parse", "HEAD")
		q := &retryqueue.Queue{SessionID: ss.SessionID}
		entry := retryqueue.Entry{
			CommitSHA:   strings.TrimSpace(sha),
			Attempts:    3,
			LastError:   result.Message,
			LastErrorAt: time.Now(),
		}
		if enqErr := q.Enqueue(entry); enqErr != nil {
			fmt.Fprintf(os.Stderr, "jamsesh: failed to enqueue retry at stop: %v\n", enqErr)
		}
	case pusherr.Permanent:
		fmt.Fprintf(os.Stderr, "jamsesh: push rejected (permanent): %s %s\n", result.Code, result.Message)
	}

	// Step 4: Check retry queue size.
	q := &retryqueue.Queue{SessionID: ss.SessionID}
	size, err := q.Size()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jamsesh: checking retry queue: %v\n", err)
	}
	if size > maxQueueSize {
		fmt.Fprintln(os.Stderr, "jamsesh: session is wedged — retry queue has too many pending commits; run `jamsesh status` to investigate")
		os.Exit(1)
	}

	// Step 5: POST turn.ended — skipped in v1. The portal endpoint does not yet
	// exist. This will be wired up in a future story when the endpoint is added
	// to the API spec.

	return stopOutput{}, nil
}

// Stop is the urfave/cli action for "jamsesh hook stop".
func Stop(ctx context.Context, _ *cli.Command) error {
	if err := hookio.Run(ctx, stdinOf(ctx), stdoutOf(ctx), handleStop); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	return nil
}
