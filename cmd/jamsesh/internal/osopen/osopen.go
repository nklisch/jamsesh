// Package osopen launches URLs in the user's default browser with graceful
// degradation. Single home for the platform xdg-open/open/rundll32 logic
// previously inlined in cmd/jamsesh/auth and cmd/jamsesh/finalizecmd.
package osopen

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

// execCommand is overridable in tests to avoid launching a real browser.
var execCommand = exec.Command

// Open launches rawURL in the user's default browser. Any failure —
// unsupported platform, missing launcher, exec error — degrades gracefully:
// rawURL is written to errOut so the caller can copy it, and nil is returned.
// The launched process is detached.
func Open(rawURL string, errOut io.Writer) error {
	argv := platformArgv(rawURL)
	if argv == nil {
		fmt.Fprintf(errOut, "Cannot open browser automatically on %s.\nPlease visit: %s\n", runtime.GOOS, rawURL)
		return nil
	}
	cmd := execCommand(argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(errOut, "Could not launch browser: %v\nPlease visit: %s\n", err, rawURL)
		return nil
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// OpenSilent launches rawURL in the user's default browser but NEVER writes
// the URL (or any derivative of it) to any output. Any failure — unsupported
// platform, missing launcher, exec error — is returned as an error so the
// caller can decide how to report it without echoing a secret URL.
// The launched process is detached, matching the behaviour of Open.
func OpenSilent(rawURL string) error {
	argv := platformArgv(rawURL)
	if argv == nil {
		return fmt.Errorf("cannot open browser automatically on %s", runtime.GOOS)
	}
	cmd := execCommand(argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not launch browser: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// platformArgv returns the launcher argv for the current OS, or nil if
// unsupported.
func platformArgv(rawURL string) []string {
	switch runtime.GOOS {
	case "linux":
		return []string{"xdg-open", rawURL}
	case "darwin":
		return []string{"open", rawURL}
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", rawURL}
	default:
		return nil
	}
}
