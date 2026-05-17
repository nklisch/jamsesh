// Package containerlog provides a shared helper for the Testcontainers
// fixtures (mailhog, wiremock, toxiproxy, portal) to dump container logs
// via t.Logf when a test has failed, before terminating the container.
//
// Usage:
//
//	t.Cleanup(func() {
//	    containerlog.DumpAndTerminate(ctx, t, c, "portal")
//	})
//
// On a passing test, DumpAndTerminate behaves identically to a plain
// testcontainers.TerminateContainer(c) call — no log noise. On a failed
// test, the container's stdout+stderr stream is read via Container.Logs
// and emitted with t.Logf("<fixture> container logs on failure:\n%s",
// ...) so CI runners pick the logs up in the failure output.
//
// The postgres fixture is intentionally NOT wired through this helper.
// Its container is a sync.Once singleton shared across the test binary,
// so per-test log dumps would interleave with prior-test logs and add
// more noise than signal. If postgres-level log capture becomes useful,
// it should be added at the shared-container layer with its own
// strategy (e.g., remembering the byte offset at Start time).
package containerlog

import (
	"context"
	"io"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

// DumpAndTerminate is registered as a t.Cleanup callback by each
// fixture's Start function. When the test has failed by the time
// cleanup runs, it reads the container's full log stream and emits it
// via t.Logf, then terminates the container. On a passing test, the
// log-read step is skipped entirely.
//
// name is the short label used in log/error messages (e.g. "portal",
// "mailhog") so a developer scanning the test output can match each
// log block to its source fixture.
//
// Errors during log capture are themselves logged via t.Logf — they
// must not mask the original test failure or prevent termination.
func DumpAndTerminate(ctx context.Context, t *testing.T, c testcontainers.Container, name string) {
	t.Helper()
	if t.Failed() {
		dumpLogs(ctx, t, c, name)
	}
	if err := testcontainers.TerminateContainer(c); err != nil {
		t.Logf("%s: cleanup: terminate: %v", name, err)
	}
}

func dumpLogs(ctx context.Context, t *testing.T, c testcontainers.Container, name string) {
	t.Helper()
	if c == nil {
		return
	}
	rc, err := c.Logs(ctx)
	if err != nil {
		t.Logf("%s: cleanup: read container logs: %v", name, err)
		return
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Logf("%s: cleanup: read container log body: %v", name, err)
		return
	}
	if len(data) == 0 {
		return
	}
	t.Logf("%s container logs on failure:\n%s", name, data)
}
