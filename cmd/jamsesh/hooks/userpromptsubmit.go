package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/hookio"
	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/cmd/jamsesh/pusherr"
	"jamsesh/cmd/jamsesh/retryqueue"
	"jamsesh/cmd/jamsesh/state"
	"jamsesh/internal/api/openapi"
)

// userPromptSubmitInput mirrors the CC UserPromptSubmit hook input schema.
type userPromptSubmitInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
}

// userPromptSubmitOutput is the CC UserPromptSubmit hook output.
type userPromptSubmitOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// HookRunGit is the git execution function used by user-prompt-submit,
// post-tool-use, and stop. Override in tests via package-level variable
// injection (exported so external test packages can swap it).
var HookRunGit = func(args ...string) (stdout string, stderr string, exitCode int) {
	cmd := exec.Command("git", args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = 1
		}
	}
	return strings.TrimSpace(outBuf.String()), strings.TrimSpace(errBuf.String()), code
}

// humanDuration converts a duration in seconds to a short human-readable
// string like "4 min 47 sec". Used when rendering the playground destruction
// warning so the urgency is immediately legible.
func humanDuration(seconds int) string {
	if seconds <= 0 {
		return "0 sec"
	}
	m := seconds / 60
	s := seconds % 60
	if m == 0 {
		return fmt.Sprintf("%d sec", s)
	}
	if s == 0 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%d min %d sec", m, s)
}

// readLastSeq reads the last_seen_seq for a session from local state.
func readLastSeq(sessionID string) (int64, error) {
	data, err := state.Read(filepath.Join("sessions", sessionID, "last_seen_seq"))
	if err != nil {
		return 0, nil // treat missing file as seq=0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, nil // corrupt file — start from 0
	}
	return n, nil
}

// writeLastSeq atomically writes the last_seen_seq for a session.
func writeLastSeq(sessionID string, seq int64) error {
	return state.Write(
		filepath.Join("sessions", sessionID, "last_seen_seq"),
		[]byte(strconv.FormatInt(seq, 10)),
		0o600,
	)
}

// pushCommitWithRetry attempts to push sha to remote using the given ref,
// retrying on transient errors up to maxAttempts times (with exponential
// backoff). Returns the final pusherr.Result and whether the push succeeded.
func pushCommitWithRetry(ref string, maxAttempts int) pusherr.Result {
	backoffs := []time.Duration{250 * time.Millisecond, time.Second, 4 * time.Second}

	var last pusherr.Result
	for attempt := 0; attempt < maxAttempts; attempt++ {
		_, stderr, code := HookRunGit("push", "session-remote", ref)

		// Map exit code to HTTP-like status for the classifier.
		// exit 0 → OK; non-zero with JSON error envelope → parse; else treat as network error.
		httpStatus := 0
		if code == 0 {
			httpStatus = 200
		} else {
			// Try to extract an HTTP status embedded in stderr by the smart-http server.
			httpStatus = extractHTTPStatus(stderr)
		}
		// Extract just the JSON body from stderr (the smart-http server may
		// prefix the response body with "error: <N> <phrase>\n").
		body := extractJSONBody(stderr)

		result := pusherr.Classify(httpStatus, body)
		last = result

		if result.Class == pusherr.OK {
			return result
		}
		if result.Class == pusherr.Permanent {
			return result
		}
		// Transient: backoff and retry (unless this was the last attempt).
		if attempt < maxAttempts-1 {
			delay := backoffs[attempt]
			if attempt < len(backoffs) {
				delay = backoffs[attempt]
			}
			time.Sleep(delay)
		}
	}
	return last
}

// extractHTTPStatus tries to parse an HTTP status code embedded in a git
// smart-http error message (e.g. "error: 422 Unprocessable Entity"). If it
// cannot find one it returns 0 (network-level error bucket).
func extractHTTPStatus(stderr string) int {
	// git smart-http surfaces: "error: <N> <phrase>"
	lower := strings.ToLower(stderr)
	if idx := strings.Index(lower, "error: "); idx >= 0 {
		rest := stderr[idx+len("error: "):]
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			if n, err := strconv.Atoi(fields[0]); err == nil && n >= 100 && n < 600 {
				return n
			}
		}
	}
	return 0
}

// extractJSONBody returns the first JSON object found in s as a byte slice.
// When the git smart-http server writes an error envelope, it prepends a line
// such as "error: 422 Unprocessable Entity\n" before the JSON payload. This
// helper skips non-JSON prefix lines and returns only the JSON portion so that
// pusherr.Classify can parse it. If no JSON object is found the full stderr is
// returned as the body (the classifier will fail to parse it and use default
// classification).
func extractJSONBody(s string) []byte {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{") {
			return []byte(trimmed)
		}
	}
	return []byte(s)
}

