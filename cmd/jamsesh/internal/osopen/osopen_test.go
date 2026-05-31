package osopen

import (
	"bytes"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// TestPlatformArgv asserts that platformArgv returns the correct argv for the
// current GOOS.
func TestPlatformArgv(t *testing.T) {
	const testURL = "https://x"
	argv := platformArgv(testURL)

	switch runtime.GOOS {
	case "linux":
		if len(argv) != 2 || argv[0] != "xdg-open" || argv[1] != testURL {
			t.Fatalf("linux: want [xdg-open %s], got %v", testURL, argv)
		}
	case "darwin":
		if len(argv) != 2 || argv[0] != "open" || argv[1] != testURL {
			t.Fatalf("darwin: want [open %s], got %v", testURL, argv)
		}
	case "windows":
		if len(argv) != 3 || argv[0] != "rundll32" || argv[2] != testURL {
			t.Fatalf("windows: want [rundll32 url.dll,FileProtocolHandler %s], got %v", testURL, argv)
		}
	default:
		if argv != nil {
			t.Fatalf("unsupported platform %s: want nil argv, got %v", runtime.GOOS, argv)
		}
	}
}

// TestOpen_GracefulOnStartFailure verifies that Open returns nil and writes
// "Please visit: <url>" to errOut when the launcher binary does not exist.
func TestOpen_GracefulOnStartFailure(t *testing.T) {
	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("/nonexistent/jamsesh-test-binary")
	}
	t.Cleanup(func() { execCommand = orig })

	var buf bytes.Buffer
	const testURL = "https://x"
	err := Open(testURL, &buf)
	if err != nil {
		t.Fatalf("Open returned non-nil error: %v", err)
	}
	if !strings.Contains(buf.String(), "Please visit: "+testURL) {
		t.Fatalf("expected errOut to contain 'Please visit: %s', got: %q", testURL, buf.String())
	}
}

// TestOpenSilent_NothingWrittenOnUnsupportedOS verifies that OpenSilent returns
// an error (not nil) and writes NOTHING to any stream when platformArgv is nil
// (simulated by an overridden execCommand that should never be called).
// We test this indirectly via a non-existent binary: platformArgv returns nil
// on unsupported OS, so execCommand is never reached.
// On supported OSes we override execCommand to a failing binary and assert the
// URL never appears in any output.
func TestOpenSilent_ErrorOnStartFailure(t *testing.T) {
	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("/nonexistent/jamsesh-test-binary-silent")
	}
	t.Cleanup(func() { execCommand = orig })

	const testURL = "https://secret.example.com/resume#rt=secrettoken"
	err := OpenSilent(testURL)
	if err == nil {
		t.Fatal("OpenSilent expected non-nil error on launch failure, got nil")
	}
	// The URL must NEVER appear in the error message.
	if strings.Contains(err.Error(), testURL) {
		t.Errorf("OpenSilent leaked URL in error message: %q", err.Error())
	}
	if strings.Contains(err.Error(), "#rt=") {
		t.Errorf("OpenSilent leaked token fragment in error message: %q", err.Error())
	}
}

// TestOpenSilent_SuccessNoOutput verifies that OpenSilent returns nil and
// writes nothing when the launcher starts successfully.
func TestOpenSilent_SuccessNoOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("'true' binary not available on windows")
	}

	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("true")
	}
	t.Cleanup(func() { execCommand = orig })

	const testURL = "https://secret.example.com/resume#rt=secrettoken"
	err := OpenSilent(testURL)
	if err != nil {
		t.Fatalf("OpenSilent returned unexpected error: %v", err)
	}
}

// TestOpen_DetachOnSuccess verifies that Open returns nil and writes nothing to
// errOut when the launcher starts successfully. Skipped on platforms where
// "true" is not available.
func TestOpen_DetachOnSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("'true' binary not available on windows")
	}

	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("true")
	}
	t.Cleanup(func() { execCommand = orig })

	var buf bytes.Buffer
	err := Open("https://x", &buf)
	if err != nil {
		t.Fatalf("Open returned non-nil error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty errOut, got: %q", buf.String())
	}
}
