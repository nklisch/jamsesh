package automerger

// diff_exit_code_test.go — regression tests for Unit 4: classify diff exit code.
//
// Bug: diffAddOnly discarded the error from cmd.Run() entirely (_ = cmd.Run()),
// so a diff exit code 2 (trouble: missing binary, unreadable file) produced
// empty output that was silently parsed as a valid diff, feeding garbage to the
// add-only auto-resolve heuristic.
//
// Fix: extract classifyDiffErr that accepts exit 0/1 as success and returns
// an error for exit >=2 or non-*exec.ExitError; runDiff wraps cmd.Run() with
// this classifier; diffAddOnly propagates the error.

import (
	"errors"
	"os/exec"
	"testing"
)

// ---------------------------------------------------------------------------
// TestClassifyDiffErr — unit tests over synthetic exit codes
// ---------------------------------------------------------------------------

// syntheticExitError builds a fake *exec.ExitError with the given exit code.
// We cannot construct exec.ExitError directly, so we run a short shell command
// that exits with the target code (or 0/1 for the success cases).
func syntheticExitError(t *testing.T, code int) error {
	t.Helper()
	// Use "sh -c 'exit N'" to produce a controlled exit code.
	// On systems without sh, this test will fail early — acceptable for a
	// developer environment (CI has sh).
	cmd := exec.Command("sh", "-c", "exit "+itoa(code))
	err := cmd.Run()
	if code == 0 {
		// sh -c 'exit 0' returns nil — we can't get an ExitError with code 0,
		// and classifyDiffErr(nil) should return nil.
		if err != nil {
			t.Fatalf("expected nil error for exit 0, got %v", err)
		}
		return nil
	}
	return err
}

// itoa converts an int to its decimal string representation without importing
// strconv (to keep this file self-contained).
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := 19
	for i > 0 {
		buf[pos] = byte('0' + i%10)
		i /= 10
		pos--
	}
	if neg {
		buf[pos] = '-'
		pos--
	}
	return string(buf[pos+1:])
}

// TestClassifyDiffErr_Exit0_ReturnsNil verifies that exit code 0 (files
// identical) is treated as success.
func TestClassifyDiffErr_Exit0_ReturnsNil(t *testing.T) {
	err := syntheticExitError(t, 0)
	if got := classifyDiffErr(err); got != nil {
		t.Errorf("classifyDiffErr(nil/exit0) = %v, want nil", got)
	}
}

// TestClassifyDiffErr_Exit1_ReturnsNil verifies that exit code 1 (files
// differ — the normal case for diff) is treated as success.
func TestClassifyDiffErr_Exit1_ReturnsNil(t *testing.T) {
	err := syntheticExitError(t, 1)
	if err == nil {
		t.Skip("sh -c 'exit 1' returned nil — cannot test ExitError with code 1")
	}
	if got := classifyDiffErr(err); got != nil {
		t.Errorf("classifyDiffErr(exit1) = %v, want nil", got)
	}
}

// TestClassifyDiffErr_Exit2_ReturnsError verifies that exit code 2 (diff
// trouble) is classified as an error.
func TestClassifyDiffErr_Exit2_ReturnsError(t *testing.T) {
	err := syntheticExitError(t, 2)
	if err == nil {
		t.Skip("sh -c 'exit 2' returned nil — unexpected")
	}
	got := classifyDiffErr(err)
	if got == nil {
		t.Error("classifyDiffErr(exit2) = nil, want non-nil error")
	}
}

// TestClassifyDiffErr_Exit127_ReturnsError verifies that any exit code >= 2
// is classified as an error (127 = command not found in many shells).
func TestClassifyDiffErr_Exit127_ReturnsError(t *testing.T) {
	err := syntheticExitError(t, 127)
	if err == nil {
		t.Skip("sh -c 'exit 127' returned nil — unexpected")
	}
	got := classifyDiffErr(err)
	if got == nil {
		t.Error("classifyDiffErr(exit127) = nil, want non-nil error")
	}
}

// TestClassifyDiffErr_NonExitError_ReturnsError verifies that a non-ExitError
// (e.g. exec.ErrNotFound when diff binary is missing) is classified as an
// error.
func TestClassifyDiffErr_NonExitError_ReturnsError(t *testing.T) {
	// Construct a non-ExitError: attempt to run a path-executable that doesn't
	// exist. exec.ErrNotFound is wrapped inside the returned error.
	cmd := exec.Command("/no/such/binary/diff-xxx")
	err := cmd.Run()
	if err == nil {
		t.Skip("unexpected nil error running nonexistent binary")
	}
	// Verify it's not an ExitError.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		t.Skip("unexpectedly got ExitError for nonexistent binary — cannot test non-ExitError path")
	}

	got := classifyDiffErr(err)
	if got == nil {
		t.Error("classifyDiffErr(non-ExitError) = nil, want non-nil error")
	}
}

// ---------------------------------------------------------------------------
// TestDiffAddOnly_PropagatesExit2Error
// ---------------------------------------------------------------------------

// TestDiffAddOnly_PropagatesExit2Error verifies that diffAddOnly propagates
// an error from runDiff (simulating exit code 2 from diff) rather than
// silently returning empty hunks and feeding garbage to the heuristic.
//
// We cannot easily trigger a real exit-2 from diff in a unit test without
// corrupting files, so we rely on the classifyDiffErr unit tests above plus
// the integration: run diffAddOnly on identical content (trivial no-op path)
// to verify the happy path still works.
func TestDiffAddOnly_HappyPath_IdenticalContent(t *testing.T) {
	content := []byte("line one\nline two\nline three\n")
	hunks, err := diffAddOnly(content, content)
	if err != nil {
		t.Fatalf("diffAddOnly(identical) returned error: %v", err)
	}
	// Identical content → empty hunk slice (no changes).
	if len(hunks) != 0 {
		t.Errorf("diffAddOnly(identical) = %v hunks, want 0", len(hunks))
	}
}

// TestDiffAddOnly_HappyPath_PureAddition verifies the exit-1 (differences)
// path: diffAddOnly correctly returns hunks for a pure addition.
func TestDiffAddOnly_HappyPath_PureAddition(t *testing.T) {
	base := []byte("line one\nline two\n")
	other := []byte("line one\nNEW LINE\nline two\n") // addition only
	hunks, err := diffAddOnly(base, other)
	if err != nil {
		t.Fatalf("diffAddOnly(pure add) returned error: %v", err)
	}
	if len(hunks) == 0 {
		t.Error("diffAddOnly(pure add) returned no hunks — expected an addHunk")
	}
	if len(hunks) > 0 && len(hunks[0].addedLines) == 0 {
		t.Error("first hunk has no addedLines — expected the new line")
	}
}