// handleUserPromptSubmit implements the user-prompt-submit hook logic.
func handleUserPromptSubmit(ctx context.Context, _ userPromptSubmitInput) (userPromptSubmitOutput, error) {
	ss, err := resolveHookSession()
	if err != nil {
		return userPromptSubmitOutput{}, err
	}
	if ss == nil {
		return userPromptSubmitOutput{}, nil
	}

	var sb strings.Builder

	// Step 1: git fetch from session-remote.
	_, fetchErr, fetchCode := HookRunGit("fetch", "session-remote")
	if fetchCode != 0 {
		// Non-fatal: log but continue. The digest will still have portal data.
		fmt.Fprintf(&sb, "## Warning: git fetch failed\n%s\n\n", fetchErr)
	}

	// Step 2: Drain retry queue — push each queued commit.
	q := &retryqueue.Queue{SessionID: ss.SessionID}
	queued, err := q.Drain()
	if err != nil {
		return userPromptSubmitOutput{}, fmt.Errorf("user-prompt-submit: draining retry queue: %w", err)
	}

	var reEnqueue []retryqueue.Entry
	for _, entry := range queued {
		result := pushCommitWithRetry(entry.CommitSHA, 3)
		switch result.Class {
		case pusherr.OK:
			// Pushed successfully.
		case pusherr.Transient:
			// Still failing — re-enqueue with updated attempt count.
			entry.Attempts++
			entry.LastError = result.Message
			entry.LastErrorAt = time.Now()
			reEnqueue = append(reEnqueue, entry)
		case pusherr.Permanent:
			// Permanent failure — drop and log.
			fmt.Fprintf(os.Stderr, "jamsesh: dropping queued commit %s (permanent push error: %s %s)\n",
				entry.CommitSHA, result.Code, result.Message)
		}
	}
	// Re-save any entries that are still transient.
	for _, e := range reEnqueue {
		if enqErr := q.Enqueue(e); enqErr != nil {
			fmt.Fprintf(os.Stderr, "jamsesh: re-enqueue failed for %s: %v\n", e.CommitSHA, enqErr)
		}
	}

	// Step 3: GET /digest?since=<lastSeq>.
	lastSeq, err := readLastSeq(ss.SessionID)
	if err != nil {
		return userPromptSubmitOutput{}, fmt.Errorf("user-prompt-submit: reading lastSeq: %w", err)
	}

	pc, err := buildPortalClient(ss.SessionID)
	if err != nil {
		return userPromptSubmitOutput{}, err
	}

	digestPath := fmt.Sprintf("/api/orgs/%s/sessions/%s/digest?since=%d", ss.OrgID, ss.SessionID, lastSeq)
	digest, err := portalclient.GetJSON[openapi.DigestResponse](ctx, pc, digestPath)
	if err != nil {
		// Non-fatal: include error in context but don't fail the hook.
		fmt.Fprintf(&sb, "## Warning: could not fetch digest\n%v\n\n", err)
	} else {
		// Step 4: Surface urgent events (e.g. playground destruction warning)
		// before the regular digest text so they grab the agent's attention.
		if len(digest.UrgentEvents) > 0 {
			sb.WriteString("## ⚠️  Urgent\n\n")
			for _, ev := range digest.UrgentEvents {
				sb.WriteString(fmt.Sprintf(
					"⚠️  Playground session ending in %s due to %s.\n"+
						"   Ends at %s. Run `jamsesh finalize --local` now to keep your work.\n\n",
					humanDuration(ev.RemainingSeconds),
					string(ev.Reason),
					ev.EndsAt.Format(time.RFC3339),
				))
			}
		}

		sb.WriteString(digest.Text)
		// Step 5: Advance lastSeq.
		if writeErr := writeLastSeq(ss.SessionID, digest.NextCursor); writeErr != nil {
			fmt.Fprintf(os.Stderr, "jamsesh: updating lastSeq: %v\n", writeErr)
		}
	}

	return userPromptSubmitOutput{AdditionalContext: sb.String()}, nil
}

// UserPromptSubmit is the urfave/cli action for "jamsesh hook user-prompt-submit".
func UserPromptSubmit(ctx context.Context, _ *cli.Command) error {
	if err := hookio.Run(ctx, stdinOf(ctx), stdoutOf(ctx), handleUserPromptSubmit); err != nil {
		return fmt.Errorf("user-prompt-submit: %w", err)
	}
	return nil
}
